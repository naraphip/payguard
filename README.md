# PayGuard Simulation Lab

A local sandbox for synthetic OTA bookings and e-commerce checkout risk. It runs a seeded scenario through Go, computes entity velocity in Redis, saves run evidence in PostgreSQL, and exposes an offline QA report in a Next.js and Tailwind UI.

## Run locally

Requires Docker Compose, Go 1.26+, and Node.js 24+.

1. Start the data stores from the repo root.

   ```sh
   docker compose up -d --wait
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
