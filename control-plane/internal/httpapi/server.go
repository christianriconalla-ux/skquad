// Package httpapi exposes the skquad control-plane REST API.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/rossbrigoli/skquad/control-plane/internal/auth"
	"github.com/rossbrigoli/skquad/control-plane/internal/config"
	"github.com/rossbrigoli/skquad/control-plane/internal/domain"
	"github.com/rossbrigoli/skquad/control-plane/internal/storage"
)

// Store is the persistence surface required by the current API slice.
type Store interface {
	storage.UserStore
	storage.SquadStore
	storage.AgentStore
	storage.BoardStore
	storage.GrantStore
	storage.RegistryStore
	storage.PermissionStore
	storage.MeteringStore
	storage.AuditStore
	storage.TaskStore
}

// Server owns HTTP routing and request-scoped dependencies.
type Server struct {
	cfg      *config.Config
	store    Store
	oidcAuth OIDCAuthenticator
	crWriter CRWriter
}

// OIDCAuthenticator authenticates OIDC Authorization headers.
type OIDCAuthenticator interface {
	Authenticate(ctx context.Context, authorization string) (*auth.Profile, error)
}

// CRWriter mirrors persisted squad/agent state into Kubernetes custom
// resources for the operator.
type CRWriter interface {
	UpsertSquad(ctx context.Context, squad *domain.Squad) error
	DeleteSquad(ctx context.Context, squad *domain.Squad) error
	UpsertAgent(ctx context.Context, agent *domain.Agent, identity *domain.AgentIdentity) error
	DeleteAgent(ctx context.Context, agent *domain.Agent) error
}

// New returns an HTTP handler for the control-plane API.
func New(cfg *config.Config, store Store) http.Handler {
	return NewWithOIDCAuthenticator(cfg, store, nil)
}

// NewWithCRWriter returns an HTTP handler that mirrors squad/agent mutations
// to Kubernetes CRs.
func NewWithCRWriter(cfg *config.Config, store Store, crWriter CRWriter) http.Handler {
	return newServer(cfg, store, nil, crWriter)
}

// NewWithDependencies returns an HTTP handler with explicit optional
// integrations for tests and production startup.
func NewWithDependencies(cfg *config.Config, store Store, oidcAuth OIDCAuthenticator, crWriter CRWriter) http.Handler {
	return newServer(cfg, store, oidcAuth, crWriter)
}

// NewWithOIDCAuthenticator returns an HTTP handler using oidcAuth when
// SKQUAD_AUTH_MODE=oidc.
func NewWithOIDCAuthenticator(cfg *config.Config, store Store, oidcAuth OIDCAuthenticator) http.Handler {
	return newServer(cfg, store, oidcAuth, nil)
}

func newServer(cfg *config.Config, store Store, oidcAuth OIDCAuthenticator, crWriter CRWriter) http.Handler {
	if crWriter == nil {
		crWriter = noopCRWriter{}
	}
	s := &Server{cfg: cfg, store: store, oidcAuth: oidcAuth, crWriter: crWriter}

	r := chi.NewRouter()
	r.Get("/healthz", s.health)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.authenticate)

		r.Get("/auth/me", s.me)

		r.Post("/squads", s.createSquad)
		r.Get("/squads", s.listSquads)
		r.Get("/squads/{squadID}", s.getSquad)
		r.Patch("/squads/{squadID}", s.updateSquad)
		r.Delete("/squads/{squadID}", s.deleteSquad)
		r.Post("/squads/{squadID}/access-grants", s.createGrant)
		r.Get("/squads/{squadID}/access-grants", s.listGrants)
		r.Delete("/access-grants/{grantID}", s.deleteGrant)

		r.Post("/squads/{squadID}/agents", s.createAgent)
		r.Get("/squads/{squadID}/agents", s.listAgents)
		r.Get("/agents/{agentID}", s.getAgent)
		r.Patch("/agents/{agentID}", s.updateAgent)
		r.Delete("/agents/{agentID}", s.deleteAgent)
		r.Post("/agents/{agentID}/identity", s.createAgentIdentity)
		r.Post("/agents/{agentID}/identity/rotate", s.rotateAgentIdentity)
		r.Get("/agents/{agentID}/permissions", s.listAgentPermissions)
		r.Put("/agents/{agentID}/permissions", s.setAgentPermissions)

		r.Get("/squads/{squadID}/board", s.getBoard)
		r.Get("/squads/{squadID}/metering", s.getSquadMetering)
		r.Get("/squads/{squadID}/audit", s.listSquadAudit)
		r.Post("/squads/{squadID}/board/tasks", s.createTask)
		r.Get("/tasks/{taskID}", s.getTask)
		r.Patch("/tasks/{taskID}", s.updateTask)
		r.Post("/tasks/{taskID}/move", s.moveTask)
		r.Delete("/tasks/{taskID}", s.deleteTask)
		r.Get("/agents/{agentID}/metering", s.getAgentMetering)

		r.Post("/registry/llm-providers", s.createLLMProvider)
		r.Get("/registry/llm-providers", s.listLLMProviders)
		r.Get("/registry/llm-providers/{providerID}", s.getLLMProvider)
		r.Patch("/registry/llm-providers/{providerID}", s.updateLLMProvider)
		r.Post("/registry/llm-providers/{providerID}/deprecate", s.deprecateLLMProvider)

		r.Post("/registry/{registryType}", s.createRegistryResource)
		r.Get("/registry/{registryType}", s.listRegistryResources)
		r.Get("/registry/{registryType}/{resourceID}", s.getRegistryResource)
		r.Patch("/registry/{registryType}/{resourceID}", s.updateRegistryResource)
		r.Post("/registry/{registryType}/{resourceID}/deprecate", s.deprecateRegistryResource)

		r.Get("/metering/summary", s.getMeteringSummary)
		r.Get("/audit", s.listAudit)
	})

	return r
}

type noopCRWriter struct{}

func (noopCRWriter) UpsertSquad(context.Context, *domain.Squad) error { return nil }
func (noopCRWriter) DeleteSquad(context.Context, *domain.Squad) error { return nil }
func (noopCRWriter) UpsertAgent(context.Context, *domain.Agent, *domain.AgentIdentity) error {
	return nil
}
func (noopCRWriter) DeleteAgent(context.Context, *domain.Agent) error { return nil }

type principalKey struct{}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch s.cfg.AuthMode {
		case config.AuthDev:
			u := &domain.User{
				Email: s.cfg.DevEmail,
				Name:  s.cfg.DevName,
				Role:  domain.RolePlatformAdmin,
			}
			user, err := s.store.UpsertUser(r.Context(), u)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal", "failed to load dev principal")
				return
			}
			if err := s.store.SetUserRole(r.Context(), user.ID, domain.RolePlatformAdmin); err != nil {
				writeError(w, http.StatusInternalServerError, "internal", "failed to promote dev principal")
				return
			}
			user.Role = domain.RolePlatformAdmin
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, user)))
		case config.AuthOIDC:
			if s.oidcAuth == nil {
				writeError(w, http.StatusInternalServerError, "internal", "OIDC authentication is not configured")
				return
			}
			profile, err := s.oidcAuth.Authenticate(r.Context(), r.Header.Get("Authorization"))
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
				return
			}
			user, err := s.store.UpsertUser(r.Context(), &domain.User{
				Email: profile.Email,
				Name:  profile.Name,
				Role:  domain.RoleUser,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal", "failed to load authenticated principal")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, user)))
		default:
			writeError(w, http.StatusInternalServerError, "internal", "unsupported auth mode")
		}
	})
}

func currentUser(ctx context.Context) *domain.User {
	u, _ := ctx.Value(principalKey{}).(*domain.User)
	return u
}

func (s *Server) requirePlatformAdmin(w http.ResponseWriter, r *http.Request) bool {
	if currentUser(r.Context()).Role != domain.RolePlatformAdmin {
		writeError(w, http.StatusForbidden, "forbidden", "platform admin role is required")
		return false
	}
	return true
}

func registryTypeFromRequest(w http.ResponseWriter, r *http.Request) (domain.ResourceType, bool) {
	switch chi.URLParam(r, "registryType") {
	case "skills":
		return domain.ResSkill, true
	case "tools":
		return domain.ResTool, true
	case "apis":
		return domain.ResAPI, true
	case "knowledge-bases":
		return domain.ResKnowledgeBase, true
	case "project-workspaces":
		return domain.ResProjectWorkspace, true
	default:
		writeError(w, http.StatusNotFound, "not_found", "registry resource type not found")
		return "", false
	}
}

func resourceTypeFromString(value string) (domain.ResourceType, bool) {
	switch domain.ResourceType(value) {
	case domain.ResLLMProvider, domain.ResSkill, domain.ResTool, domain.ResAPI, domain.ResKnowledgeBase, domain.ResProjectWorkspace:
		return domain.ResourceType(value), true
	default:
		return "", false
	}
}

func validateName(w http.ResponseWriter, name string) bool {
	return validateRequired(w, "name", name)
}

func validateRequired(w http.ResponseWriter, field, value string) bool {
	if strings.TrimSpace(value) == "" {
		writeError(w, http.StatusBadRequest, "bad_request", field+" is required")
		return false
	}
	return true
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r.Context()))
}

func (s *Server) createLLMProvider(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatformAdmin(w, r) {
		return
	}
	var req struct {
		Name      string          `json:"name"`
		Kind      string          `json:"kind"`
		BaseURL   string          `json:"base_url"`
		APIKeyRef string          `json:"api_key_ref"`
		Models    json.RawMessage `json:"models"`
		Pricing   json.RawMessage `json:"pricing"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validateName(w, req.Name) || !validateRequired(w, "kind", req.Kind) || !validateRequired(w, "base_url", req.BaseURL) {
		return
	}
	if len(req.Models) == 0 {
		req.Models = json.RawMessage(`[]`)
	}
	if len(req.Pricing) == 0 {
		req.Pricing = json.RawMessage(`{}`)
	}
	u := currentUser(r.Context())
	provider := &domain.LLMProvider{
		Name:         strings.TrimSpace(req.Name),
		Kind:         strings.TrimSpace(req.Kind),
		BaseURL:      strings.TrimSpace(req.BaseURL),
		APIKeyRef:    req.APIKeyRef,
		Models:       req.Models,
		Pricing:      req.Pricing,
		Status:       domain.ResourceActive,
		RegisteredBy: u.ID,
	}
	created, err := s.store.CreateLLMProvider(r.Context(), provider)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	s.recordUserAudit(r, "registry.llm_provider.create", string(domain.ResLLMProvider), created.ID, "", nil)
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) listLLMProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := s.store.ListLLMProviders(r.Context())
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, providers)
}

func (s *Server) getLLMProvider(w http.ResponseWriter, r *http.Request) {
	provider, err := s.store.GetLLMProvider(r.Context(), chi.URLParam(r, "providerID"))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, provider)
}

func (s *Server) updateLLMProvider(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatformAdmin(w, r) {
		return
	}
	provider, err := s.store.GetLLMProvider(r.Context(), chi.URLParam(r, "providerID"))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	var req struct {
		Name      *string          `json:"name"`
		Kind      *string          `json:"kind"`
		BaseURL   *string          `json:"base_url"`
		APIKeyRef *string          `json:"api_key_ref"`
		Models    *json.RawMessage `json:"models"`
		Pricing   *json.RawMessage `json:"pricing"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name != nil {
		if !validateName(w, *req.Name) {
			return
		}
		provider.Name = strings.TrimSpace(*req.Name)
	}
	if req.Kind != nil {
		if !validateRequired(w, "kind", *req.Kind) {
			return
		}
		provider.Kind = strings.TrimSpace(*req.Kind)
	}
	if req.BaseURL != nil {
		if !validateRequired(w, "base_url", *req.BaseURL) {
			return
		}
		provider.BaseURL = strings.TrimSpace(*req.BaseURL)
	}
	if req.APIKeyRef != nil {
		provider.APIKeyRef = *req.APIKeyRef
	}
	if req.Models != nil {
		provider.Models = *req.Models
	}
	if req.Pricing != nil {
		provider.Pricing = *req.Pricing
	}
	updated, err := s.store.UpdateLLMProvider(r.Context(), provider)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	s.recordUserAudit(r, "registry.llm_provider.update", string(domain.ResLLMProvider), updated.ID, "", nil)
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deprecateLLMProvider(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatformAdmin(w, r) {
		return
	}
	if err := s.store.DeprecateLLMProvider(r.Context(), chi.URLParam(r, "providerID")); err != nil {
		writeStorageError(w, err)
		return
	}
	s.recordUserAudit(r, "registry.llm_provider.deprecate", string(domain.ResLLMProvider), chi.URLParam(r, "providerID"), "", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createRegistryResource(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatformAdmin(w, r) {
		return
	}
	typ, ok := registryTypeFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Endpoint    string          `json:"endpoint"`
		AuthRef     string          `json:"auth_ref"`
		Manifest    json.RawMessage `json:"manifest"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validateName(w, req.Name) {
		return
	}
	if len(req.Manifest) == 0 {
		req.Manifest = json.RawMessage(`{}`)
	}
	u := currentUser(r.Context())
	resource := &domain.RegistryResource{
		Type:         typ,
		Name:         strings.TrimSpace(req.Name),
		Description:  req.Description,
		Endpoint:     req.Endpoint,
		AuthRef:      req.AuthRef,
		Manifest:     req.Manifest,
		Status:       domain.ResourceActive,
		RegisteredBy: u.ID,
	}
	created, err := s.store.CreateResource(r.Context(), resource)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	s.recordUserAudit(r, "registry.resource.create", string(typ), created.ID, "", nil)
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) listRegistryResources(w http.ResponseWriter, r *http.Request) {
	typ, ok := registryTypeFromRequest(w, r)
	if !ok {
		return
	}
	resources, err := s.store.ListResources(r.Context(), typ)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resources)
}

func (s *Server) getRegistryResource(w http.ResponseWriter, r *http.Request) {
	typ, ok := registryTypeFromRequest(w, r)
	if !ok {
		return
	}
	resource, err := s.store.GetResource(r.Context(), typ, chi.URLParam(r, "resourceID"))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resource)
}

func (s *Server) updateRegistryResource(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatformAdmin(w, r) {
		return
	}
	typ, ok := registryTypeFromRequest(w, r)
	if !ok {
		return
	}
	resource, err := s.store.GetResource(r.Context(), typ, chi.URLParam(r, "resourceID"))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	var req struct {
		Name        *string          `json:"name"`
		Description *string          `json:"description"`
		Endpoint    *string          `json:"endpoint"`
		AuthRef     *string          `json:"auth_ref"`
		Manifest    *json.RawMessage `json:"manifest"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name != nil {
		if !validateName(w, *req.Name) {
			return
		}
		resource.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		resource.Description = *req.Description
	}
	if req.Endpoint != nil {
		resource.Endpoint = *req.Endpoint
	}
	if req.AuthRef != nil {
		resource.AuthRef = *req.AuthRef
	}
	if req.Manifest != nil {
		resource.Manifest = *req.Manifest
	}
	updated, err := s.store.UpdateResource(r.Context(), resource)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	s.recordUserAudit(r, "registry.resource.update", string(typ), updated.ID, "", nil)
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deprecateRegistryResource(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatformAdmin(w, r) {
		return
	}
	typ, ok := registryTypeFromRequest(w, r)
	if !ok {
		return
	}
	if err := s.store.DeprecateResource(r.Context(), typ, chi.URLParam(r, "resourceID")); err != nil {
		writeStorageError(w, err)
		return
	}
	s.recordUserAudit(r, "registry.resource.deprecate", string(typ), chi.URLParam(r, "resourceID"), "", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createSquad(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string          `json:"name"`
		Mission        string          `json:"mission"`
		OperatingModel json.RawMessage `json:"operating_model"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "name is required")
		return
	}
	if len(req.OperatingModel) == 0 {
		req.OperatingModel = json.RawMessage(`{}`)
	}

	u := currentUser(r.Context())
	squad := &domain.Squad{
		Name:           req.Name,
		Mission:        req.Mission,
		OperatingModel: req.OperatingModel,
		OwnerID:        u.ID,
		Namespace:      namespaceFor(req.Name),
		Status:         domain.SquadActive,
	}
	created, err := s.store.CreateSquad(r.Context(), squad)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	if err := s.crWriter.UpsertSquad(r.Context(), created); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to write squad custom resource")
		return
	}
	s.recordUserAudit(r, "squad.create", "squad", created.ID, created.ID, nil)
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) listSquads(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	ownerID := u.ID
	if u.Role == domain.RolePlatformAdmin && r.URL.Query().Get("all") == "true" {
		ownerID = ""
	}
	squads, err := s.store.ListSquads(r.Context(), ownerID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, squads)
}

func (s *Server) getSquad(w http.ResponseWriter, r *http.Request) {
	squad, ok := s.loadAccessibleSquad(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, squad)
}

func (s *Server) updateSquad(w http.ResponseWriter, r *http.Request) {
	squad, ok := s.loadOwnedSquad(w, r)
	if !ok {
		return
	}

	var req struct {
		Name           *string          `json:"name"`
		Mission        *string          `json:"mission"`
		OperatingModel *json.RawMessage `json:"operating_model"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "name must not be empty")
			return
		}
		squad.Name = name
	}
	if req.Mission != nil {
		squad.Mission = *req.Mission
	}
	if req.OperatingModel != nil {
		squad.OperatingModel = *req.OperatingModel
	}

	updated, err := s.store.UpdateSquad(r.Context(), squad)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	if err := s.crWriter.UpsertSquad(r.Context(), updated); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to write squad custom resource")
		return
	}
	s.recordUserAudit(r, "squad.update", "squad", updated.ID, updated.ID, nil)
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteSquad(w http.ResponseWriter, r *http.Request) {
	squad, ok := s.loadOwnedSquad(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteSquad(r.Context(), squad.ID); err != nil {
		writeStorageError(w, err)
		return
	}
	if err := s.crWriter.DeleteSquad(r.Context(), squad); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to delete squad custom resource")
		return
	}
	s.recordUserAudit(r, "squad.delete", "squad", squad.ID, squad.ID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createGrant(w http.ResponseWriter, r *http.Request) {
	squad, ok := s.loadOwnedSquad(w, r)
	if !ok {
		return
	}
	var req struct {
		GranteeType domain.GranteeType `json:"grantee_type"`
		GranteeID   string             `json:"grantee_id"`
		Permissions string             `json:"permissions"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.GranteeType != domain.GranteeUser && req.GranteeType != domain.GranteeAgent {
		writeError(w, http.StatusBadRequest, "bad_request", "grantee_type is invalid")
		return
	}
	req.GranteeID = strings.TrimSpace(req.GranteeID)
	if req.GranteeID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "grantee_id is required")
		return
	}
	if req.Permissions == "" {
		req.Permissions = "talk"
	}
	if req.GranteeType == domain.GranteeUser {
		if _, err := s.store.GetUser(r.Context(), req.GranteeID); err != nil {
			writeStorageError(w, err)
			return
		}
	} else if _, err := s.store.GetAgent(r.Context(), req.GranteeID); err != nil {
		writeStorageError(w, err)
		return
	}

	u := currentUser(r.Context())
	grant := &domain.AccessGrant{
		SquadID:     squad.ID,
		GranteeType: req.GranteeType,
		GranteeID:   req.GranteeID,
		Permissions: req.Permissions,
		GrantedBy:   u.ID,
	}
	created, err := s.store.CreateGrant(r.Context(), grant)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	s.recordUserAudit(r, "access_grant.create", "access_grant", created.ID, squad.ID, nil)
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) listGrants(w http.ResponseWriter, r *http.Request) {
	squad, ok := s.loadOwnedSquad(w, r)
	if !ok {
		return
	}
	grants, err := s.store.ListGrants(r.Context(), squad.ID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, grants)
}

func (s *Server) deleteGrant(w http.ResponseWriter, r *http.Request) {
	grant, err := s.store.GetGrant(r.Context(), chi.URLParam(r, "grantID"))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	if _, ok := s.ensureSquadAccess(w, r, grant.SquadID, true); !ok {
		return
	}
	if err := s.store.RevokeGrant(r.Context(), grant.ID); err != nil {
		writeStorageError(w, err)
		return
	}
	s.recordUserAudit(r, "access_grant.delete", "access_grant", grant.ID, grant.SquadID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	squad, ok := s.loadOwnedSquad(w, r)
	if !ok {
		return
	}
	var req struct {
		Name              string          `json:"name"`
		Role              string          `json:"role"`
		DefaultProviderID string          `json:"default_provider_id"`
		Permissions       json.RawMessage `json:"permissions"`
		IdleTimeoutSec    int             `json:"idle_timeout_sec"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "name is required")
		return
	}
	if len(req.Permissions) == 0 {
		req.Permissions = json.RawMessage(`[]`)
	}
	if req.IdleTimeoutSec <= 0 {
		req.IdleTimeoutSec = int(s.cfg.DefaultIdleTimeout / time.Second)
	}

	agent := &domain.Agent{
		SquadID:         squad.ID,
		Name:            req.Name,
		Role:            req.Role,
		DefaultProvider: req.DefaultProviderID,
		Permissions:     req.Permissions,
		IdleTimeoutSec:  req.IdleTimeoutSec,
		Status:          domain.AgentIdle,
	}
	created, err := s.store.CreateAgent(r.Context(), agent)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	if err := s.upsertAgentCR(r.Context(), created); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to write agent custom resource")
		return
	}
	s.recordUserAudit(r, "agent.create", "agent", created.ID, squad.ID, nil)
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	squad, ok := s.loadAccessibleSquad(w, r)
	if !ok {
		return
	}
	agents, err := s.store.ListAgents(r.Context(), squad.ID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.loadAccessibleAgent(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (s *Server) updateAgent(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.loadOwnedAgent(w, r)
	if !ok {
		return
	}
	var req struct {
		Name              *string          `json:"name"`
		Role              *string          `json:"role"`
		DefaultProviderID *string          `json:"default_provider_id"`
		Permissions       *json.RawMessage `json:"permissions"`
		IdleTimeoutSec    *int             `json:"idle_timeout_sec"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "name must not be empty")
			return
		}
		agent.Name = name
	}
	if req.Role != nil {
		agent.Role = *req.Role
	}
	if req.DefaultProviderID != nil {
		agent.DefaultProvider = *req.DefaultProviderID
	}
	if req.Permissions != nil {
		agent.Permissions = *req.Permissions
	}
	if req.IdleTimeoutSec != nil {
		if *req.IdleTimeoutSec <= 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "idle_timeout_sec must be positive")
			return
		}
		agent.IdleTimeoutSec = *req.IdleTimeoutSec
	}

	updated, err := s.store.UpdateAgent(r.Context(), agent)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	if err := s.upsertAgentCR(r.Context(), updated); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to write agent custom resource")
		return
	}
	s.recordUserAudit(r, "agent.update", "agent", updated.ID, updated.SquadID, nil)
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteAgent(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.loadOwnedAgent(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteAgent(r.Context(), agent.ID); err != nil {
		writeStorageError(w, err)
		return
	}
	if err := s.crWriter.DeleteAgent(r.Context(), agent); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to delete agent custom resource")
		return
	}
	s.recordUserAudit(r, "agent.delete", "agent", agent.ID, agent.SquadID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createAgentIdentity(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.loadOwnedAgent(w, r)
	if !ok {
		return
	}
	squad, err := s.store.GetSquad(r.Context(), agent.SquadID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	u := currentUser(r.Context())
	identity := &domain.AgentIdentity{
		AgentID:       agent.ID,
		CredentialRef: generatedCredentialRef(squad.Namespace, agent.ID),
		VirtualKeyRef: generatedVirtualKeyRef(agent.ID),
		CreatedBy:     u.ID,
	}
	created, err := s.store.CreateAgentIdentity(r.Context(), identity)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	if err := s.crWriter.UpsertAgent(r.Context(), agent, created); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to write agent custom resource")
		return
	}
	s.recordUserAudit(r, "agent_identity.create", "agent_identity", created.ID, agent.SquadID, nil)
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) rotateAgentIdentity(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.loadOwnedAgent(w, r)
	if !ok {
		return
	}
	squad, err := s.store.GetSquad(r.Context(), agent.SquadID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	identity, err := s.store.RotateAgentIdentity(r.Context(), agent.ID, generatedCredentialRef(squad.Namespace, agent.ID))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	if err := s.crWriter.UpsertAgent(r.Context(), agent, identity); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to write agent custom resource")
		return
	}
	s.recordUserAudit(r, "agent_identity.rotate", "agent_identity", identity.ID, agent.SquadID, nil)
	writeJSON(w, http.StatusOK, identity)
}

func (s *Server) listAgentPermissions(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.loadOwnedAgent(w, r)
	if !ok {
		return
	}
	perms, err := s.store.ListAgentPermissions(r.Context(), agent.ID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, perms)
}

func (s *Server) setAgentPermissions(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.loadOwnedAgent(w, r)
	if !ok {
		return
	}
	var req []struct {
		ResourceType string `json:"resource_type"`
		ResourceID   string `json:"resource_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	u := currentUser(r.Context())
	perms := make([]domain.AgentPermission, 0, len(req))
	seen := map[string]bool{}
	for _, item := range req {
		typ, ok := resourceTypeFromString(item.ResourceType)
		if !ok {
			writeError(w, http.StatusBadRequest, "bad_request", "resource_type is invalid")
			return
		}
		resourceID := strings.TrimSpace(item.ResourceID)
		if resourceID == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "resource_id is required")
			return
		}
		if err := s.ensureRegistryResourceExists(r.Context(), typ, resourceID); err != nil {
			writeStorageError(w, err)
			return
		}
		key := string(typ) + ":" + resourceID
		if seen[key] {
			continue
		}
		seen[key] = true
		perms = append(perms, domain.AgentPermission{
			AgentID:      agent.ID,
			ResourceType: typ,
			ResourceID:   resourceID,
			GrantedBy:    u.ID,
		})
	}
	if err := s.store.SetAgentPermissions(r.Context(), agent.ID, perms); err != nil {
		writeStorageError(w, err)
		return
	}
	current, err := s.store.ListAgentPermissions(r.Context(), agent.ID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	metadata, _ := json.Marshal(map[string]int{"count": len(current)})
	s.recordUserAudit(r, "agent_permissions.set", "agent", agent.ID, agent.SquadID, metadata)
	writeJSON(w, http.StatusOK, current)
}

func (s *Server) ensureRegistryResourceExists(ctx context.Context, typ domain.ResourceType, resourceID string) error {
	if typ == domain.ResLLMProvider {
		_, err := s.store.GetLLMProvider(ctx, resourceID)
		return err
	}
	_, err := s.store.GetResource(ctx, typ, resourceID)
	return err
}

func (s *Server) getBoard(w http.ResponseWriter, r *http.Request) {
	squad, ok := s.loadAccessibleSquad(w, r)
	if !ok {
		return
	}
	board, err := s.store.GetBoard(r.Context(), squad.ID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	tasks, err := s.store.ListTasks(r.Context(), board.ID, "")
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"board": board,
		"tasks": tasks,
	})
}

func (s *Server) getSquadMetering(w http.ResponseWriter, r *http.Request) {
	squad, ok := s.loadOwnedOrAdminSquad(w, r)
	if !ok {
		return
	}
	usage, err := s.store.SumMetering(r.Context(), squad.ID, "")
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

func (s *Server) getAgentMetering(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.loadOwnedOrAdminAgent(w, r)
	if !ok {
		return
	}
	usage, err := s.store.SumMetering(r.Context(), "", agent.ID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

func (s *Server) getMeteringSummary(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatformAdmin(w, r) {
		return
	}
	usage, err := s.store.SumMetering(r.Context(), "", "")
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

func (s *Server) listSquadAudit(w http.ResponseWriter, r *http.Request) {
	squad, ok := s.loadOwnedOrAdminSquad(w, r)
	if !ok {
		return
	}
	entries, err := s.store.ListAudit(r.Context(), squad.ID, auditLimit(r))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatformAdmin(w, r) {
		return
	}
	entries, err := s.store.ListAudit(r.Context(), r.URL.Query().Get("squad_id"), auditLimit(r))
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	squad, ok := s.loadOwnedSquad(w, r)
	if !ok {
		return
	}
	board, err := s.store.GetBoard(r.Context(), squad.ID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	var req struct {
		Title           string            `json:"title"`
		Description     string            `json:"description"`
		AssigneeAgentID string            `json:"assignee_agent_id"`
		Metadata        map[string]string `json:"metadata"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "title is required")
		return
	}
	if req.AssigneeAgentID != "" {
		agent, err := s.store.GetAgent(r.Context(), req.AssigneeAgentID)
		if err != nil {
			writeStorageError(w, err)
			return
		}
		if agent.SquadID != squad.ID {
			writeError(w, http.StatusBadRequest, "bad_request", "assignee_agent_id must belong to this squad")
			return
		}
	}
	u := currentUser(r.Context())
	task := &domain.Task{
		BoardID:         board.ID,
		SquadID:         squad.ID,
		Title:           req.Title,
		Description:     req.Description,
		Status:          domain.TaskTodo,
		AssigneeAgentID: req.AssigneeAgentID,
		CreatedByType:   "user",
		CreatedByID:     u.ID,
	}
	created, err := s.store.CreateTask(r.Context(), task)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	s.recordUserAudit(r, "task.create", "task", created.ID, squad.ID, nil)
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	task, ok := s.loadAccessibleTask(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) updateTask(w http.ResponseWriter, r *http.Request) {
	task, ok := s.loadOwnedTask(w, r)
	if !ok {
		return
	}
	var req struct {
		Title           *string `json:"title"`
		Description     *string `json:"description"`
		AssigneeAgentID *string `json:"assignee_agent_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "title must not be empty")
			return
		}
		task.Title = title
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.AssigneeAgentID != nil {
		if *req.AssigneeAgentID != "" {
			agent, err := s.store.GetAgent(r.Context(), *req.AssigneeAgentID)
			if err != nil {
				writeStorageError(w, err)
				return
			}
			if agent.SquadID != task.SquadID {
				writeError(w, http.StatusBadRequest, "bad_request", "assignee_agent_id must belong to this squad")
				return
			}
		}
		task.AssigneeAgentID = *req.AssigneeAgentID
	}

	updated, err := s.store.UpdateTask(r.Context(), task)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	s.recordUserAudit(r, "task.update", "task", updated.ID, updated.SquadID, nil)
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) moveTask(w http.ResponseWriter, r *http.Request) {
	task, ok := s.loadOwnedTask(w, r)
	if !ok {
		return
	}
	var req struct {
		Status domain.TaskStatus `json:"status"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !req.Status.Valid() {
		writeError(w, http.StatusBadRequest, "bad_request", "status is invalid")
		return
	}
	task.Status = req.Status
	updated, err := s.store.UpdateTask(r.Context(), task)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	s.recordUserAudit(r, "task.move", "task", updated.ID, updated.SquadID, nil)
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	task, ok := s.loadOwnedTask(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteTask(r.Context(), task.ID); err != nil {
		writeStorageError(w, err)
		return
	}
	s.recordUserAudit(r, "task.delete", "task", task.ID, task.SquadID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) loadOwnedOrAdminSquad(w http.ResponseWriter, r *http.Request) (*domain.Squad, bool) {
	squad, err := s.store.GetSquad(r.Context(), chi.URLParam(r, "squadID"))
	if err != nil {
		writeStorageError(w, err)
		return nil, false
	}
	u := currentUser(r.Context())
	if squad.OwnerID == u.ID || u.Role == domain.RolePlatformAdmin {
		return squad, true
	}
	writeError(w, http.StatusForbidden, "forbidden", "you do not own this squad")
	return nil, false
}

func (s *Server) loadOwnedOrAdminAgent(w http.ResponseWriter, r *http.Request) (*domain.Agent, bool) {
	agent, err := s.store.GetAgent(r.Context(), chi.URLParam(r, "agentID"))
	if err != nil {
		writeStorageError(w, err)
		return nil, false
	}
	if _, ok := s.ensureOwnedOrAdminSquad(w, r, agent.SquadID); !ok {
		return nil, false
	}
	return agent, true
}

func (s *Server) loadOwnedSquad(w http.ResponseWriter, r *http.Request) (*domain.Squad, bool) {
	squad, ok := s.loadAccessibleSquad(w, r)
	if !ok {
		return nil, false
	}
	if squad.OwnerID != currentUser(r.Context()).ID {
		writeError(w, http.StatusForbidden, "forbidden", "you do not own this squad")
		return nil, false
	}
	return squad, true
}

func (s *Server) loadAccessibleSquad(w http.ResponseWriter, r *http.Request) (*domain.Squad, bool) {
	squad, err := s.store.GetSquad(r.Context(), chi.URLParam(r, "squadID"))
	if err != nil {
		writeStorageError(w, err)
		return nil, false
	}
	u := currentUser(r.Context())
	if squad.OwnerID == u.ID || u.Role == domain.RolePlatformAdmin {
		return squad, true
	}
	ok, err := s.store.UserMayAccessSquad(r.Context(), u.ID, squad.ID)
	if err != nil {
		writeStorageError(w, err)
		return nil, false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden", "you do not have access to this squad")
		return nil, false
	}
	return squad, true
}

func (s *Server) loadAccessibleAgent(w http.ResponseWriter, r *http.Request) (*domain.Agent, bool) {
	agent, err := s.store.GetAgent(r.Context(), chi.URLParam(r, "agentID"))
	if err != nil {
		writeStorageError(w, err)
		return nil, false
	}
	if _, ok := s.ensureSquadAccess(w, r, agent.SquadID, false); !ok {
		return nil, false
	}
	return agent, true
}

func (s *Server) loadOwnedAgent(w http.ResponseWriter, r *http.Request) (*domain.Agent, bool) {
	agent, err := s.store.GetAgent(r.Context(), chi.URLParam(r, "agentID"))
	if err != nil {
		writeStorageError(w, err)
		return nil, false
	}
	if _, ok := s.ensureSquadAccess(w, r, agent.SquadID, true); !ok {
		return nil, false
	}
	return agent, true
}

func (s *Server) loadAccessibleTask(w http.ResponseWriter, r *http.Request) (*domain.Task, bool) {
	task, err := s.store.GetTask(r.Context(), chi.URLParam(r, "taskID"))
	if err != nil {
		writeStorageError(w, err)
		return nil, false
	}
	if _, ok := s.ensureSquadAccess(w, r, task.SquadID, false); !ok {
		return nil, false
	}
	return task, true
}

func (s *Server) loadOwnedTask(w http.ResponseWriter, r *http.Request) (*domain.Task, bool) {
	task, err := s.store.GetTask(r.Context(), chi.URLParam(r, "taskID"))
	if err != nil {
		writeStorageError(w, err)
		return nil, false
	}
	if _, ok := s.ensureSquadAccess(w, r, task.SquadID, true); !ok {
		return nil, false
	}
	return task, true
}

func (s *Server) ensureSquadAccess(w http.ResponseWriter, r *http.Request, squadID string, ownerOnly bool) (*domain.Squad, bool) {
	squad, err := s.store.GetSquad(r.Context(), squadID)
	if err != nil {
		writeStorageError(w, err)
		return nil, false
	}
	u := currentUser(r.Context())
	if squad.OwnerID == u.ID || (!ownerOnly && u.Role == domain.RolePlatformAdmin) {
		return squad, true
	}
	if !ownerOnly {
		ok, err := s.store.UserMayAccessSquad(r.Context(), u.ID, squad.ID)
		if err != nil {
			writeStorageError(w, err)
			return nil, false
		}
		if ok {
			return squad, true
		}
	}
	if ownerOnly {
		writeError(w, http.StatusForbidden, "forbidden", "you do not own this squad")
	} else {
		writeError(w, http.StatusForbidden, "forbidden", "you do not have access to this squad")
	}
	return nil, false
}

func (s *Server) ensureOwnedOrAdminSquad(w http.ResponseWriter, r *http.Request, squadID string) (*domain.Squad, bool) {
	squad, err := s.store.GetSquad(r.Context(), squadID)
	if err != nil {
		writeStorageError(w, err)
		return nil, false
	}
	u := currentUser(r.Context())
	if squad.OwnerID == u.ID || u.Role == domain.RolePlatformAdmin {
		return squad, true
	}
	writeError(w, http.StatusForbidden, "forbidden", "you do not own this squad")
	return nil, false
}

func (s *Server) recordUserAudit(r *http.Request, action, resourceType, resourceID, squadID string, metadata json.RawMessage) {
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	u := currentUser(r.Context())
	if u == nil {
		return
	}
	_ = s.store.RecordAudit(r.Context(), &domain.AuditEntry{
		ActorType:    "user",
		ActorID:      u.ID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		SquadID:      squadID,
		Metadata:     metadata,
	})
}

func (s *Server) upsertAgentCR(ctx context.Context, agent *domain.Agent) error {
	identity, err := s.store.GetAgentIdentity(ctx, agent.ID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return s.crWriter.UpsertAgent(ctx, agent, nil)
		}
		return err
	}
	return s.crWriter.UpsertAgent(ctx, agent, identity)
}

func auditLimit(r *http.Request) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return false
	}
	return true
}

func writeStorageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, storage.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "resource already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal", "unexpected storage error")
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func namespaceFor(name string) string {
	parts := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	slug := strings.Join(parts, "-")
	if slug == "" {
		slug = "squad"
	}
	if len(slug) > 40 {
		slug = slug[:40]
	}
	return fmt.Sprintf("squad-%s-%s", slug, uuid.NewString()[:8])
}

func generatedCredentialRef(namespace, agentID string) string {
	return fmt.Sprintf("k8s://%s/agent-%s-credential-%s", namespace, agentID, uuid.NewString()[:8])
}

func generatedVirtualKeyRef(agentID string) string {
	return "llm-gateway://virtual-keys/agent-" + agentID
}
