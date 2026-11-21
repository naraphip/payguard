# PayGuard Simulation Lab

A local sandbox for synthetic OTA bookings and e-commerce checkout risk. It runs a seeded scenario through Go, computes entity velocity in Redis, saves run evidence in PostgreSQL, and exposes an offline QA report in a Next.js and Tailwind UI.

## Run locally

The repository-root [`.env.example`](.env.example) lists the demo merchant and optional provider variables. For Omise Sandbox checkout, put `OMISE_PUBLIC_KEY` and `OMISE_SECRET_KEY` from Omise Test mode in the ignored `backend/.env`. Compose loads that file for the Go backend. See the [local keys setup](docs/simulation-runbook.md#local-keys-and-omise-sandbox-preparation).

For live Jev in merchant checkout and scenario simulation, set `OPENROUTER_API_KEY` in the ignored root `.env` and select OpenRouter in **Decision Engine → AI Configuration**. Until an administrator saves a configuration version, `PAYGUARD_AI_MODE` sets the deployment default. The [Jev walkthrough](docs/simulation-runbook.md#enable-live-jev-for-synthetic-tests) shows how to verify the saved provider response.

Requires Docker with Docker Compose. From the repository root, build and start the Go API, TypeScript/Next.js frontend, PostgreSQL, and Redis:

```sh
docker compose up --build -d --wait
```

Open **Testing → Simulation Lab**, choose a batch product, and select **Run batch simulation** in the inline summary. Under **Batch simulation results**, select an event to inspect the recorded decision and any earlier events with shared signals. Eligible first orders in **Travel** and **Online store** can proceed to Omise Sandbox checkout. [Simulation Lab](http://127.0.0.1:3000/simulation-lab) · [Batch evidence walkthrough](docs/simulation-runbook.md#verify-batch-decision-evidence) · [Omise Sandbox checkout](docs/simulation-runbook.md#omise-sandbox-checkout)

Use `docker compose logs -f` to follow service logs and `docker compose down` to stop the stack. PostgreSQL data persists in the Compose volume.

### Run the application directly on the host

For Go 1.26+ and Node.js 24+ development on the host, start only PostgreSQL and Redis with Compose.

1. Start the data stores from the repo root.

   ```sh
   docker compose up -d --wait postgres redis
   ```

2. Start the Go API in a terminal. For sandbox checkout, use the [host setup that exports the backend keys](docs/simulation-runbook.md#local-keys-and-omise-sandbox-preparation).

   ```sh
   cd backend
   go run .
   ```

3. Start Next.js in another terminal.

   ```sh
   cd frontend
   npm ci
   npm run dev
   ```

4. Open [Simulation Lab](http://127.0.0.1:3000/simulation-lab) and run the same storefront flow.

Go listens on port 8081, PostgreSQL on 55432, and Redis on 56379. The default bindings use loopback. `docker compose down` stops the data stores and preserves the PostgreSQL volume.

## AI Rule Management

Use **Merchant AI Rules** with the scoped token issued to a merchant, or **Platform Rules** with the demo administrator token. Enable server-side composition in the root `.env` with `PAYGUARD_RULE_COMPOSER_ENABLED=true`, `PAYGUARD_RULE_COMPOSER_PROVIDER=openai`, and a structured-output text model such as `PAYGUARD_RULE_COMPOSER_MODEL=gpt-4.1-mini`. Set `OPENAI_API_KEY` in `backend/.env`. [Configuration, API, and rollback](docs/simulation-runbook.md#ai-rule-composer) · [Rule contract](docs/requirements/ai-rule-management.md)

Platform administrators can test Jev with a fixed synthetic input in **AI Configuration**. Saved settings apply to new merchant scenario transactions. The legacy Booking API and batch simulation retain their mock Jev evaluators. [Jev configuration](docs/simulation-runbook.md#jev-ai-configuration)

See the [runbook](docs/simulation-runbook.md) for the API, configuration and verification commands, or open [docs/home.md](docs/home.md) as the starting point for the Obsidian vault.

## Deploy a demo to Render

The repository includes [render.yaml](render.yaml) for a free Render Blueprint with the frontend, Go API, PostgreSQL, and Redis/Valkey-compatible Key Value store.

1. Push this repository to GitHub.
2. In Render, select **New > Blueprint** and connect the repository.
3. Review and apply [render.yaml](render.yaml).
4. Set `PAYGUARD_DEMO_MERCHANT_A_TOKEN` and `PAYGUARD_DEMO_MERCHANT_B_TOKEN` if the booking API should be enabled. Their values must be distinct.
5. Open the generated `payguard-frontend` URL.

After syncing this Blueprint in Render, both web services track `main` and auto-deploy on every commit to that branch.

### Update and verify the existing demo

If a service was suspended by its owner, have an authorized workspace member select **Resume** in the Render Dashboard. Confirm that `payguard-postgres` is available before deploying. [Render service controls](https://render.com/changelog/suspend-and-resume-services-in-bulk-from-the-render-dashboard)

1. Compare the commit on GitHub `main` with the live commit shown on the **Deploys** page for `payguard-backend` and `payguard-frontend`. If the Blueprint changed, confirm that its latest sync succeeded before deploying. If a service still needs the verified `main` commit and no deploy is running, select **Manual Deploy > Deploy latest commit** for that service. [Render deployment instructions](https://render.com/docs/deploys)
2. Once both services show the expected commit as live, use the public URLs shown in Render. Check that the backend returns HTTP 200 at `/healthz` and that the frontend returns HTTP 200 at `/dashboard` and JSON with HTTP 200 at `/api/v1/system/rules`. The rules request verifies the frontend-to-backend connection.
3. If a deployment fails, check its build and service logs. If a new live version fails the checks, use **Rollback** on the affected service's previous healthy deploy and repeat the health checks. A dashboard rollback turns off automatic deploys for that service. Turn them back on after the fix is verified. [Render rollback instructions](https://render.com/docs/rollbacks)

The Key Value store blocks external connections with an empty IP allow list; the backend connects using Render's internal URL.

This deployment is an online demo, not a production rollout. Render's free web services spin down after inactivity. The free PostgreSQL database expires after 30 days and the free Key Value instance may lose state on restart. Free PostgreSQL has no managed backups. Keep demo credentials and synthetic data only.
