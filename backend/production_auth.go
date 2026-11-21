package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
)

const (
	productionRoleAdmin    = "platform_admin"
	productionRoleChecker  = "platform_checker"
	productionRoleMerchant = "merchant"
	productionSessionName  = "__Host-payguard_session"
	productionOIDCName     = "__Host-payguard_oidc"
	productionLoginTTL     = 10 * time.Minute
	productionSessionTTL   = 8 * time.Hour
)

type productionActor struct {
	ID         string `json:"-"`
	Role       string `json:"role"`
	MerchantID string `json:"merchantId,omitempty"`
}

type productionActorKey struct{}

func productionActorFromRequest(r *http.Request) (productionActor, bool) {
	return productionActorFromContext(r.Context())
}

func productionActorFromContext(ctx context.Context) (productionActor, bool) {
	actor, ok := ctx.Value(productionActorKey{}).(productionActor)
	return actor, ok
}

type productionAuthConfig struct {
	issuer       string
	clientID     string
	clientSecret string
	origin       string
}

type productionAuth struct {
	store    productionAuthStore
	issuer   string
	origin   string
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	client   *http.Client
}

func newProductionAuth(ctx context.Context, db *pgxpool.Pool) (*productionAuth, error) {
	switch os.Getenv("PAYGUARD_AUTH_MODE") {
	case "", "sandbox":
		return nil, nil
	case "oidc":
	default:
		return nil, errors.New("PAYGUARD_AUTH_MODE must be sandbox or oidc")
	}
	config := productionAuthConfig{
		issuer: os.Getenv("PAYGUARD_OIDC_ISSUER"), clientID: os.Getenv("PAYGUARD_OIDC_CLIENT_ID"),
		clientSecret: os.Getenv("PAYGUARD_OIDC_CLIENT_SECRET"), origin: os.Getenv("PAYGUARD_PUBLIC_ORIGIN"),
	}
	auth, err := configureProductionAuth(ctx, productionAuthStore{db: db}, config, &http.Client{Timeout: 10 * time.Second})
	if err == nil {
		go auth.store.pruneExpired(ctx)
	}
	return auth, err
}

func productionHTTPSURL(raw string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	return u, err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil &&
		!strings.Contains(raw, "#") && u.Opaque == ""
}

func configureProductionAuth(ctx context.Context, store productionAuthStore, config productionAuthConfig, client *http.Client) (*productionAuth, error) {
	origin, valid := productionHTTPSURL(config.origin)
	if !valid || config.origin != "https://"+origin.Host {
		return nil, errors.New("PAYGUARD_PUBLIC_ORIGIN must be an HTTPS origin without a path, query, or fragment")
	}
	issuer, valid := productionHTTPSURL(config.issuer)
	if !valid || issuer.RawQuery != "" || issuer.ForceQuery || strings.TrimSpace(config.clientID) == "" || strings.TrimSpace(config.clientSecret) == "" {
		return nil, errors.New("OIDC requires an HTTPS issuer, client ID and client secret")
	}
	// Authorization codes, PKCE verifiers and client secrets stay at the discovered token endpoint.
	noRedirects := *client
	noRedirects.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	ctx = oidc.ClientContext(ctx, &noRedirects)
	provider, err := oidc.NewProvider(ctx, config.issuer)
	if err != nil {
		return nil, errors.New("OIDC discovery failed for the configured issuer")
	}
	var metadata struct {
		JWKSURL string   `json:"jwks_uri"`
		PKCE    []string `json:"code_challenge_methods_supported"`
	}
	endpoint := provider.Endpoint()
	if err := provider.Claims(&metadata); err != nil {
		return nil, errors.New("OIDC discovery metadata is invalid")
	}
	for _, raw := range []string{endpoint.AuthURL, endpoint.TokenURL, metadata.JWKSURL} {
		if _, valid := productionHTTPSURL(raw); !valid {
			return nil, errors.New("OIDC authorization, token and key endpoints must use HTTPS")
		}
	}
	if len(metadata.PKCE) > 0 && !slices.Contains(metadata.PKCE, "S256") {
		return nil, errors.New("OIDC provider must support S256 PKCE")
	}
	return &productionAuth{
		store: store, issuer: config.issuer, origin: config.origin, client: &noRedirects,
		oauth: oauth2.Config{ClientID: config.clientID, ClientSecret: config.clientSecret, Endpoint: endpoint,
			RedirectURL: config.origin + "/api/auth/callback", Scopes: []string{oidc.ScopeOpenID}},
		verifier: provider.Verifier(&oidc.Config{ClientID: config.clientID}),
	}, nil
}

func (a *productionAuth) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auth/login", a.login)
	mux.HandleFunc("GET /api/auth/callback", a.callback)
	mux.HandleFunc("GET /api/auth/session", a.session)
	mux.HandleFunc("POST /api/auth/logout", a.logout)
}

func authResponseHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func authCookie(name, value string, ttl time.Duration) *http.Cookie {
	maxAge := -1
	if ttl > 0 {
		maxAge = int(ttl.Seconds())
	}
	return &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: maxAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}
}

func randomAuthValue() string {
	var value [32]byte
	// crypto/rand.Read fills the entire buffer or terminates the process on entropy failure.
	rand.Read(value[:])
	return base64.RawURLEncoding.EncodeToString(value[:])
}

func authHash(value string) []byte {
	if len(value) != base64.RawURLEncoding.EncodedLen(32) {
		return nil
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return nil
	}
	digest := sha256.Sum256([]byte(value))
	return digest[:]
}

func authCookieHash(r *http.Request, name string) []byte {
	cookies := r.CookiesNamed(name)
	if len(cookies) != 1 {
		return nil
	}
	return authHash(cookies[0].Value)
}

func (a *productionAuth) sameOrigin(r *http.Request) bool {
	origins := r.Header.Values("Origin")
	return len(origins) == 1 && origins[0] == a.origin
}

type authResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *authResponseWriter) WriteHeader(status int) {
	if status >= 200 && w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *authResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

func (w *authResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (a *productionAuth) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz", "/api/auth/login", "/api/auth/callback", "/api/auth/session", "/api/auth/logout":
			next.ServeHTTP(w, r)
			return
		}
		authResponseHeaders(w)
		write := r.Method != http.MethodGet && r.Method != http.MethodHead
		originOK := !write || a.sameOrigin(r)
		response := &authResponseWriter{ResponseWriter: w}
		actor, ok := a.requireSession(response, r, originOK)
		defer func() {
			if write || response.status >= http.StatusBadRequest {
				pattern := r.Pattern
				if pattern == "" {
					if mux, ok := next.(*http.ServeMux); ok {
						_, pattern = mux.Handler(r)
					}
				}
				slog.Info("production API access", "actor_id", actor.ID, "role", actor.Role,
					"merchant_id", actor.MerchantID, "method", r.Method, "route", pattern, "status", response.status)
			}
		}()
		if !ok {
			return
		}
		if !originOK {
			a.audit(r.Context(), actor, "request_denied", "origin")
			problem(response, http.StatusForbidden, "The request must come from the PayGuard origin.")
			return
		}
		// Existing sandbox bearer-token readers never act on browser-supplied credentials in OIDC mode.
		r.Header.Del("Authorization")
		ctx := context.WithValue(r.Context(), productionActorKey{}, actor)
		r = r.WithContext(ctx)
		next.ServeHTTP(response, r)
	})
}

func (a *productionAuth) login(w http.ResponseWriter, r *http.Request) {
	authResponseHeaders(w)
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		problem(w, http.StatusMethodNotAllowed, "Use GET to start sign-in.")
		return
	}
	state, browser, nonce := randomAuthValue(), randomAuthValue(), randomAuthValue()
	verifier := oauth2.GenerateVerifier()
	if err := a.store.begin(r.Context(), authHash(state), authHash(browser), nonce, verifier); err != nil {
		a.unavailable(w, "login_start")
		return
	}
	http.SetCookie(w, authCookie(productionOIDCName, browser, productionLoginTTL))
	http.Redirect(w, r, a.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusSeeOther)
}

func (a *productionAuth) callback(w http.ResponseWriter, r *http.Request) {
	authResponseHeaders(w)
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		problem(w, http.StatusMethodNotAllowed, "Use GET for the sign-in callback.")
		return
	}
	state := authHash(r.URL.Query().Get("state"))
	browser := authCookieHash(r, productionOIDCName)
	http.SetCookie(w, authCookie(productionOIDCName, "", 0))
	if state == nil || browser == nil || len(r.URL.Query()["state"]) != 1 {
		a.denyLogin(w, r, "flow")
		return
	}
	nonce, verifier, err := a.store.consume(r.Context(), state, browser)
	if errors.Is(err, pgx.ErrNoRows) {
		a.denyLogin(w, r, "flow")
		return
	}
	if err != nil {
		a.unavailable(w, "login_consume")
		return
	}
	query := r.URL.Query()
	code, issuer := query.Get("code"), query.Get("iss")
	if query.Has("error") || len(query["code"]) != 1 || code == "" || len(code) > 4096 ||
		(query.Has("iss") && (len(query["iss"]) != 1 || issuer != a.issuer)) {
		a.denyLogin(w, r, "provider_response")
		return
	}
	subject, err := a.exchange(r.Context(), code, verifier, nonce)
	if err != nil {
		a.denyLogin(w, r, "identity_verification")
		return
	}
	session := randomAuthValue()
	actor, err := a.store.issue(r.Context(), a.issuer, subject, authHash(session), authCookieHash(r, productionSessionName))
	if errors.Is(err, pgx.ErrNoRows) {
		a.denyLogin(w, r, "identity_not_provisioned")
		return
	}
	if err != nil {
		a.unavailable(w, "session_issue")
		return
	}
	http.SetCookie(w, authCookie(productionSessionName, session, productionSessionTTL))
	destination := "/settings/merchants"
	if actor.Role == productionRoleChecker {
		destination = "/settings/system-rules"
	}
	http.Redirect(w, r, destination, http.StatusSeeOther)
}

func (a *productionAuth) exchange(ctx context.Context, code, verifier, nonce string) (string, error) {
	ctx = oidc.ClientContext(ctx, a.client)
	token, err := a.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return "", err
	}
	rawIDToken, _ := token.Extra("id_token").(string)
	id, err := a.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return "", err
	}
	var claims struct {
		AuthorizedParty string `json:"azp"`
	}
	if err := id.Claims(&claims); err != nil {
		return "", err
	}
	if id.Subject == "" || len(id.Subject) > 255 || id.IssuedAt.IsZero() || id.IssuedAt.After(time.Now().Add(time.Minute)) ||
		subtle.ConstantTimeCompare([]byte(id.Nonce), []byte(nonce)) != 1 ||
		((len(id.Audience) > 1 || claims.AuthorizedParty != "") && claims.AuthorizedParty != a.oauth.ClientID) {
		return "", errors.New("OIDC ID token claims are invalid")
	}
	if id.AccessTokenHash != "" && id.VerifyAccessToken(token.AccessToken) != nil {
		return "", errors.New("OIDC access token hash is invalid")
	}
	return id.Subject, nil
}

func (a *productionAuth) requireSession(w http.ResponseWriter, r *http.Request, touch bool) (productionActor, bool) {
	hash := authCookieHash(r, productionSessionName)
	if hash != nil {
		actor, err := a.store.actor(r.Context(), hash, touch)
		if err == nil {
			return actor, true
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			a.unavailable(w, "session_lookup")
			return productionActor{}, false
		}
	}
	problem(w, http.StatusUnauthorized, "Sign in to PayGuard.")
	return productionActor{}, false
}

func (a *productionAuth) session(w http.ResponseWriter, r *http.Request) {
	authResponseHeaders(w)
	if actor, ok := a.requireSession(w, r, false); ok {
		respond(w, http.StatusOK, actor)
	}
}

func (a *productionAuth) logout(w http.ResponseWriter, r *http.Request) {
	authResponseHeaders(w)
	if !a.sameOrigin(r) {
		problem(w, http.StatusForbidden, "The request must come from the PayGuard origin.")
		return
	}
	if err := a.store.revoke(r.Context(), authCookieHash(r, productionSessionName)); err != nil {
		a.unavailable(w, "session_revoke")
		return
	}
	http.SetCookie(w, authCookie(productionSessionName, "", 0))
	w.WriteHeader(http.StatusNoContent)
}

func (a *productionAuth) denyLogin(w http.ResponseWriter, r *http.Request, reason string) {
	a.audit(r.Context(), productionActor{}, "login_denied", reason)
	http.Redirect(w, r, "/login?error=sign_in", http.StatusSeeOther)
}

func (a *productionAuth) audit(ctx context.Context, actor productionActor, event, reason string) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := a.store.audit(ctx, actor, event, reason); err != nil {
		log.Printf("production authentication audit write failed (event=%s)", event)
	}
}

func (a *productionAuth) unavailable(w http.ResponseWriter, operation string) {
	log.Printf("production authentication dependency unavailable (operation=%s)", operation)
	problem(w, http.StatusServiceUnavailable, "Sign-in is temporarily unavailable.")
}
