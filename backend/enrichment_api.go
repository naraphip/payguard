package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pwatakul/payguard/backend/enrichment"
	"github.com/pwatakul/payguard/backend/masterdata"
	"github.com/redis/go-redis/v9"
)

type enrichmentHandler struct {
	auth    bookingAuth
	service enrichment.Service
	db      *pgxpool.Pool
	domains func() *masterdata.Domains
	cache   *redis.Client
}

func newEnrichmentHandler(auth bookingAuth, db *pgxpool.Pool, redisOptions *redis.Options) (enrichmentHandler, error) {
	handler := enrichmentHandler{auth: auth, db: db, domains: sync.OnceValue(func() *masterdata.Domains {
		domains, err := masterdata.LoadDomains()
		if err != nil {
			log.Printf("Disposable email master could not be loaded: %v", err)
			return nil
		}
		local := os.Getenv("PAYGUARD_EMAIL_MASTER_LOCAL")
		if local != "" {
			if err := domains.AddLocal(local); err != nil {
				log.Printf("Local disposable email master could not be loaded: %v", err)
			}
		} else {
			const defaultLocal = "masterdata/local/domains.txt.gz"
			if _, err := os.Stat(defaultLocal); err == nil {
				if err := domains.AddLocal(defaultLocal); err != nil {
					log.Printf("Local disposable email master could not be loaded: %v", err)
				}
			}
		}
		return domains
	})}

	enabled, err := strconv.ParseBool(env("PAYGUARD_ENRICHMENT_ENABLED", "false"))
	if err != nil {
		return handler, errors.New("PAYGUARD_ENRICHMENT_ENABLED must be a boolean")
	}
	if !enabled {
		return handler, nil
	}
	secret := []byte(os.Getenv("PAYGUARD_ENRICHMENT_CACHE_SECRET"))
	if len(secret) == 0 {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return handler, err
		}
	}
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	cacheOptions := *redisOptions
	cacheOptions.ContextTimeoutEnabled = true
	cacheOptions.MaxRetries = -1
	handler.cache = redis.NewClient(&cacheOptions)
	handler.service = enrichment.Service{
		IPProviders: []enrichment.Provider[enrichment.IPData]{
			enrichment.IPAPIIS(client, os.Getenv("PAYGUARD_IPAPI_IS_KEY")),
			enrichment.ProxyCheck(client, os.Getenv("PAYGUARD_PROXYCHECK_KEY")),
		},
		BINProviders: []enrichment.Provider[enrichment.BINData]{
			enrichment.BINScanner(client),
			enrichment.CardBINCheck(client),
		},
		Cache:       enrichment.RedisCache{Client: handler.cache},
		CacheSecret: secret,
	}
	return handler, nil
}

type emailMasterData struct {
	Domain        string   `json:"domain"`
	Listed        bool     `json:"listed"`
	MatchedDomain string   `json:"matchedDomain,omitempty"`
	Sources       []string `json:"sources,omitempty"`
}

type binMasterEvidence struct {
	Status        string              `json:"status"`
	Match         string              `json:"match,omitempty"`
	MatchedPrefix string              `json:"matchedPrefix,omitempty"`
	Source        string              `json:"source,omitempty"`
	Revision      string              `json:"revision,omitempty"`
	RecordSource  string              `json:"recordSource,omitempty"`
	Seen          string              `json:"seen,omitempty"`
	Reference     *masterdata.BINData `json:"reference,omitempty"`
}

type binEnrichmentResult struct {
	enrichment.Result[enrichment.BINData]
	Master       binMasterEvidence `json:"master"`
	FieldSources map[string]string `json:"fieldSources,omitempty"`
}

type enrichmentOutput struct {
	IP    *enrichment.Result[enrichment.IPData] `json:"ip,omitempty"`
	BIN   *binEnrichmentResult                  `json:"bin,omitempty"`
	Email *enrichment.Result[emailMasterData]   `json:"email,omitempty"`
}

func (h enrichmentHandler) lookup(w http.ResponseWriter, r *http.Request) {
	tenant := h.auth.merchant(r.Header.Values("Authorization"))
	if tenant == "" {
		problem(w, http.StatusUnauthorized, "A valid demo merchant bearer token is required.")
		return
	}

	var request struct {
		IP          string `json:"ip"`
		BIN         string `json:"bin"`
		EmailDomain string `json:"emailDomain"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		problem(w, http.StatusBadRequest, "Expected valid enrichment JSON.")
		return
	}

	var ip netip.Addr
	if request.IP != "" {
		parsed, err := netip.ParseAddr(request.IP)
		if err != nil || parsed.Zone() != "" {
			problem(w, http.StatusUnprocessableEntity, "IP must be an IPv4 or IPv6 address.")
			return
		}
		ip = parsed
	}
	if request.BIN != "" && !masterdata.ValidBIN(request.BIN) {
		problem(w, http.StatusUnprocessableEntity, "BIN must contain only the first 6 to 8 digits.")
		return
	}
	var domain string
	if request.EmailDomain != "" {
		var valid bool
		domain, valid = masterdata.NormalizeDomain(request.EmailDomain)
		if !valid {
			problem(w, http.StatusUnprocessableEntity, "Supply only the email domain.")
			return
		}
	}
	if !ip.IsValid() && request.BIN == "" && domain == "" {
		problem(w, http.StatusUnprocessableEntity, "Supply an IP address, BIN, or email domain.")
		return
	}

	var output enrichmentOutput
	var local masterdata.BINLookup
	vendorBIN := request.BIN
	if request.BIN != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 150*time.Millisecond)
		found, err := masterdata.LookupBIN(ctx, h.db, request.BIN)
		cancel()
		local = found
		if err != nil {
			local.Status = "UNKNOWN"
		}
		if local.Match == "EXACT" && local.Data != nil && local.Data.BINData.Complete() {
			vendorBIN = ""
		}
	}
	if domain != "" {
		output.Email = &enrichment.Result[emailMasterData]{
			Status: "UNKNOWN", Reason: "UNAVAILABLE", Provider: "email-master", LookedUpAt: time.Now().UTC(),
		}
		if domains := h.domains(); domains != nil {
			matched, sources := domains.Lookup(domain)
			output.Email.Status, output.Email.Reason = "OK", ""
			output.Email.Data = &emailMasterData{Domain: domain, Listed: matched != "", MatchedDomain: matched, Sources: sources}
		}
	}

	providers := h.service.Enrich(r.Context(), tenant, ip, vendorBIN)
	output.IP = providers.IP
	if request.BIN != "" {
		merged := mergeBIN(local, providers.BIN)
		output.BIN = &merged
		logEnrichment("bin", &merged.Result)
	}
	logEnrichment("ip", output.IP)
	logEnrichment("email", output.Email)
	respond(w, http.StatusOK, output)
}

func mergeBIN(local masterdata.BINLookup, vendor *enrichment.Result[enrichment.BINData]) binEnrichmentResult {
	result := binEnrichmentResult{
		Result: enrichment.Result[enrichment.BINData]{Status: "UNKNOWN", Reason: "UNAVAILABLE", LookedUpAt: time.Now().UTC()},
		Master: binMasterEvidence{
			Status: local.Status, Match: local.Match, MatchedPrefix: local.MatchedPrefix, Source: local.Source, Revision: local.Revision,
		},
		FieldSources: make(map[string]string, 4),
	}
	if vendor != nil {
		result.Result = *vendor
	}
	var data masterdata.BINData
	if row := local.Data; row != nil {
		result.Master.RecordSource, result.Master.Seen = row.RecordSource, row.Seen
		if local.Match == "EXACT" {
			fillBINFields(&data, row.BINData, "postgresql", result.FieldSources)
			if result.Provider == "" {
				result.Provider = "postgresql"
			}
		} else {
			result.Master.Reference = &row.BINData
		}
	}
	if vendor != nil && vendor.Status == "OK" && vendor.Data != nil {
		fillBINFields(&data, *vendor.Data, vendor.Provider, result.FieldSources)
	}
	if len(result.FieldSources) > 0 {
		result.Data = &data
	} else {
		result.Data = nil
	}
	if data.Complete() {
		result.Status, result.Reason = "OK", ""
	} else if result.Status == "OK" {
		result.Status, result.Reason = "UNKNOWN", "INCOMPLETE"
	}
	return result
}

func fillBINFields(target *masterdata.BINData, data masterdata.BINData, source string, provenance map[string]string) {
	for _, field := range []struct {
		name  string
		dst   *string
		value string
	}{
		{"scheme", &target.Scheme, data.Scheme},
		{"funding", &target.Funding, data.Funding},
		{"issuer", &target.Issuer, data.Issuer},
		{"countryCode", &target.CountryCode, data.CountryCode},
	} {
		if *field.dst == "" && field.value != "" {
			*field.dst = field.value
			provenance[field.name] = source
		}
	}
}

func logEnrichment[T any](kind string, result *enrichment.Result[T]) {
	if result == nil {
		return
	}
	dataTimestamp := ""
	if result.DataTimestamp != nil {
		dataTimestamp = result.DataTimestamp.Format(time.RFC3339)
	}
	log.Printf("enrichment kind=%s status=%s provider=%s looked_up_at=%s data_timestamp=%q cached=%t",
		kind, result.Status, result.Provider, result.LookedUpAt.Format(time.RFC3339), dataTimestamp, result.Cached)
}
