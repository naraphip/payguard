package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pwatakul/payguard/backend/masterdata"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://payguard:payguard@127.0.0.1:55432/payguard?sslmode=disable"
	}
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		log.Fatal("could not configure the BIN master database")
	}
	defer db.Close()

	result, err := masterdata.ImportBINMaster(ctx, db)
	if err != nil {
		log.Fatalf("import BIN master: %v", err)
	}
	action := "Imported"
	if result.Reused {
		action = "Already imported"
	}
	fmt.Printf("%s %d BIN records from %s at %s\n", action, result.Rows, result.Source, result.Revision)
}
