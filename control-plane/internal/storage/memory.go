package storage

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/rossbrigoli/skquad/control-plane/internal/domain"
)

// MemoryStore is a process-local store used for development and handler tests.
// It is intentionally simple; production persistence belongs in the Postgres
// implementation.
type MemoryStore struct {
	mu sync.RWMutex

	users         map[string]*domain.User
	usersByEmail  map[string]string
	squads        map[string]*domain.Squad
	agents        map[string]*domain.Agent
	identities    map[string]*domain.AgentIdentity
	identityAgent map[string]string
	boards        map[string]*domain.Board
	boardsBySquad map[string]string
	grants        map[string]*domain.AccessGrant
	llmProviders  map[string]*domain.LLMProvider
	resources     map[string]*domain.RegistryResource
	permissions   map[string]*domain.AgentPermission
	metering      map[string]*domain.MeteringEvent
	auditLog      map[string]*domain.AuditEntry
	tasks         map[string]*domain.Task
}

// NewMemoryStore creates an empty development store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		users:         map[string]*domain.User{},
		usersByEmail:  map[string]string{},
		squads:        map[string]*domain.Squad{},
		agents:        map[string]*domain.Agent{},
		identities:    map[string]*domain.AgentIdentity{},
		identityAgent: map[string]string{},
		boards:        map[string]*domain.Board{},
		boardsBySquad: map[string]string{},
		grants:        map[string]*domain.AccessGrant{},
		llmProviders:  map[string]*domain.LLMProvider{},
		resources:     map[string]*domain.RegistryResource{},
		permissions:   map[string]*domain.AgentPermission{},
		metering:      map[string]*domain.MeteringEvent{},
		auditLog:      map[string]*domain.AuditEntry{},
		tasks:         map[string]*domain.Task{},
	}
}

func (m *MemoryStore) GetUser(_ context.Context, id string) (*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneUser(u), nil
}

func (m *MemoryStore) GetUserByEmail(_ context.Context, email string) (*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.usersByEmail[strings.ToLower(email)]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneUser(m.users[id]), nil
}

func (m *MemoryStore) UpsertUser(_ context.Context, u *domain.User) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	email := strings.ToLower(strings.TrimSpace(u.Email))
	if id, ok := m.usersByEmail[email]; ok {
		existing := cloneUser(m.users[id])
		if u.Name != "" {
			existing.Name = u.Name
		}
		m.users[id] = existing
		return cloneUser(existing), nil
	}

	now := time.Now().UTC()
	created := cloneUser(u)
	created.ID = uuid.NewString()
	created.Email = email
	if created.Role == "" {
		created.Role = domain.RoleUser
	}
	created.CreatedAt = now
	m.users[created.ID] = created
	m.usersByEmail[email] = created.ID
	return cloneUser(created), nil
}

func (m *MemoryStore) SetUserRole(_ context.Context, id string, role domain.Role) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !role.Valid() {
		return ErrConflict
	}
	u, ok := m.users[id]
	if !ok {
		return ErrNotFound
	}
	u.Role = role
	return nil
}

func (m *MemoryStore) ListUsers(_ context.Context) ([]*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*domain.User, 0, len(m.users))
	for _, u := range m.users {
		out = append(out, cloneUser(u))
	}
	slices.SortFunc(out, func(a, b *domain.User) int {
		return strings.Compare(a.Email, b.Email)
	})
	return out, nil
}

func (m *MemoryStore) CreateSquad(_ context.Context, s *domain.Squad) (*domain.Squad, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.squads {
		if existing.OwnerID == s.OwnerID && strings.EqualFold(existing.Name, s.Name) {
			return nil, ErrConflict
		}
	}

	now := time.Now().UTC()
	created := cloneSquad(s)
	created.ID = uuid.NewString()
	if created.Status == "" {
		created.Status = domain.SquadActive
	}
	created.CreatedAt = now
	created.UpdatedAt = now
	m.squads[created.ID] = created

	board := &domain.Board{ID: uuid.NewString(), SquadID: created.ID, CreatedAt: now}
	m.boards[board.ID] = board
	m.boardsBySquad[created.ID] = board.ID
	return cloneSquad(created), nil
}

func (m *MemoryStore) GetSquad(_ context.Context, id string) (*domain.Squad, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.squads[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneSquad(s), nil
}

func (m *MemoryStore) GetSquadByName(_ context.Context, ownerID, name string) (*domain.Squad, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.squads {
		if s.OwnerID == ownerID && strings.EqualFold(s.Name, name) {
			return cloneSquad(s), nil
		}
	}
	return nil, ErrNotFound
}

func (m *MemoryStore) UpdateSquad(_ context.Context, s *domain.Squad) (*domain.Squad, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.squads[s.ID]
	if !ok {
		return nil, ErrNotFound
	}
	for _, other := range m.squads {
		if other.ID != s.ID && other.OwnerID == existing.OwnerID && strings.EqualFold(other.Name, s.Name) {
			return nil, ErrConflict
		}
	}
	updated := cloneSquad(s)
	updated.OwnerID = existing.OwnerID
	updated.Namespace = existing.Namespace
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now().UTC()
	m.squads[s.ID] = updated
	return cloneSquad(updated), nil
}

func (m *MemoryStore) DeleteSquad(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.squads[id]; !ok {
		return ErrNotFound
	}
	delete(m.squads, id)
	if boardID, ok := m.boardsBySquad[id]; ok {
		delete(m.boards, boardID)
		delete(m.boardsBySquad, id)
		for taskID, task := range m.tasks {
			if task.BoardID == boardID {
				delete(m.tasks, taskID)
			}
		}
	}
	for agentID, agent := range m.agents {
		if agent.SquadID == id {
			delete(m.agents, agentID)
		}
	}
	for grantID, grant := range m.grants {
		if grant.SquadID == id {
			delete(m.grants, grantID)
		}
	}
	return nil
}

func (m *MemoryStore) ListSquads(_ context.Context, ownerID string) ([]*domain.Squad, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*domain.Squad, 0, len(m.squads))
	for _, s := range m.squads {
		if ownerID == "" || s.OwnerID == ownerID {
			out = append(out, cloneSquad(s))
		}
	}
	slices.SortFunc(out, func(a, b *domain.Squad) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

func (m *MemoryStore) CreateAgent(_ context.Context, a *domain.Agent) (*domain.Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.squads[a.SquadID]; !ok {
		return nil, ErrNotFound
	}
	for _, existing := range m.agents {
		if existing.SquadID == a.SquadID && strings.EqualFold(existing.Name, a.Name) {
			return nil, ErrConflict
		}
	}
	now := time.Now().UTC()
	created := cloneAgent(a)
	created.ID = uuid.NewString()
	if created.Status == "" {
		created.Status = domain.AgentIdle
	}
	created.CreatedAt = now
	created.UpdatedAt = now
	m.agents[created.ID] = created
	return cloneAgent(created), nil
}

func (m *MemoryStore) GetAgent(_ context.Context, id string) (*domain.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.agents[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneAgent(a), nil
}

func (m *MemoryStore) UpdateAgent(_ context.Context, a *domain.Agent) (*domain.Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.agents[a.ID]
	if !ok {
		return nil, ErrNotFound
	}
	for _, other := range m.agents {
		if other.ID != a.ID && other.SquadID == existing.SquadID && strings.EqualFold(other.Name, a.Name) {
			return nil, ErrConflict
		}
	}
	updated := cloneAgent(a)
	updated.SquadID = existing.SquadID
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now().UTC()
	m.agents[a.ID] = updated
	return cloneAgent(updated), nil
}

func (m *MemoryStore) DeleteAgent(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.agents[id]; !ok {
		return ErrNotFound
	}
	delete(m.agents, id)
	for _, task := range m.tasks {
		if task.AssigneeAgentID == id {
			task.AssigneeAgentID = ""
			task.UpdatedAt = time.Now().UTC()
		}
	}
	return nil
}

func (m *MemoryStore) ListAgents(_ context.Context, squadID string) ([]*domain.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []*domain.Agent{}
	for _, a := range m.agents {
		if a.SquadID == squadID {
			out = append(out, cloneAgent(a))
		}
	}
	slices.SortFunc(out, func(a, b *domain.Agent) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

func (m *MemoryStore) SetAgentStatus(_ context.Context, id string, status domain.AgentStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.agents[id]
	if !ok {
		return ErrNotFound
	}
	a.Status = status
	a.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MemoryStore) CreateAgentIdentity(_ context.Context, i *domain.AgentIdentity) (*domain.AgentIdentity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	agent, ok := m.agents[i.AgentID]
	if !ok {
		return nil, ErrNotFound
	}
	if _, ok := m.identityAgent[i.AgentID]; ok {
		return nil, ErrConflict
	}
	created := cloneAgentIdentity(i)
	created.ID = uuid.NewString()
	created.CreatedAt = time.Now().UTC()
	m.identities[created.ID] = created
	m.identityAgent[created.AgentID] = created.ID
	agent.IdentityID = created.ID
	agent.UpdatedAt = created.CreatedAt
	return cloneAgentIdentity(created), nil
}

func (m *MemoryStore) GetAgentIdentity(_ context.Context, agentID string) (*domain.AgentIdentity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	identityID, ok := m.identityAgent[agentID]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneAgentIdentity(m.identities[identityID]), nil
}

func (m *MemoryStore) RotateAgentIdentity(_ context.Context, agentID string, credentialRef string) (*domain.AgentIdentity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	identityID, ok := m.identityAgent[agentID]
	if !ok {
		return nil, ErrNotFound
	}
	identity := m.identities[identityID]
	identity.CredentialRef = credentialRef
	identity.RotatedAt = time.Now().UTC()
	return cloneAgentIdentity(identity), nil
}

func (m *MemoryStore) CreateGrant(_ context.Context, g *domain.AccessGrant) (*domain.AccessGrant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.squads[g.SquadID]; !ok {
		return nil, ErrNotFound
	}
	for _, existing := range m.grants {
		if existing.SquadID == g.SquadID && existing.GranteeType == g.GranteeType && existing.GranteeID == g.GranteeID {
			return nil, ErrConflict
		}
	}
	created := cloneGrant(g)
	created.ID = uuid.NewString()
	created.CreatedAt = time.Now().UTC()
	m.grants[created.ID] = created
	return cloneGrant(created), nil
}

func (m *MemoryStore) GetGrant(_ context.Context, id string) (*domain.AccessGrant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	grant, ok := m.grants[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneGrant(grant), nil
}

func (m *MemoryStore) RevokeGrant(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.grants[id]; !ok {
		return ErrNotFound
	}
	delete(m.grants, id)
	return nil
}

func (m *MemoryStore) ListGrants(_ context.Context, squadID string) ([]*domain.AccessGrant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []*domain.AccessGrant{}
	for _, grant := range m.grants {
		if grant.SquadID == squadID {
			out = append(out, cloneGrant(grant))
		}
	}
	slices.SortFunc(out, func(a, b *domain.AccessGrant) int {
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

func (m *MemoryStore) UserMayAccessSquad(_ context.Context, userID, squadID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if squad, ok := m.squads[squadID]; ok && squad.OwnerID == userID {
		return true, nil
	}
	for _, grant := range m.grants {
		if grant.SquadID == squadID && grant.GranteeType == domain.GranteeUser && grant.GranteeID == userID {
			return true, nil
		}
	}
	return false, nil
}

func (m *MemoryStore) AgentMayMessageSquad(_ context.Context, agentID, squadID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, grant := range m.grants {
		if grant.SquadID == squadID && grant.GranteeType == domain.GranteeAgent && grant.GranteeID == agentID {
			return true, nil
		}
	}
	return false, nil
}

func (m *MemoryStore) CreateLLMProvider(_ context.Context, p *domain.LLMProvider) (*domain.LLMProvider, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.llmProviders {
		if strings.EqualFold(existing.Name, p.Name) {
			return nil, ErrConflict
		}
	}
	created := cloneLLMProvider(p)
	created.ID = uuid.NewString()
	if created.Status == "" {
		created.Status = domain.ResourceActive
	}
	created.CreatedAt = time.Now().UTC()
	m.llmProviders[created.ID] = created
	return cloneLLMProvider(created), nil
}

func (m *MemoryStore) GetLLMProvider(_ context.Context, id string) (*domain.LLMProvider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	provider, ok := m.llmProviders[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneLLMProvider(provider), nil
}

func (m *MemoryStore) UpdateLLMProvider(_ context.Context, p *domain.LLMProvider) (*domain.LLMProvider, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.llmProviders[p.ID]
	if !ok {
		return nil, ErrNotFound
	}
	for _, other := range m.llmProviders {
		if other.ID != p.ID && strings.EqualFold(other.Name, p.Name) {
			return nil, ErrConflict
		}
	}
	updated := cloneLLMProvider(p)
	updated.RegisteredBy = existing.RegisteredBy
	updated.CreatedAt = existing.CreatedAt
	m.llmProviders[p.ID] = updated
	return cloneLLMProvider(updated), nil
}

func (m *MemoryStore) DeprecateLLMProvider(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	provider, ok := m.llmProviders[id]
	if !ok {
		return ErrNotFound
	}
	provider.Status = domain.ResourceDeprecated
	return nil
}

func (m *MemoryStore) ListLLMProviders(_ context.Context) ([]*domain.LLMProvider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*domain.LLMProvider, 0, len(m.llmProviders))
	for _, provider := range m.llmProviders {
		out = append(out, cloneLLMProvider(provider))
	}
	slices.SortFunc(out, func(a, b *domain.LLMProvider) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

func (m *MemoryStore) CreateResource(_ context.Context, r *domain.RegistryResource) (*domain.RegistryResource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.resources {
		if existing.Type == r.Type && strings.EqualFold(existing.Name, r.Name) {
			return nil, ErrConflict
		}
	}
	created := cloneResource(r)
	created.ID = uuid.NewString()
	if created.Status == "" {
		created.Status = domain.ResourceActive
	}
	created.CreatedAt = time.Now().UTC()
	m.resources[created.ID] = created
	return cloneResource(created), nil
}

func (m *MemoryStore) GetResource(_ context.Context, typ domain.ResourceType, id string) (*domain.RegistryResource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resource, ok := m.resources[id]
	if !ok || resource.Type != typ {
		return nil, ErrNotFound
	}
	return cloneResource(resource), nil
}

func (m *MemoryStore) UpdateResource(_ context.Context, r *domain.RegistryResource) (*domain.RegistryResource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.resources[r.ID]
	if !ok || existing.Type != r.Type {
		return nil, ErrNotFound
	}
	for _, other := range m.resources {
		if other.ID != r.ID && other.Type == r.Type && strings.EqualFold(other.Name, r.Name) {
			return nil, ErrConflict
		}
	}
	updated := cloneResource(r)
	updated.Type = existing.Type
	updated.RegisteredBy = existing.RegisteredBy
	updated.CreatedAt = existing.CreatedAt
	m.resources[r.ID] = updated
	return cloneResource(updated), nil
}

func (m *MemoryStore) DeprecateResource(_ context.Context, typ domain.ResourceType, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	resource, ok := m.resources[id]
	if !ok || resource.Type != typ {
		return ErrNotFound
	}
	resource.Status = domain.ResourceDeprecated
	return nil
}

func (m *MemoryStore) ListResources(_ context.Context, typ domain.ResourceType) ([]*domain.RegistryResource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []*domain.RegistryResource{}
	for _, resource := range m.resources {
		if resource.Type == typ {
			out = append(out, cloneResource(resource))
		}
	}
	slices.SortFunc(out, func(a, b *domain.RegistryResource) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

func (m *MemoryStore) GrantAgentPermission(_ context.Context, p *domain.AgentPermission) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.agents[p.AgentID]; !ok {
		return ErrNotFound
	}
	key := permissionKey(p.AgentID, p.ResourceType, p.ResourceID)
	if _, ok := m.permissions[key]; ok {
		return nil
	}
	created := cloneAgentPermission(p)
	created.ID = uuid.NewString()
	created.CreatedAt = time.Now().UTC()
	m.permissions[key] = created
	return nil
}

func (m *MemoryStore) RevokeAgentPermission(_ context.Context, agentID string, typ domain.ResourceType, resourceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.agents[agentID]; !ok {
		return ErrNotFound
	}
	delete(m.permissions, permissionKey(agentID, typ, resourceID))
	return nil
}

func (m *MemoryStore) ListAgentPermissions(_ context.Context, agentID string) ([]*domain.AgentPermission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.agents[agentID]; !ok {
		return nil, ErrNotFound
	}
	out := []*domain.AgentPermission{}
	for _, perm := range m.permissions {
		if perm.AgentID == agentID {
			out = append(out, cloneAgentPermission(perm))
		}
	}
	slices.SortFunc(out, func(a, b *domain.AgentPermission) int {
		if a.ResourceType != b.ResourceType {
			return strings.Compare(string(a.ResourceType), string(b.ResourceType))
		}
		return strings.Compare(a.ResourceID, b.ResourceID)
	})
	return out, nil
}

func (m *MemoryStore) SetAgentPermissions(_ context.Context, agentID string, perms []domain.AgentPermission) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.agents[agentID]; !ok {
		return ErrNotFound
	}
	for key, perm := range m.permissions {
		if perm.AgentID == agentID {
			delete(m.permissions, key)
		}
	}
	now := time.Now().UTC()
	for _, perm := range perms {
		created := cloneAgentPermission(&perm)
		created.ID = uuid.NewString()
		created.AgentID = agentID
		created.CreatedAt = now
		m.permissions[permissionKey(agentID, created.ResourceType, created.ResourceID)] = created
	}
	return nil
}

func (m *MemoryStore) AgentHasPermission(_ context.Context, agentID string, typ domain.ResourceType, resourceID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.agents[agentID]; !ok {
		return false, ErrNotFound
	}
	_, ok := m.permissions[permissionKey(agentID, typ, resourceID)]
	return ok, nil
}

func (m *MemoryStore) RecordMetering(_ context.Context, event *domain.MeteringEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.agents[event.AgentID]; !ok {
		return ErrNotFound
	}
	if _, ok := m.squads[event.SquadID]; !ok {
		return ErrNotFound
	}
	created := cloneMeteringEvent(event)
	created.ID = uuid.NewString()
	if created.Currency == "" {
		created.Currency = "USD"
	}
	if created.Timestamp.IsZero() {
		created.Timestamp = time.Now().UTC()
	}
	m.metering[created.ID] = created
	return nil
}

func (m *MemoryStore) SumMetering(_ context.Context, squadID, agentID string) (*domain.MeteringEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := &domain.MeteringEvent{
		SquadID:  squadID,
		AgentID:  agentID,
		Currency: "USD",
	}
	for _, event := range m.metering {
		if squadID != "" && event.SquadID != squadID {
			continue
		}
		if agentID != "" && event.AgentID != agentID {
			continue
		}
		out.InputTokens += event.InputTokens
		out.OutputTokens += event.OutputTokens
		out.Cost += event.Cost
		if out.Timestamp.IsZero() || event.Timestamp.After(out.Timestamp) {
			out.Timestamp = event.Timestamp
		}
	}
	return out, nil
}

func (m *MemoryStore) RecordAudit(_ context.Context, entry *domain.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	created := cloneAuditEntry(entry)
	created.ID = uuid.NewString()
	if len(created.Metadata) == 0 {
		created.Metadata = []byte(`{}`)
	}
	if created.Timestamp.IsZero() {
		created.Timestamp = time.Now().UTC()
	}
	m.auditLog[created.ID] = created
	return nil
}

func (m *MemoryStore) ListAudit(_ context.Context, squadID string, limit int) ([]*domain.AuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	out := []*domain.AuditEntry{}
	for _, entry := range m.auditLog {
		if squadID == "" || entry.SquadID == squadID {
			out = append(out, cloneAuditEntry(entry))
		}
	}
	slices.SortFunc(out, func(a, b *domain.AuditEntry) int {
		if !a.Timestamp.Equal(b.Timestamp) {
			if a.Timestamp.After(b.Timestamp) {
				return -1
			}
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryStore) GetBoard(_ context.Context, squadID string) (*domain.Board, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	boardID, ok := m.boardsBySquad[squadID]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneBoard(m.boards[boardID]), nil
}

func (m *MemoryStore) CreateTask(_ context.Context, t *domain.Task) (*domain.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.boards[t.BoardID]; !ok {
		return nil, ErrNotFound
	}
	now := time.Now().UTC()
	created := cloneTask(t)
	created.ID = uuid.NewString()
	if created.Status == "" {
		created.Status = domain.TaskTodo
	}
	created.Position = m.nextTaskPosition(t.BoardID, created.Status)
	created.CreatedAt = now
	created.UpdatedAt = now
	m.tasks[created.ID] = created
	return cloneTask(created), nil
}

func (m *MemoryStore) GetTask(_ context.Context, id string) (*domain.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneTask(t), nil
}

func (m *MemoryStore) UpdateTask(_ context.Context, t *domain.Task) (*domain.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.tasks[t.ID]
	if !ok {
		return nil, ErrNotFound
	}
	updated := cloneTask(t)
	updated.BoardID = existing.BoardID
	updated.SquadID = existing.SquadID
	updated.CreatedByType = existing.CreatedByType
	updated.CreatedByID = existing.CreatedByID
	updated.CreatedAt = existing.CreatedAt
	if updated.Status != existing.Status {
		updated.Position = m.nextTaskPosition(existing.BoardID, updated.Status)
	}
	updated.UpdatedAt = time.Now().UTC()
	m.tasks[t.ID] = updated
	return cloneTask(updated), nil
}

func (m *MemoryStore) DeleteTask(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tasks[id]; !ok {
		return ErrNotFound
	}
	delete(m.tasks, id)
	return nil
}

func (m *MemoryStore) ListTasks(_ context.Context, boardID string, status domain.TaskStatus) ([]*domain.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []*domain.Task{}
	for _, t := range m.tasks {
		if t.BoardID == boardID && (status == "" || t.Status == status) {
			out = append(out, cloneTask(t))
		}
	}
	slices.SortFunc(out, func(a, b *domain.Task) int {
		if a.Status != b.Status {
			return strings.Compare(string(a.Status), string(b.Status))
		}
		return a.Position - b.Position
	})
	return out, nil
}

func (m *MemoryStore) nextTaskPosition(boardID string, status domain.TaskStatus) int {
	next := 1
	for _, task := range m.tasks {
		if task.BoardID == boardID && task.Status == status && task.Position >= next {
			next = task.Position + 1
		}
	}
	return next
}

func cloneUser(u *domain.User) *domain.User {
	if u == nil {
		return nil
	}
	v := *u
	return &v
}

func cloneSquad(s *domain.Squad) *domain.Squad {
	if s == nil {
		return nil
	}
	v := *s
	v.OperatingModel = slices.Clone(s.OperatingModel)
	return &v
}

func cloneAgent(a *domain.Agent) *domain.Agent {
	if a == nil {
		return nil
	}
	v := *a
	v.Permissions = slices.Clone(a.Permissions)
	return &v
}

func cloneAgentIdentity(i *domain.AgentIdentity) *domain.AgentIdentity {
	if i == nil {
		return nil
	}
	v := *i
	return &v
}

func cloneBoard(b *domain.Board) *domain.Board {
	if b == nil {
		return nil
	}
	v := *b
	return &v
}

func cloneTask(t *domain.Task) *domain.Task {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}

func cloneGrant(g *domain.AccessGrant) *domain.AccessGrant {
	if g == nil {
		return nil
	}
	v := *g
	return &v
}

func cloneLLMProvider(p *domain.LLMProvider) *domain.LLMProvider {
	if p == nil {
		return nil
	}
	v := *p
	v.Models = slices.Clone(p.Models)
	v.Pricing = slices.Clone(p.Pricing)
	return &v
}

func cloneResource(r *domain.RegistryResource) *domain.RegistryResource {
	if r == nil {
		return nil
	}
	v := *r
	v.Manifest = slices.Clone(r.Manifest)
	return &v
}

func cloneAgentPermission(p *domain.AgentPermission) *domain.AgentPermission {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneMeteringEvent(event *domain.MeteringEvent) *domain.MeteringEvent {
	if event == nil {
		return nil
	}
	v := *event
	return &v
}

func cloneAuditEntry(entry *domain.AuditEntry) *domain.AuditEntry {
	if entry == nil {
		return nil
	}
	v := *entry
	v.Metadata = slices.Clone(entry.Metadata)
	return &v
}

func permissionKey(agentID string, typ domain.ResourceType, resourceID string) string {
	return agentID + ":" + string(typ) + ":" + resourceID
}
