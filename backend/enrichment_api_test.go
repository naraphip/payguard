package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pwatakul/payguard/backend/enrichment"
	"github.com/pwatakul/payguard/backend/masterdata"
)

func TestEnrichmentRequiresMerchantRole(t *testing.T) {
	auth, err := newBookingAuth("merchant-a-fixture", "",
		bookingActorCredential{"demo-merchant-a", "analyst", bookingRoleAnalyst, "analyst-a-fixture"},
		bookingActorCredential{"demo-merchant-a", "viewer", bookingRoleViewer, "viewer-a-fixture"},
	)
	if err != nil {
		t.Fatal(err)
	}
	handler := enrichmentHandler{auth: auth}
	for _, tc := range []struct {
		token  string
		status int
	}{
		{"", http.StatusUnauthorized},
		{"analyst-a-fixture", http.StatusForbidden},
		{"viewer-a-fixture", http.StatusForbidden},
		{"merchant-a-fixture", http.StatusUnprocessableEntity},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/enrichment", strings.NewReader("{}"))
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		handler.lookup(w, req)
		if w.Code != tc.status {
			t.Errorf("got unexpected enrichment status %d, want %d", w.Code, tc.status)
		}
	}
}

func TestBINMergeUsesOneRecordAndFieldProvenance(t *testing.T) {
	local := masterdata.BINLookup{
		Status: "FOUND", Match: "EXACT", MatchedPrefix: "12345678", Source: "master",
		Data: &masterdata.BINRecord{
			BINData: masterdata.BINData{Scheme: "visa", Funding: "debit", CountryCode: "TH"},
		},
	}
	vendor := enrichment.Result[enrichment.BINData]{
		Status: "OK", Provider: "backup", LookedUpAt: time.Now(),
		Data: &enrichment.BINData{Scheme: "mastercard", Funding: "credit", Issuer: "Example Bank", CountryCode: "US"},
	}
	result := mergeBIN(local, &vendor)
	want := masterdata.BINData{Scheme: "visa", Funding: "debit", Issuer: "Example Bank", CountryCode: "TH"}
	if result.Data == nil || *result.Data != want || result.Status != "OK" ||
		result.FieldSources["scheme"] != "postgresql" || result.FieldSources["issuer"] != "backup" ||
		result.Master.Reference != nil {
		t.Fatalf("unexpected canonical merged record: %+v", result)
	}
}

func TestBINMergeKeepsCoarseReferenceOutOfExactRecord(t *testing.T) {
	local := masterdata.BINLookup{
		Status: "FOUND", Match: "PREFIX", MatchedPrefix: "123456", Source: "master",
		Data: &masterdata.BINRecord{
			BINData: masterdata.BINData{Scheme: "visa", Funding: "debit", CountryCode: "TH", Issuer: "Prefix Bank"},
		},
	}
	vendor := enrichment.Result[enrichment.BINData]{
		Status: "OK", Provider: "backup", LookedUpAt: time.Now(),
		Data: &enrichment.BINData{Scheme: "mastercard"},
	}
	result := mergeBIN(local, &vendor)
	if result.Status != "UNKNOWN" || result.Reason != "INCOMPLETE" ||
		result.Data == nil || result.Data.Issuer != "" ||
		result.Master.Reference == nil || result.Master.Reference.Issuer != "Prefix Bank" ||
		len(result.FieldSources) != 1 {
		t.Fatalf("coarse evidence leaked into canonical record: %+v", result)
	}
	if result := mergeBIN(local, nil); result.Data != nil || result.Status != "UNKNOWN" || result.Master.Reference == nil {
		t.Fatalf("coarse evidence treated as confirmed data: %+v", result)
	}
}
