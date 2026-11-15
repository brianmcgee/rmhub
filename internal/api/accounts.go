package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/brianmcgee/rmhub/internal/accounts"
	"github.com/brianmcgee/rmhub/internal/devices"
	"github.com/brianmcgee/rmhub/internal/grants"
	"github.com/brianmcgee/rmhub/internal/library"
	"github.com/brianmcgee/rmhub/internal/pairing"
	"github.com/brianmcgee/rmhub/internal/ratelimit"
	"github.com/brianmcgee/rmhub/internal/tokens"
)

// Service holds the dependencies of the account endpoints.
type Service struct {
	Accounts *accounts.Store
	Tokens   *tokens.Store
	Devices  *devices.Registry
	Pairing  *pairing.Store
	// Library checks the grant folders of new scoped tokens.
	Library *library.Library
	// Admin checks the admin token. Nil means that no admin token is
	// configured, and each admin request gets status 401.
	Admin *tokens.Admin
	// Limit limits the requests of each token. Nil means no limit.
	Limit *ratelimit.Limiter
	Log   *slog.Logger
}

// Kinds returns the token kinds of the service.
func (s *Service) Kinds() []TokenKind {
	return []TokenKind{adminKind{s.Admin, s.Limit}, ownerKind{s.Tokens, s.Limit}}
}

// requestsOfToken names the request limit in the error message.
const requestsOfToken = "API requests of this token"

type adminKind struct {
	admin *tokens.Admin
	limit *ratelimit.Limiter
}

func (adminKind) Match(tok string) bool { return tokens.IsAdmin(tok) }

func (k adminKind) Authenticate(_ context.Context, tok string) (*Principal, error) {
	if k.admin == nil || !k.admin.Check(tok) {
		return nil, Errorf(CodeUnauthorized, "the token is not valid")
	}
	// The admin token has no ID. A token ID cannot be this key.
	if err := take(k.limit, "admin", requestsOfToken); err != nil {
		return nil, err
	}
	return &Principal{Admin: true}, nil
}

type ownerKind struct {
	tokens *tokens.Store
	limit  *ratelimit.Limiter
}

func (ownerKind) Match(tok string) bool {
	_, _, _, ok := tokens.Parse(tok)
	return ok
}

func (k ownerKind) Authenticate(ctx context.Context, tok string) (*Principal, error) {
	acct, chain, err := k.tokens.Check(ctx, tok)
	if errors.Is(err, tokens.ErrInvalid) {
		return nil, Errorf(CodeUnauthorized, "the token is not valid, was revoked or expired")
	}
	if err != nil {
		return nil, err
	}
	// The limit comes after the token check, so that a request with a
	// token that is not valid cannot use the bucket of a valid token.
	// The key is the ID of the token itself: a child token does not
	// use the bucket of its parent.
	if err := take(k.limit, acct+"/"+chain[0].ID, requestsOfToken); err != nil {
		return nil, err
	}
	return &Principal{Account: acct, TokenID: chain[0].ID, Chain: chain}, nil
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
	m.Handle("PATCH /api/v1/account", auth(Owner(s.patchAccount)))
	m.Handle("GET /api/v1/account/tokens", auth(AccountToken(s.listTokens)))
	m.Handle("POST /api/v1/account/tokens", auth(AccountToken(s.createToken)))
	m.Handle("DELETE /api/v1/account/tokens/{id}", auth(AccountToken(s.revokeToken)))
	m.Handle("POST /api/v1/account/pairing-codes", auth(Owner(s.createPairingCode)))
	m.Handle("GET /api/v1/account/devices", auth(Owner(s.listDevices)))
	m.Handle("DELETE /api/v1/account/devices/{id}", auth(Owner(s.removeDevice)))
}

// Owner refuses each principal that is not an owner token with code
// forbidden. A scoped token is not an owner token.
func Owner(h HandlerFunc) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p := PrincipalFrom(r.Context())
		if p == nil || p.Account == "" || p.Scoped() {
			return Errorf(CodeForbidden, "this endpoint needs an owner token")
		}
		return h(w, r)
	}
}

// AccountToken refuses each principal that is not an owner token or a
// scoped token with code forbidden.
func AccountToken(h HandlerFunc) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p := PrincipalFrom(r.Context())
		if p == nil || p.Account == "" {
			return Errorf(CodeForbidden, "this endpoint needs an account token")
		}
		return h(w, r)
	}
}

type userView struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	SchemaVersion int    `json:"schemaVersion"`
}

func viewOf(a accounts.Account) userView {
	return userView{ID: a.ID, Email: a.Email, Name: a.Name, SchemaVersion: a.SchemaVersion}
}

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

type createTokenRequest struct {
	Label     string         `json:"label"`
	ExpiresAt *time.Time     `json:"expiresAt"`
	Grants    []tokens.Grant `json:"grants"`
}

type issueResponse struct {
	ID        string         `json:"id"`
	Token     string         `json:"token"`
	Label     string         `json:"label"`
	ExpiresAt *time.Time     `json:"expiresAt"`
	Parent    string         `json:"parent,omitempty"`
	Grants    []tokens.Grant `json:"grants,omitempty"`
}

func (s *Service) issue(w http.ResponseWriter, ctx context.Context, account string, spec tokens.Spec) error {
	text, tok, err := s.Tokens.Issue(ctx, account, spec)
	if err != nil {
		return err
	}
	s.Log.Info("account token issued", "account", account, "token", tok.ID, "parent", tok.Parent, "grants", len(tok.Grants))
	WriteJSON(w, http.StatusCreated, issueResponse{
		ID: tok.ID, Token: text, Label: tok.Label, ExpiresAt: tok.ExpiresAt,
		Parent: tok.Parent, Grants: tok.Grants,
	})
	return nil
}

// issueToken makes an owner token for a user. The admin cannot make a
// scoped token, because a scoped token needs a parent token.
func (s *Service) issueToken(w http.ResponseWriter, r *http.Request) error {
	a, err := s.account(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	var req issueRequest
	if err := DecodeJSON(r, &req); err != nil {
		return err
	}
	return s.issue(w, r.Context(), a.ID, tokens.Spec{Label: req.Label, ExpiresAt: req.ExpiresAt})
}

// createToken makes an owner token, or a scoped token if the request has
// grants. A scoped principal can make only a scoped child token with
// fewer rights.
func (s *Service) createToken(w http.ResponseWriter, r *http.Request) error {
	p := PrincipalFrom(r.Context())
	var req createTokenRequest
	if err := DecodeJSON(r, &req); err != nil {
		return err
	}
	spec := tokens.Spec{Label: req.Label, ExpiresAt: req.ExpiresAt}
	if !p.Scoped() && len(req.Grants) == 0 {
		return s.issue(w, r.Context(), p.Account, spec)
	}
	if len(req.Grants) == 0 {
		return Errorf(CodeForbidden, "a scoped token can make only a token with grants")
	}
	for _, g := range req.Grants {
		if g.Access != tokens.AccessRead && g.Access != tokens.AccessWrite {
			return Errorf(CodeBadRequest, "the access of a grant must be read or write")
		}
	}
	if p.Scoped() {
		if err := checkChild(p, &spec); err != nil {
			return err
		}
	}
	snap, err := s.Library.Current(r.Context(), p.Account)
	if err != nil {
		return err
	}
	if sc := grants.FromChain(p.Chain); sc != nil {
		for _, g := range req.Grants {
			if !sc.Visible(snap, g.Folder) || !sc.Covers(snap, g) {
				return Errorf(CodeForbidden, "the grant on %s is not inside a grant of the token with the same or higher access", g.Folder)
			}
		}
	}
	for _, g := range req.Grants {
		d, ok := snap.Doc(g.Folder)
		if !ok || d.Type != library.TypeFolder || d.Trashed {
			return Errorf(CodeBadRequest, "the grant folder %s is not an existing folder outside the trash", g.Folder)
		}
	}
	spec.Parent = p.TokenID
	spec.Grants = req.Grants
	return s.issue(w, r.Context(), p.Account, spec)
}

// checkChild applies the chain length and the expiry rules to a child of
// the scoped principal. A child without an expiry gets the expiry of the
// parent.
func checkChild(p *Principal, spec *tokens.Spec) error {
	scoped := 0
	for _, t := range p.Chain {
		if t.Scoped() {
			scoped++
		}
	}
	if scoped >= tokens.MaxScoped {
		return Errorf(CodeForbidden, "a chain of tokens can have at most %d scoped tokens", tokens.MaxScoped)
	}
	parent := p.Chain[0].ExpiresAt
	if spec.ExpiresAt == nil {
		spec.ExpiresAt = parent
		return nil
	}
	if parent != nil && spec.ExpiresAt.After(*parent) {
		return Errorf(CodeForbidden, "the expiry of a child token must not be later than %s", parent.Format(time.RFC3339))
	}
	return nil
}

func (s *Service) getAccount(w http.ResponseWriter, r *http.Request) error {
	a, err := s.account(r.Context(), PrincipalFrom(r.Context()).Account)
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusOK, viewOf(a))
	return nil
}

type patchAccountRequest struct {
	SchemaVersion *int `json:"schemaVersion"`
}

func (s *Service) patchAccount(w http.ResponseWriter, r *http.Request) error {
	var req patchAccountRequest
	if err := DecodeJSON(r, &req); err != nil {
		return err
	}
	acct := PrincipalFrom(r.Context()).Account
	a, err := s.account(r.Context(), acct)
	if err != nil {
		return err
	}
	if req.SchemaVersion != nil {
		a, err = s.Accounts.SetSchema(r.Context(), acct, *req.SchemaVersion)
		if errors.Is(err, accounts.ErrInvalidSchema) {
			return Errorf(CodeBadRequest, "the field schemaVersion must be 3 or 4")
		}
		if err != nil {
			return err
		}
		s.Log.Info("announced schema changed", "account", acct, "schema", a.SchemaVersion)
	}
	WriteJSON(w, http.StatusOK, viewOf(a))
	return nil
}

// visibleTokens returns the tokens that the principal can manage: each
// token of the account for an owner token, and the descendants for a
// scoped token.
func (s *Service) visibleTokens(ctx context.Context, p *Principal) ([]tokens.Token, error) {
	list, err := s.Tokens.List(ctx, p.Account)
	if err != nil || !p.Scoped() {
		return list, err
	}
	return tokens.Descendants(list, p.TokenID), nil
}

func (s *Service) listTokens(w http.ResponseWriter, r *http.Request) error {
	list, err := s.visibleTokens(r.Context(), PrincipalFrom(r.Context()))
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusOK, list)
	return nil
}

func (s *Service) revokeToken(w http.ResponseWriter, r *http.Request) error {
	p := PrincipalFrom(r.Context())
	acct, id := p.Account, r.PathValue("id")
	if p.Scoped() {
		list, err := s.visibleTokens(r.Context(), p)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(list, func(t tokens.Token) bool { return t.ID == id }) {
			return Errorf(CodeNotFound, "token %s does not exist", id)
		}
	}
	err := s.Tokens.Revoke(r.Context(), acct, id)
	if errors.Is(err, tokens.ErrNotFound) {
		return Errorf(CodeNotFound, "token %s does not exist", id)
	}
	if err != nil {
		return err
	}
	s.Log.Info("account token revoked", "account", acct, "token", id)
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
