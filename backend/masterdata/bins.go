package masterdata

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed data/bins.csv.gz
var binSnapshot []byte

var binPattern = regexp.MustCompile(`^[0-9]{6,8}$`)

func ValidBIN(value string) bool {
	return binPattern.MatchString(value)
}

type BINRecord struct {
	BINData
	RecordSource string `json:"recordSource,omitempty"`
	Seen         string `json:"seen,omitempty"`
}

type BINLookup struct {
	Status        string     `json:"status"`
	Match         string     `json:"match,omitempty"`
	MatchedPrefix string     `json:"matchedPrefix,omitempty"`
	Source        string     `json:"source,omitempty"`
	Revision      string     `json:"revision,omitempty"`
	Data          *BINRecord `json:"data,omitempty"`
}

type BINImport struct {
	Source   string
	Revision string
	Rows     int64
	Reused   bool
}

const binMasterMappingVersion = "payguard-bin-fields-v1"

const binMasterSchema = `
CREATE TABLE IF NOT EXISTS enrichment_bin_master (
	prefix text PRIMARY KEY CHECK (prefix ~ '^[0-9]{6,8}$'),
	scheme text NOT NULL,
	funding text NOT NULL,
	issuer text NOT NULL,
	country_code text NOT NULL,
	record_source text NOT NULL,
	seen text NOT NULL
);

CREATE TABLE IF NOT EXISTS enrichment_bin_snapshot (
	singleton boolean PRIMARY KEY CHECK (singleton),
	source text NOT NULL,
	revision text NOT NULL,
	snapshot_sha text NOT NULL,
	record_count bigint NOT NULL CHECK (record_count > 0),
	imported_at timestamptz NOT NULL DEFAULT now()
);
`

// ImportBINMaster atomically replaces the active master when the bundled snapshot changes.
func ImportBINMaster(ctx context.Context, db *pgxpool.Pool) (BINImport, error) {
	compressed, err := gzip.NewReader(bytes.NewReader(binSnapshot))
	if err != nil {
		return BINImport{}, fmt.Errorf("open BIN snapshot: %w", err)
	}
	defer compressed.Close()

	reader := csv.NewReader(compressed)
	reader.FieldsPerRecord = -1
	metadata, err := reader.Read()
	if err != nil || len(metadata) != 3 || metadata[0] == "" || metadata[1] == "" {
		return BINImport{}, errors.New("invalid BIN snapshot metadata")
	}
	expectedRows, err := strconv.ParseInt(metadata[2], 10, 64)
	if err != nil || expectedRows <= 0 {
		return BINImport{}, errors.New("invalid BIN snapshot count")
	}
	header, err := reader.Read()
	if err != nil || !slices.Equal(header, []string{"prefix", "scheme", "funding", "issuer", "countryCode", "recordSource", "seen"}) {
		return BINImport{}, errors.New("invalid BIN snapshot header")
	}
	reader.FieldsPerRecord = 7
	reader.ReuseRecord = true
	checksum := sha256.Sum256(binSnapshot)
	snapshotSHA := binMasterMappingVersion + ":" + hex.EncodeToString(checksum[:])
	result := BINImport{Source: metadata[0], Revision: metadata[1], Rows: expectedRows}

	tx, err := db.Begin(ctx)
	if err != nil {
		return BINImport{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(133025850)"); err != nil {
		return BINImport{}, err
	}
	if _, err := tx.Exec(ctx, binMasterSchema); err != nil {
		return BINImport{}, err
	}

	var activeSHA string
	var activeRows int64
	err = tx.QueryRow(ctx, "SELECT snapshot_sha, record_count FROM enrichment_bin_snapshot WHERE singleton").
		Scan(&activeSHA, &activeRows)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return BINImport{}, err
	}
	if activeSHA == snapshotSHA && activeRows == expectedRows {
		result.Reused = true
		if err := tx.Commit(ctx); err != nil {
			return BINImport{}, err
		}
		return result, nil
	}
	if _, err := tx.Exec(ctx, "DELETE FROM enrichment_bin_master"); err != nil {
		return BINImport{}, err
	}
	loaded, err := tx.CopyFrom(ctx, pgx.Identifier{"enrichment_bin_master"},
		[]string{"prefix", "scheme", "funding", "issuer", "country_code", "record_source", "seen"},
		pgx.CopyFromFunc(func() ([]any, error) {
			record, err := reader.Read()
			if errors.Is(err, io.EOF) {
				return nil, nil
			}
			if err != nil {
				return nil, fmt.Errorf("read BIN snapshot: %w", err)
			}
			data := NormalizeBINData(BINData{
				Scheme: record[1], Funding: record[2], Issuer: record[3], CountryCode: record[4],
			})
			return []any{record[0], data.Scheme, data.Funding, data.Issuer, data.CountryCode, record[5], record[6]}, nil
		}))
	if err != nil {
		return BINImport{}, fmt.Errorf("copy BIN master: %w", err)
	}
	if loaded != expectedRows {
		return BINImport{}, fmt.Errorf("BIN snapshot row count mismatch: copied %d, expected %d", loaded, expectedRows)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO enrichment_bin_snapshot (singleton, source, revision, snapshot_sha, record_count)
		VALUES (true, $1, $2, $3, $4)
		ON CONFLICT (singleton) DO UPDATE SET source = EXCLUDED.source, revision = EXCLUDED.revision,
			snapshot_sha = EXCLUDED.snapshot_sha, record_count = EXCLUDED.record_count, imported_at = now()`,
		result.Source, result.Revision, snapshotSHA, loaded); err != nil {
		return BINImport{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BINImport{}, err
	}
	return result, nil
}

func LookupBIN(ctx context.Context, db *pgxpool.Pool, bin string) (BINLookup, error) {
	result := BINLookup{Status: "NOT_FOUND"}
	if !ValidBIN(bin) {
		return result, nil
	}
	if db == nil {
		return BINLookup{}, errors.New("BIN master database unavailable")
	}
	var record BINRecord
	err := db.QueryRow(ctx, `
		SELECT s.source, s.revision, coalesce(r.prefix, ''), coalesce(r.scheme, ''),
			coalesce(r.funding, ''), coalesce(r.issuer, ''), coalesce(r.country_code, ''),
			coalesce(r.record_source, ''), coalesce(r.seen, '')
		FROM enrichment_bin_snapshot AS s
		LEFT JOIN LATERAL (
			SELECT * FROM enrichment_bin_master
			WHERE prefix IN ($1, left($1, 7), left($1, 6))
			ORDER BY length(prefix) DESC LIMIT 1
		) AS r ON true
		WHERE s.singleton`, bin).Scan(&result.Source, &result.Revision, &result.MatchedPrefix,
		&record.Scheme, &record.Funding, &record.Issuer, &record.CountryCode, &record.RecordSource, &record.Seen)
	if err != nil {
		return BINLookup{}, fmt.Errorf("lookup BIN master: %w", err)
	}
	if result.MatchedPrefix != "" {
		result.Status = "FOUND"
		result.Match = "PREFIX"
		if result.MatchedPrefix == bin {
			result.Match = "EXACT"
		}
		result.Data = &record
	}
	return result, nil
}
