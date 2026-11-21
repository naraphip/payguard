package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

var validID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

type app struct {
	repo   repository
	redis  *redis.Client
	secret []byte
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, env("DATABASE_URL", "postgres://payguard:payguard@127.0.0.1:55432/payguard?sslmode=disable"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := initialize(ctx, db); err != nil {
		log.Fatal(err)
	}
	options, err := redis.ParseURL(env("REDIS_URL", "redis://127.0.0.1:56379/0"))
	if err != nil {
		log.Fatal(err)
	}
	cache := redis.NewClient(options)
	defer cache.Close()

	a := app{repository{db}, cache, []byte(env("ENTITY_KEY_SECRET", "payguard-local-synthetic-v1"))}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /api/v1/simulation/runs", a.create)
	mux.HandleFunc("GET /api/v1/simulation/runs/{id}", a.readRun)
	mux.HandleFunc("GET /api/v1/simulation/runs/{id}/report", a.readReport)
	srv := &http.Server{
		Addr: env("PAYGUARD_ADDR", "127.0.0.1:8081"), Handler: mux,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: time.Minute,
	}
	log.Printf("Simulation API listening on %s", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func problem(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}

func unavailable(w http.ResponseWriter, err error) {
	log.Printf("simulation dependency error: %v", err)
	problem(w, 503, "Simulation storage is temporarily unavailable. Please retry.")
}

func (a app) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.repo.db.Ping(ctx); err != nil {
		unavailable(w, err)
		return
	}
	if err := a.redis.Ping(ctx).Err(); err != nil {
		unavailable(w, err)
		return
	}
	respond(w, 200, map[string]string{"status": "ready"})
}

func (a app) create(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID     string `json:"id"`
		Config config `json:"config"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || !validID.MatchString(request.ID) {
		problem(w, 400, "Expected a run UUID and simulation configuration.")
		return
	}
	request.Config.ContactEmailDomain = strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, strings.TrimSpace(request.Config.ContactEmailDomain))
	if err := request.Config.validate(); err != nil {
		problem(w, 422, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	saved, err := a.repo.get(ctx, merchantID, request.ID)
	switch {
	case err == nil:
		returnRun(w, saved, request.Config)
		return
	case !errors.Is(err, pgx.ErrNoRows):
		unavailable(w, err)
		return
	}

	id, err := attemptID()
	if err != nil {
		unavailable(w, err)
		return
	}
	events, truth := generate(request.Config, merchantID, a.secret)
	counter := redisVelocity{a.redis, "payguard:" + merchantID + ":" + request.ID + ":" + id}
	decisions, err := evaluate(ctx, counter, events, request.Config.MerchantPolicy, request.Config.JevTimeout)
	if err != nil {
		unavailable(w, err)
		return
	}
	result := run{request.ID, request.Config, generatorVersion, ruleVersion, jevVersion, len(events) - len(decisions), decisions}
	saved, err = a.repo.save(ctx, merchantID, result, truth)
	if err != nil {
		unavailable(w, err)
		return
	}
	returnRun(w, saved, request.Config)
}

func returnRun(w http.ResponseWriter, saved run, requested config) {
	if saved.Config != requested {
		problem(w, 409, "Run ID already exists with a different configuration.")
		return
	}
	respond(w, 200, saved)
}

func (a app) find(w http.ResponseWriter, r *http.Request) (run, bool) {
	id := r.PathValue("id")
	if !validID.MatchString(id) {
		problem(w, 404, "Run not found.")
		return run{}, false
	}
	result, err := a.repo.get(r.Context(), merchantID, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, 404, "Run not found.")
	case err != nil:
		unavailable(w, err)
	default:
		return result, true
	}
	return run{}, false
}

func (a app) readRun(w http.ResponseWriter, r *http.Request) {
	if saved, ok := a.find(w, r); ok {
		respond(w, 200, saved)
	}
}

func (a app) readReport(w http.ResponseWriter, r *http.Request) {
	saved, ok := a.find(w, r)
	if !ok {
		return
	}
	result, err := a.repo.qaReport(r.Context(), merchantID, saved)
	if err != nil {
		unavailable(w, err)
		return
	}
	respond(w, 200, result)
}
