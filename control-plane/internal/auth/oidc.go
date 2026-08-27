// Package auth validates external principals for the control-plane API.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

// ErrUnauthorized is returned for missing or invalid credentials.
var ErrUnauthorized = errors.New("auth: unauthorized")

// Profile is the normalized human user profile extracted from an OIDC token.
type Profile struct {
	Email string
	Name  string
}

// OIDCAuthenticator validates OIDC bearer tokens.
type OIDCAuthenticator struct {
	verifier *oidc.IDTokenVerifier
}

// NewOIDCAuthenticator builds an OIDC verifier from issuer discovery.
func NewOIDCAuthenticator(ctx context.Context, issuerURL, audience string) (*OIDCAuthenticator, error) {
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc: discover provider: %w", err)
	}
	return &OIDCAuthenticator{
		verifier: provider.Verifier(&oidc.Config{ClientID: audience}),
	}, nil
}

// Authenticate validates an Authorization header and returns normalized claims.
func (a *OIDCAuthenticator) Authenticate(ctx context.Context, authorization string) (*Profile, error) {
	token, ok := bearerToken(authorization)
	if !ok {
		return nil, ErrUnauthorized
	}

	idToken, err := a.verifier.Verify(ctx, token)
	if err != nil {
		return nil, ErrUnauthorized
	}

	var claims struct {
		Email             string `json:"email"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, ErrUnauthorized
	}
	claims.Email = strings.TrimSpace(strings.ToLower(claims.Email))
	if claims.Email == "" {
		return nil, ErrUnauthorized
	}
	name := strings.TrimSpace(claims.Name)
	if name == "" {
		name = strings.TrimSpace(claims.PreferredUsername)
	}
	if name == "" {
		name = claims.Email
	}
	return &Profile{Email: claims.Email, Name: name}, nil
}

func bearerToken(authorization string) (string, bool) {
	typ, token, ok := strings.Cut(strings.TrimSpace(authorization), " ")
	if !ok || !strings.EqualFold(typ, "Bearer") || strings.TrimSpace(token) == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}
