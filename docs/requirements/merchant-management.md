# Merchant Management

The Sandbox directory and merchant detail workspace reuse the existing PostgreSQL merchant records, one-time credentials, scenarios, and transaction ledger. The prior merchant IDs and saved results remain the source of truth. This slice adds administrative views and read APIs around those records. [Merchant scenario requirements](merchant-scenarios.md) · [Runbook](../simulation-runbook.md#merchant-management) · [Verification](../current-work.md#merchant-management)

## Directory and detail

Open `/settings/merchants` as Platform Admin. The directory displays stored business data, transaction and active policy counts, and the last recorded transaction time. Search by name or merchant ID, sort by name, creation, or activity, and page results on the server. Dates for legacy records are unavailable when the original timestamp was never persisted.

Create a merchant using its business name, optional description, and independently selectable catalogue products. The persisted setup from the earlier registration flow remains compatible. Copy the merchant credential from the one-time issuance message and select **Open merchant details**. The persistent detail route `/settings/merchants/{merchantId}` provides Overview, Transactions, Merchant Rules, Advanced Risk Configuration, Integrations, and Audit Logs. Legacy links with `?merchant=` redirect to the persistent route with their tab and transaction selection. The Overview transaction counts have a 30-day window. The active policy count reflects the current ruleset. Open Mock Checkout or Simulation Lab from the merchant detail to preselect the merchant.

On the merchant’s **Overview**, a Platform Admin can select **Get temporary token**, then **Show token** or **Copy token**. The token grants access as that merchant for one hour. The original token issued at creation remains valid. The creation receipt also supports **Show token** and **Copy token**.

The transaction ledger supports a transaction ID prefix, outcome, review state, and inclusive UTC day filters. It shows saved outcomes and final decision sources, matched policies and versions when available, and separate review dispositions. Merchant Rules includes business policy inspection and the existing booking review rules with saved Jev results. Advanced Risk Configuration displays the available active profile version and signals. Integrations displays recorded products, stages, signals, and saved Sandbox scenarios. Platform Rules and AI Configuration have their own administrative workspaces. [Decision Engine](decision-engine.md)

## Permission model

| Actor | Capabilities in this slice |
| --- | --- |
| Platform Admin | List and create merchants, edit metadata, issue temporary merchant tokens, deactivate merchants, read a selected merchant's records and rule history, and record existing administrative dispositions. Merchant Policy inspection uses read-only endpoints. |
| Merchant | Read its own scoped records, compose and manage its own policies, and view its own policy versions. The API derives the tenant from the issued merchant bearer token. |
| Platform checker | Reviews global Platform changes through the approval workflow. The checker credential does not confer merchant management access. |

Platform administration uses a configured demo credential. Merchant session tokens are scoped to a single UUID and deactivation revokes their access. Merchant creation, editing, deactivation, credential issuance and revocation create audit events. The audit view also reads policy versions and resolved reviews from their existing sources. It shows actor and resource, timestamp, and recorded versions or reasons when present. The Sandbox audit does not yet correlate every event to a request or expose a complete profile-approval history.

## API reads and compatibility

Use a bearer credential on the existing `/api/v1/booking/demo/merchants` base path. Replace `{merchantId}` and `{id}` with stored identifiers.

| Request relative to the base path | Result |
| --- | --- |
| `GET ?q=&sort=name&page=1&limit=20` | Platform directory page with server-side counts and pagination. Sort values are `name`, `created`, and `activity`. |
| `GET /{merchantId}` | Detail, 30-day metrics, supported profile context, and up to five recent transactions. |
| `GET /{merchantId}/transactions?page=1&limit=20` | Recent-first transaction page. Accepts `q`, `outcome`, `reviewState`, `from`, and `to`. Dates are `YYYY-MM-DD` in UTC. |
| `GET /{merchantId}/policies` | Read a merchant's policies and effective versions. |
| `GET /{merchantId}/policies/{id}` | Read the rule and its stored version history. |
| `GET /{merchantId}/audit` | Up to 100 recent audit entries for the merchant. |
| `POST /{merchantId}/access-token` | Platform Admin issues a temporary token for an active merchant. The HTTP 201 response contains only `token` and `expiresAt` and uses `Cache-Control: no-store`. |

The original directory and transaction list responses remain available without query parameters. The additive startup migration adds business description, creation time, and a creation request hash to merchant records and creates the audit table. It preserves merchant IDs and stored decisions. [API implementation](../../backend/merchant_management.go) · [Merchant policy mutation API](../simulation-runbook.md#composer-and-policy-apis)

The database stores temporary token hashes with their expiration times. Issuance records a `MERCHANT_TEMPORARY_CREDENTIAL_ISSUED` audit event. Directory, detail, and audit reads omit credential values. Deactivation ends access for both original and temporary tokens.

The existing administrator `POST /api/v1/booking/demo/merchants` accepts a multi-product catalogue and an optional `Idempotency-Key` of 1 to 128 printable ASCII characters without spaces. A repeated key returns 409 and leaves the original merchant in the directory. The issued credential appears in the original 201 response only. In this example, both product types belong to one tenant.

```json
{
  "name": "Travel and Retail Demo",
  "description": "Flight tickets and retail checkout",
  "policyIntegration": {
    "products": ["flight_ticket", "physical"],
    "emailDomain": true
  }
}
```

## Booking API v1 and the target data contract

The current `POST /api/v1/booking/events` uses authenticated merchant scope, an `Idempotency-Key` header, and a JSON body with `eventId`, `schemaVersion` set to `1`, `stage` set to `booking`, and `occurredAt`. It accepts optional `amountMinor`, `product`, and `sdkStatus`. The v1 parser rejects unknown fields. Merchant scenario transactions use their separate Sandbox endpoint with optional `customerEmailDomain`.

The production data dictionary describes a future contract. Its core fields are `schema_version`, `event_id`, `idempotency_key`, `occurred_at`, `event_type`, `stage`, and `vertical`. The server derives `merchant_id` from authenticated context. Conditional fields depend on the stage and product. They include a customer or guest reference, order or intent references, currency and amount, and payment attempt data once payment begins. Optional fraud signals can come from device, network, history, or PSP evidence. The target needs a published versioned API and validated required-field matrix. In particular, `hotel.guest_count` and `digital.delivery_method` have conflicting requirement levels between design sheets. Pending capabilities remain unavailable in the current composer. A missing observation stays distinct from zero or false. [Current v1 request](../simulation-runbook.md#booking-event-intake) · [Core API fields](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=260105) · [Required fields matrix](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=260108)
