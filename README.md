# PayGuard Simulation Lab

A local sandbox for synthetic OTA bookings and e-commerce checkout risk. It runs a seeded scenario through Go, computes entity velocity in Redis, saves run evidence in PostgreSQL, and exposes an offline QA report in a Next.js and Tailwind UI.

## Run locally

The repository-root [`.env.example`](.env.example) lists the demo merchant and optional provider variables. For authenticated API requests or external enrichment, follow the [local keys setup](docs/simulation-runbook.md#local-keys-and-omise-sandbox-preparation).

Requires Docker with Docker Compose. From the repository root, build and start the Go API, TypeScript/Next.js frontend, PostgreSQL, and Redis:

```sh
docker compose up --build -d --wait
```

Open [Simulation Lab](http://127.0.0.1:3000). Select **Run simulation**, inspect a decision, then select **Score run** to calculate the synthetic QA metrics. Use `docker compose logs -f` to follow service logs and `docker compose down` to stop the stack. PostgreSQL data persists in the Compose volume.

### Run the application directly on the host

For Go 1.26+ and Node.js 24+ development on the host, start only PostgreSQL and Redis with Compose.

1. Start the data stores from the repo root.

   ```sh
   docker compose up -d --wait postgres redis
   ```

2. Start the Go API in a terminal.

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

4. Open [Simulation Lab](http://127.0.0.1:3000). Select **Run simulation**, inspect a decision, then select **Score run** to calculate the synthetic QA metrics.

Go listens on port 8081, PostgreSQL on 55432, and Redis on 56379. The default bindings use loopback. `docker compose down` stops the data stores and preserves the PostgreSQL volume.

See the [runbook](docs/simulation-runbook.md) for the API, configuration and verification commands, or open [docs/home.md](docs/home.md) as the starting point for the Obsidian vault.
