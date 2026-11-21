# Production authentication

**Status** · Implemented in the working tree. Live identity-provider and browser acceptance are pending.

PayGuard users sign in with their organization's OIDC provider. Go verifies the identity, creates the session, and authorizes access to merchant data and platform operations. Next.js presents the permitted workspace and forwards the session cookie to Go.

## Runtime configuration

| Environment variable | Service | Required value |
| --- | --- | --- |
| `PAYGUARD_AUTH_MODE` | Next.js and Go | `oidc` |
| `PAYGUARD_PUBLIC_ORIGIN` | Next.js and Go | The same fixed HTTPS origin, with no path, query, fragment, or trailing slash |
| `PAYGUARD_OIDC_ISSUER` | Go | The exact HTTPS issuer from the provider's discovery document |
| `PAYGUARD_OIDC_CLIENT_ID` | Go | The confidential web client's ID |
| `PAYGUARD_OIDC_CLIENT_SECRET` | Go | The web client's secret from the deployment secret store |
| `PAYGUARD_API_URL` | Next.js | The Go service origin reachable by the Next.js server at runtime |

Register the exact redirect URI as the public origin followed by `/api/auth/callback`. Configure Authorization Code with S256 PKCE and keep the public host intact through the proxy. Use a stable HTTPS origin for staging and production. Cloudflare Quick Tunnels provide temporary addresses for development. [OAuth 2.0 Security Best Current Practice](https://datatracker.ietf.org/doc/html/rfc9700) · [Cloudflare Quick Tunnels](https://developers.cloudflare.com/tunnel/get-started/quick-tunnels/)

Keep `PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN` unset in public environments. Local Sandbox Preview runs separately on loopback. Its route requires a loopback host and rejects public proxy metadata and non-loopback peer headers. OIDC mode disables that route. The Dockerfiles, Compose file, and Render blueprint retain their existing configuration. Supply the deployment variables to both service runtimes through the authorized deployment environment.

## Accounts and permissions

An authorized operator provisions `production_auth_identities` after migration. Use the verified OIDC `(issuer, subject)` pair, with the same issuer configured in Go and the exact case-sensitive `sub` identifier issued for this client. Use a separate named account for the maker and checker. Map merchant accounts to an active `merchant_demo_merchants.id` UUID. Set `merchant_id` to NULL for platform accounts.

| Role | Landing page | Authorization |
| --- | --- | --- |
| `platform_admin` | Merchant Directory | Manage merchant records and Platform Rule drafts, submit changes, and administer AI Configuration |
| `merchant` | Merchant Directory | See the assigned merchant and its permitted ledger and Merchant Policy functions |
| `platform_checker` | Platform Rules | Review governance history and approve or roll back rules with checker permission |

Store the account's immutable identity ID with new platform submissions and decisions. Go rejects approval by the submitting account even if its role changes. A platform maker must resubmit a historical pending revision whose original submitter has no identity ID. Historical approved versions and transaction evidence remain available.

Set `disabled_at` on the identity to revoke access. Permission changes invalidate outstanding sessions. A deactivated merchant also loses access. Provision a new identity for a new issuer or subject and disable the old identity. Use the identity provider's MFA policy for platform accounts.

## Session and audit

Login uses one-time browser-bound state, a nonce and S256 PKCE, with a ten-minute login window. Go verifies the signed ID token and grants access through the pre-provisioned identity. The browser receives an opaque host-only cookie with `Secure`, `HttpOnly` and `SameSite=Lax`. PostgreSQL stores the session hash. Sessions expire after 30 minutes of inactivity or eight hours from issuance. Logout revokes the session. Go requires the exact configured browser origin for logout and business mutations.

Login success, login denial, logout and origin rejection are recorded in `production_auth_audit`. Structured Go access logs record account ID, role, merchant ID, route pattern, method and status for mutations and denied requests. Set audit retention and review alerting as part of the deployment configuration. Redact cookies and authorization callback query values in edge logs.

## Migration and release gates

At startup, Go applies the additive `production-auth-v1` migration within the existing PostgreSQL transaction and advisory lock. It adds the identity, login-flow, session and audit tables, and a nullable identity reference on platform revisions. `payguard_schema_migrations` records successful application. Use a dedicated authorized database operator for account provisioning and keep a database backup before the first production startup with the migration.

1. Verify migration and a repeat startup on staging using representative existing data. Check the migration marker, rule versions and transaction lineage.
2. Configure a real staging OIDC client and stable HTTPS origin. Run fresh browser logins for all three roles. Verify the landing pages, tenant scope, secure cookie attributes, logout, session expiry and lack of demo-token prompts.
3. Verify direct Go authorization and platform maker–checker separation after account role changes. Confirm audit delivery and keep the local preview credential and route out of the public deployment.
4. Authorize any rollout separately after staging acceptance. Retain the prior application release and database backup for recovery.

## Verification

- In `backend`, `go test -json -count=1 ./...` and `go test -json -race -count=1 ./...` each recorded 376 passing tests and subtests with zero failures or skips against disposable PostgreSQL. `go vet ./...` passed.
- The Go suite covered a TLS OIDC fixture, login and logout, cookie attributes, session expiry and grant invalidation, direct API authorization, separate merchants and maker–checker separation. A separate legacy-schema fixture preserved historical rule and transaction data across migration and a repeat startup.
- Twelve frontend tests, TypeScript checking and the production build passed. An isolated Next.js runtime test verified role-specific pages, sign-in without demo prompts, callback and cookie forwarding, logout, and loopback preview isolation. A browser test with a synthetic identity fixture verified the merchant landing page, assigned-merchant visibility and return to sign-in after expiration.
- `docker compose config --quiet` and `git diff --check` passed. Live browser acceptance with the intended identity provider remains pending.
