package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type testOIDCIdentity struct {
	subject, audience, nonce, challenge string
	expiresAt                           time.Time
}

func testIdentityToken(key *rsa.PrivateKey, issuer string, identity testOIDCIdentity) (string, error) {
	encode := base64.RawURLEncoding.EncodeToString
	claims, err := json.Marshal(map[string]any{
		"iss": issuer, "sub": identity.subject, "aud": identity.audience, "nonce": identity.nonce,
		"iat": time.Now().Add(-time.Minute).Unix(), "exp": identity.expiresAt.Unix(),
	})
	if err != nil {
		return "", err
	}
	message := encode([]byte(`{"alg":"RS256","kid":"issuer-fixture"}`)) + "." + encode(claims)
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	return message + "." + encode(signature), err
}

func requireAuthStatus(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status=%d, expected %d: %s", response.Code, status, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("auth response can be cached")
	}
}

func requireAuthCookie(t *testing.T, response *httptest.ResponseRecorder, name string, maxAge int) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name != name {
			continue
		}
		if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Domain != "" || cookie.Path != "/" || cookie.MaxAge != maxAge {
			t.Fatalf("incorrect cookie attributes for %s", name)
		}
		return cookie
	}
	t.Fatalf("missing cookie %s", name)
	return nil
}

func TestProductionAuthOIDCLifecyclePostgres(t *testing.T) {
	db := openPolicyTestDatabase(t)
	const origin = "https://payguard.example.test"
	const clientID = "payguard-fixture"
	const clientSecret = "fixture-client-secret"
	const code = "fixture-authorization-code"
	const redirectURI = origin + "/api/auth/callback"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer string
	var identity testOIDCIdentity
	var mu sync.Mutex
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			respond(w, http.StatusOK, map[string]any{
				"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys",
				"code_challenge_methods_supported": []string{"S256"}, "id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/token":
			user, secret, _ := r.BasicAuth()
			if err := r.ParseForm(); err != nil || user != clientID || secret != clientSecret || r.Form.Get("code") != code ||
				r.Form.Get("redirect_uri") != redirectURI || r.Form.Get("grant_type") != "authorization_code" ||
				oauth2.S256ChallengeFromVerifier(r.Form.Get("code_verifier")) != identity.challenge {
				respond(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
				return
			}
			idToken, err := testIdentityToken(key, issuer, identity)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			respond(w, http.StatusOK, map[string]any{"token_type": "Bearer", "access_token": "fixture-opaque-access-token", "id_token": idToken})
		case "/keys":
			respond(w, http.StatusOK, map[string]any{"keys": []map[string]string{{
				"kty": "RSA", "alg": "RS256", "kid": "issuer-fixture", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()), "e": "AQAB",
			}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(provider.Close)
	mu.Lock()
	issuer = provider.URL
	mu.Unlock()
	auth, err := configureProductionAuth(t.Context(), productionAuthStore{db: db},
		productionAuthConfig{issuer: issuer, clientID: clientID, clientSecret: clientSecret, origin: origin}, provider.Client())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	auth.register(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /api/v1/protected", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := productionActorFromRequest(r)
		if !ok || actor.Role != productionRoleAdmin || r.Header.Get("Authorization") != "" {
			t.Error("protected handler did not receive only the trusted principal")
		}
		respond(w, http.StatusOK, actor)
	})
	mux.HandleFunc("POST /api/v1/protected", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := auth.protect(mux)

	request := func(method, path, requestOrigin string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, origin+path, nil)
		if requestOrigin != "" {
			r.Header.Set("Origin", requestOrigin)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		r.Header.Set("Authorization", "Bearer sandbox-test-credential")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	start := func(subject, audience string, expiry time.Time, correctNonce bool) (string, *http.Cookie) {
		t.Helper()
		response := request(http.MethodGet, "/api/auth/login", "", nil)
		requireAuthStatus(t, response, http.StatusSeeOther)
		location, err := url.Parse(response.Header().Get("Location"))
		if err != nil || location.Scheme+"://"+location.Host != issuer || location.Path != "/authorize" {
			t.Fatal("sign in did not use the discovered authorize endpoint")
		}
		query := location.Query()
		if query.Get("redirect_uri") != redirectURI || query.Get("response_type") != "code" || query.Get("scope") != "openid" ||
			query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" ||
			query.Get("state") == "" || query.Get("nonce") == "" || query.Get("client_secret") != "" {
			t.Fatal("invalid OIDC request configuration")
		}
		nonce := query.Get("nonce")
		if !correctNonce {
			nonce = randomAuthValue()
		}
		mu.Lock()
		identity = testOIDCIdentity{subject: subject, audience: audience, nonce: nonce, challenge: query.Get("code_challenge"), expiresAt: expiry}
		mu.Unlock()
		return "/api/auth/callback?" + url.Values{"state": {query.Get("state")}, "code": {code}}.Encode(),
			requireAuthCookie(t, response, productionOIDCName, int(productionLoginTTL.Seconds()))
	}

	subject := randomAuthValue()
	var identityID string
	if err := db.QueryRow(t.Context(), `INSERT INTO production_auth_identities (issuer,subject,role) VALUES($1,$2,$3) RETURNING id::text`,
		issuer, subject, productionRoleAdmin).Scan(&identityID); err != nil {
		t.Fatal(err)
	}
	if request(http.MethodGet, "/healthz", "", nil).Code != http.StatusOK {
		t.Fatal("health check must remain public")
	}
	requireAuthStatus(t, request(http.MethodGet, "/api/v1/protected", "", nil), http.StatusUnauthorized)
	flowPath, flowCookie := start(subject, clientID, time.Now().Add(time.Hour), true)
	callback := request(http.MethodGet, flowPath, "", flowCookie)
	requireAuthStatus(t, callback, http.StatusSeeOther)
	if callback.Header().Get("Location") != "/settings/merchants" {
		t.Fatal("successful admin login must open the directory")
	}
	requireAuthCookie(t, callback, productionOIDCName, -1)
	session := requireAuthCookie(t, callback, productionSessionName, int(productionSessionTTL.Seconds()))
	if session.Value == flowCookie.Value || authHash(session.Value) == nil {
		t.Fatal("expected a distinct opaque session after sign-in")
	}

	status := request(http.MethodGet, "/api/auth/session", "", session)
	requireAuthStatus(t, status, http.StatusOK)
	if status.Body.String() != "{\"role\":\"platform_admin\"}\n" {
		t.Fatalf("unexpected public session response: %s", status.Body.String())
	}
	requireAuthStatus(t, request(http.MethodGet, "/api/v1/protected", "", session), http.StatusOK)
	requireAuthStatus(t, request(http.MethodPost, "/api/v1/protected", "", session), http.StatusForbidden)
	requireAuthStatus(t, request(http.MethodPost, "/api/v1/protected", origin, session), http.StatusNoContent)
	for _, item := range []struct {
		name, subject, audience string
		expiry                  time.Time
		correctNonce            bool
	}{
		{"unprovisioned identity", randomAuthValue(), clientID, time.Now().Add(time.Hour), true},
		{"wrong audience", subject, "other-client", time.Now().Add(time.Hour), true},
		{"wrong nonce", subject, clientID, time.Now().Add(time.Hour), false},
		{"expired ID token", subject, clientID, time.Now().Add(-time.Second), true},
	} {
		t.Run(item.name, func(t *testing.T) {
			path, cookie := start(item.subject, item.audience, item.expiry, item.correctNonce)
			denial := request(http.MethodGet, path, "", cookie)
			requireAuthStatus(t, denial, http.StatusSeeOther)
			if denial.Header().Get("Location") != "/login?error=sign_in" {
				t.Fatal("invalid identity must return to sign-in")
			}
			if strings.Contains(denial.Header().Get("Set-Cookie"), productionSessionName) {
				t.Fatal("rejected login must not create a session")
			}
		})
	}
	repeated := request(http.MethodGet, flowPath, "", flowCookie)
	if repeated.Header().Get("Location") != "/login?error=sign_in" {
		t.Fatal("used login flow must be rejected")
	}

	requireAuthStatus(t, request(http.MethodPost, "/api/auth/logout", "", session), http.StatusForbidden)
	requireAuthStatus(t, request(http.MethodGet, "/api/auth/session", "", session), http.StatusOK)
	logout := request(http.MethodPost, "/api/auth/logout", origin, session)
	requireAuthStatus(t, logout, http.StatusNoContent)
	requireAuthCookie(t, logout, productionSessionName, -1)
	requireAuthStatus(t, request(http.MethodGet, "/api/auth/session", "", session), http.StatusUnauthorized)
	var auditCount int
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM production_auth_audit WHERE actor_id=$1::uuid
  AND event IN ('login_succeeded','logout')`, identityID).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("expected login and logout audit rows, got %d (%v)", auditCount, err)
	}
}

func TestProductionAuthSessionExpirationPostgres(t *testing.T) {
	db := openPolicyTestDatabase(t)
	const issuer = "https://issuer.example.test"
	const origin = "https://payguard.example.test"
	subject := randomAuthValue()
	if _, err := db.Exec(t.Context(), `INSERT INTO production_auth_identities(issuer,subject,role) VALUES($1,$2,$3)`,
		issuer, subject, productionRoleChecker); err != nil {
		t.Fatal(err)
	}
	auth := &productionAuth{store: productionAuthStore{db: db}, origin: origin}
	mux := http.NewServeMux()
	auth.register(mux)
	mux.HandleFunc("GET /api/v1/protected", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := auth.protect(mux)
	for _, item := range []struct {
		name, update, path string
		status             int
		touched            bool
	}{
		{"absolute expiry", "expires_at=clock_timestamp()-interval '1 second'", "/api/auth/session", http.StatusUnauthorized, false},
		{"idle expiry", "last_seen_at=clock_timestamp()-interval '30 minutes'", "/api/auth/session", http.StatusUnauthorized, false},
		{"read-only probe", "last_seen_at=clock_timestamp()-interval '27 minutes'", "/api/auth/session", http.StatusOK, false},
		{"business activity", "last_seen_at=clock_timestamp()-interval '27 minutes'", "/api/v1/protected", http.StatusNoContent, true},
	} {
		t.Run(item.name, func(t *testing.T) {
			raw := randomAuthValue()
			hash := authHash(raw)
			if _, err := auth.store.issue(t.Context(), issuer, subject, hash, nil); err != nil {
				t.Fatal(err)
			}
			var lastSeenBefore, expiryBefore time.Time
			if err := db.QueryRow(t.Context(), `UPDATE production_auth_sessions SET `+item.update+` WHERE token_hash=$1
        RETURNING last_seen_at,expires_at`, hash).Scan(&lastSeenBefore, &expiryBefore); err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodGet, origin+item.path, nil)
			r.AddCookie(authCookie(productionSessionName, raw, productionSessionTTL))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			requireAuthStatus(t, w, item.status)
			var lastSeenAfter, expiryAfter time.Time
			if err := db.QueryRow(t.Context(), `SELECT last_seen_at,expires_at FROM production_auth_sessions WHERE token_hash=$1`,
				hash).Scan(&lastSeenAfter, &expiryAfter); err != nil {
				t.Fatal(err)
			}
			if !expiryBefore.Equal(expiryAfter) || lastSeenAfter.After(lastSeenBefore) != item.touched {
				t.Fatal("session expiry or last-seen is incorrect")
			}
		})
	}
}

func TestProductionAuthModes(t *testing.T) {
	t.Setenv("PAYGUARD_AUTH_MODE", "")
	if auth, err := newProductionAuth(t.Context(), nil); err != nil || auth != nil {
		t.Fatal("default startup must preserve the sandbox")
	}
	t.Setenv("PAYGUARD_AUTH_MODE", "invalid")
	if auth, err := newProductionAuth(t.Context(), nil); err == nil || auth != nil {
		t.Fatal("unrecognized auth mode must fail startup")
	}
	t.Setenv("PAYGUARD_AUTH_MODE", "oidc")
	t.Setenv("PAYGUARD_PUBLIC_ORIGIN", "http://payguard.example.test")
	if auth, err := newProductionAuth(t.Context(), nil); err == nil || auth != nil {
		t.Fatal("OIDC production origin must use HTTPS")
	}
}
