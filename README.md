# PayGuard Simulation Lab

A local sandbox for synthetic OTA bookings and e-commerce checkout risk. It runs a seeded scenario through Go, computes entity velocity in Redis, saves run evidence in PostgreSQL, and exposes an offline QA report in a Next.js and Tailwind UI.

## Run locally

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

## Deploy a demo to Render

The repository includes [render.yaml](render.yaml) for a free Render Blueprint with the frontend, Go API, PostgreSQL, and Redis/Valkey-compatible Key Value store.

1. Push this repository to GitHub.
2. In Render, select **New > Blueprint** and connect the repository.
3. Review and apply [render.yaml](render.yaml).
4. Set `PAYGUARD_DEMO_MERCHANT_A_TOKEN` and `PAYGUARD_DEMO_MERCHANT_B_TOKEN` if the booking API should be enabled. Their values must be distinct.
5. Open the generated `payguard-frontend` URL.

This deployment is an online demo, not a production rollout. Render's free web services spin down after inactivity. The free PostgreSQL database expires after 30 days and the free Key Value instance may lose state on restart. Free PostgreSQL has no managed backups. Keep demo credentials and synthetic data only.
