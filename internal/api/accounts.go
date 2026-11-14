package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/brianmcgee/rmhub/internal/accounts"
	"github.com/brianmcgee/rmhub/internal/devices"
	"github.com/brianmcgee/rmhub/internal/pairing"
	"github.com/brianmcgee/rmhub/internal/tokens"
)

// Service holds the dependencies of the account endpoints.
type Service struct {
	Accounts *accounts.Store
	Tokens   *tokens.Store
	Devices  *devices.Registry
	Pairing  *pairing.Store
	// Admin checks the admin token. Nil means that no admin token is
	// configured, and each admin request gets status 401.
	Admin *tokens.Admin
	Log   *slog.Logger
}

// Kinds returns the token kinds of the service.
func (s *Service) Kinds() []TokenKind {
	return []TokenKind{adminKind{s.Admin}, ownerKind{s.Tokens}}
}

type adminKind struct{ admin *tokens.Admin }

func (adminKind) Match(tok string) bool { return tokens.IsAdmin(tok) }

func (k adminKind) Authenticate(_ context.Context, tok string) (*Principal, error) {
	if k.admin == nil || !k.admin.Check(tok) {
		return nil, Errorf(CodeUnauthorized, "the token is not valid")
	}
	return &Principal{Admin: true}, nil
}

type ownerKind struct{ tokens *tokens.Store }

func (ownerKind) Match(tok string) bool {
	_, _, _, ok := tokens.Parse(tok)
	return ok
}

func (k ownerKind) Authenticate(ctx context.Context, tok string) (*Principal, error) {
	acct, id, err := k.tokens.Check(ctx, tok)
	if errors.Is(err, tokens.ErrInvalid) {
		return nil, Errorf(CodeUnauthorized, "the token is not valid, was revoked or expired")
	}
	if err != nil {
		return nil, err
	}
	return &Principal{Account: acct, TokenID: id}, nil
}

// Register adds the account endpoints to the mux. Each endpoint requires
// a bearer token.
func (s *Service) Register(m *Mux) {
	auth := func(h HandlerFunc) HandlerFunc { return Auth(s.Kinds(), h) }
	admin := func(h HandlerFunc) HandlerFunc {
		return auth(func(w http.ResponseWriter, r *http.Request) error {
			if !PrincipalFrom(r.Context()).Admin {
				return Errorf(CodeForbidden, "this endpoint needs the admin token")
			}
			return h(w, r)
		})
	}
	m.Handle("POST /api/v1/admin/users", admin(s.createUser))
	m.Handle("GET /api/v1/admin/users", admin(s.listUsers))
	m.Handle("GET /api/v1/admin/users/{id}", admin(s.getUser))
	m.Handle("POST /api/v1/admin/users/{id}/tokens", admin(s.issueToken))

	m.Handle("GET /api/v1/account", auth(Owner(s.getAccount)))
	m.Handle("GET /api/v1/account/tokens", auth(Owner(s.listTokens)))
	m.Handle("POST /api/v1/account/tokens", auth(Owner(s.createToken)))
	m.Handle("DELETE /api/v1/account/tokens/{id}", auth(Owner(s.revokeToken)))
	m.Handle("POST /api/v1/account/pairing-codes", auth(Owner(s.createPairingCode)))
	m.Handle("GET /api/v1/account/devices", auth(Owner(s.listDevices)))
	m.Handle("DELETE /api/v1/account/devices/{id}", auth(Owner(s.removeDevice)))
}

// Owner refuses each principal that is not an owner token with code
// forbidden.
func Owner(h HandlerFunc) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p := PrincipalFrom(r.Context())
		if p == nil || p.Account == "" {
			return Errorf(CodeForbidden, "this endpoint needs an owner token")
		}
		return h(w, r)
	}
}

type userView struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

func viewOf(a accounts.Account) userView { return userView{ID: a.ID, Email: a.Email, Name: a.Name} }

type createUserRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func (s *Service) createUser(w http.ResponseWriter, r *http.Request) error {
	var req createUserRequest
	if err := DecodeJSON(r, &req); err != nil {
		return err
	}
	a, err := s.Accounts.Create(r.Context(), req.Email, req.Name)
	switch {
	case errors.Is(err, accounts.ErrInvalidEmail):
		return Errorf(CodeBadRequest, "the field email must hold an email address")
	case errors.Is(err, accounts.ErrEmailInUse):
		return Errorf(CodeConflict, "an account with this email address exists")
	case err != nil:
		return err
	}
	s.Log.Info("account created", "account", a.ID)
	WriteJSON(w, http.StatusCreated, viewOf(a))
	return nil
}

func (s *Service) listUsers(w http.ResponseWriter, r *http.Request) error {
	list, err := s.Accounts.List(r.Context())
	if err != nil {
		return err
	}
	out := make([]userView, len(list))
	for i, a := range list {
		out[i] = viewOf(a)
	}
	WriteJSON(w, http.StatusOK, out)
	return nil
}

func (s *Service) account(ctx context.Context, id string) (accounts.Account, error) {
	a, err := s.Accounts.Get(ctx, id)
	if errors.Is(err, accounts.ErrNotFound) {
		return a, Errorf(CodeNotFound, "account %s does not exist", id)
	}
	return a, err
}

func (s *Service) getUser(w http.ResponseWriter, r *http.Request) error {
	a, err := s.account(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusOK, viewOf(a))
	return nil
}

type issueRequest struct {
	Label     string     `json:"label"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

type issueResponse struct {
	ID        string     `json:"id"`
	Token     string     `json:"token"`
	Label     string     `json:"label"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

func (s *Service) issue(w http.ResponseWriter, r *http.Request, account string) error {
	var req issueRequest
	if err := DecodeJSON(r, &req); err != nil {
		return err
	}
	text, tok, err := s.Tokens.Issue(r.Context(), account, req.Label, req.ExpiresAt)
	if err != nil {
		return err
	}
	s.Log.Info("owner token issued", "account", account, "token", tok.ID)
	WriteJSON(w, http.StatusCreated, issueResponse{ID: tok.ID, Token: text, Label: tok.Label, ExpiresAt: tok.ExpiresAt})
	return nil
}

func (s *Service) issueToken(w http.ResponseWriter, r *http.Request) error {
	a, err := s.account(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	return s.issue(w, r, a.ID)
}

func (s *Service) createToken(w http.ResponseWriter, r *http.Request) error {
	return s.issue(w, r, PrincipalFrom(r.Context()).Account)
}

func (s *Service) getAccount(w http.ResponseWriter, r *http.Request) error {
	a, err := s.account(r.Context(), PrincipalFrom(r.Context()).Account)
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusOK, viewOf(a))
	return nil
}

func (s *Service) listTokens(w http.ResponseWriter, r *http.Request) error {
	list, err := s.Tokens.List(r.Context(), PrincipalFrom(r.Context()).Account)
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusOK, list)
	return nil
}

func (s *Service) revokeToken(w http.ResponseWriter, r *http.Request) error {
	acct, id := PrincipalFrom(r.Context()).Account, r.PathValue("id")
	err := s.Tokens.Revoke(r.Context(), acct, id)
	if errors.Is(err, tokens.ErrNotFound) {
		return Errorf(CodeNotFound, "token %s does not exist", id)
	}
	if err != nil {
		return err
	}
	s.Log.Info("owner token revoked", "account", acct, "token", id)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Service) createPairingCode(w http.ResponseWriter, r *http.Request) error {
	c, err := s.Pairing.Create(r.Context(), PrincipalFrom(r.Context()).Account)
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusCreated, c)
	return nil
}

func (s *Service) listDevices(w http.ResponseWriter, r *http.Request) error {
	list, err := s.Devices.List(r.Context(), PrincipalFrom(r.Context()).Account)
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusOK, list)
	return nil
}

func (s *Service) removeDevice(w http.ResponseWriter, r *http.Request) error {
	acct, id := PrincipalFrom(r.Context()).Account, r.PathValue("id")
	err := s.Devices.Remove(r.Context(), acct, id)
	if errors.Is(err, devices.ErrNotFound) {
		return Errorf(CodeNotFound, "device %s does not exist", id)
	}
	if err != nil {
		return err
	}
	s.Log.Info("device removed", "account", acct, "device", id)
	w.WriteHeader(http.StatusNoContent)
	return nil
}
