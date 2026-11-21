package enrichment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type fixtureProviderTransport func(*http.Request) (*http.Response, error)

func (f fixtureProviderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func providerFixture(t *testing.T, status int, body string, inspect func(*http.Request)) *http.Client {
	t.Helper()
	return &http.Client{Transport: fixtureProviderTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Scheme != "https" || req.Header.Get("Accept") != "application/json" {
			t.Fatalf("unexpected request transport or Accept: %s %q", req.URL.Scheme, req.Header.Get("Accept"))
		}
		if inspect != nil {
			inspect(req)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
}

func TestIPAPIISKeyedAndAnonymousRequests(t *testing.T) {
	for _, key := range []string{"unit-test-secret", ""} {
		client := providerFixture(t, 200, `{"ip":"8.8.8.8","is_vpn":true,"is_proxy":false,"location":{"country_code":"us"}}`, func(req *http.Request) {
			if req.Method != http.MethodPost || req.URL.String() != ipapiISEndpoint || req.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected ipapi.is request shape: %s", req.Method)
			}
			var input map[string]string
			if err := json.NewDecoder(req.Body).Decode(&input); err != nil || input["q"] != "8.8.8.8" {
				t.Fatalf("unexpected ipapi.is request body")
			}
			if value, sent := input["key"]; value != key || sent != (key != "") {
				t.Fatal("ipapi.is request did not match key configuration")
			}
		})
		result, timestamp, err := IPAPIIS(client, key).Lookup(context.Background(), "8.8.8.8")
		if err != nil || timestamp != nil || result.VPN == nil || !*result.VPN || result.Proxy == nil ||
			*result.Proxy || result.CountryCode != "US" || result.RiskScore != nil || result.Abuser != nil {
			t.Fatalf("unexpected normalized ipapi.is result: %+v, %v", result, err)
		}
	}
}

func TestProxyCheckMatchesCanonicalKeyAndUsesDocumentedFields(t *testing.T) {
	client := providerFixture(t, 200, `{
		"status":"warning",
		"2606:4700:4700:0000:0000:0000:0000:1111":{
			"last_updated":"2026-10-03T10:00:00+07:00",
			"risk":0,
			"location":{"isocode":"us"},
			"detections":{"proxy":false,"vpn":true,"tor":null,"hosting":false,"compromised":true}
		}
	}`, func(req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Host != "proxycheck.io" ||
			req.URL.Path != "/v3/2606:4700:4700::1111" ||
			req.URL.Query().Get("key") != "test-key" || req.URL.Query().Get("tag") != "0" {
			t.Fatalf("unexpected ProxyCheck request shape")
		}
	})
	result, timestamp, err := ProxyCheck(client, "test-key").Lookup(context.Background(), "2606:4700:4700::1111")
	wantTime := time.Date(2026, 10, 3, 3, 0, 0, 0, time.UTC)
	if err != nil || timestamp == nil || !timestamp.Equal(wantTime) || result.RiskScore == nil ||
		*result.RiskScore != 0 || result.VPN == nil || !*result.VPN || result.Proxy == nil ||
		*result.Proxy || result.Abuser != nil || result.Tor != nil || result.CountryCode != "US" {
		t.Fatalf("unexpected normalized ProxyCheck result: %+v, %v, %v", result, timestamp, err)
	}
}

func TestBINProviderSchemasNormalizeKnownFields(t *testing.T) {
	tests := []struct {
		name, host, body string
		constructor      func(*http.Client) Provider[BINData]
		want             BINData
	}{
		{"scanner", "binscan.live", `{"success":true,"data":{"bin":"0123456","brand":"MASTER CARD","type":"PRE-PAID","bank":" Test Bank ","country":"ve","country_flag":"unused"}}`, BINScanner,
			BINData{Scheme: "mastercard", Funding: "prepaid", Issuer: "Test Bank", CountryCode: "VE"}},
		{"card Bin Check", "cardbincheck.com", `{"success":true,"data":{"bin":"0123456","scheme":"American Express","funding":"Charge","issuer":"Test Bank A/S","country_code":"dk","country_name":"unused"}}`, CardBINCheck,
			BINData{Scheme: "amex", Funding: "charge", Issuer: "Test Bank A/S", CountryCode: "DK"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := providerFixture(t, 200, tc.body, func(req *http.Request) {
				if req.Method != http.MethodGet || req.URL.Host != tc.host || req.URL.Path != "/api/bin/0123456" || req.URL.RawQuery != "" {
					t.Fatal("unexpected BIN provider request shape")
				}
			})
			result, timestamp, err := tc.constructor(client).Lookup(context.Background(), "0123456")
			if err != nil || timestamp != nil || result != tc.want {
				t.Fatalf("got %+v, %v; want %+v", result, err, tc.want)
			}
		})
	}
}

func TestProviderErrorsPreserveOnlyFailureCategories(t *testing.T) {
	tests := []struct {
		name, body string
		status     int
		provider   func(*http.Client) error
		want       error
	}{
		{"rate limit", "", 429, proxyFixtureLookup, ErrUnavailable},
		{"auth", "", 403, proxyFixtureLookup, ErrRejected},
		{"upstream", "", 503, proxyFixtureLookup, ErrUnavailable},
		{"timeout", "", 408, proxyFixtureLookup, ErrUnavailable},
		{"not found", "", 404, binFixtureLookup, ErrNotFound},
		{"redirect", "", 302, binFixtureLookup, ErrRejected},
		{"denied body", `{"status":"denied","8.8.8.8":{"risk":9}}`, 200, proxyFixtureLookup, ErrRejected},
		{"empty detections", `{"status":"ok","8.8.8.8":{"detections":{"compromised":true}}}`, 200, proxyFixtureLookup, ErrUnavailable},
		{"risk outside range", `{"status":"ok","8.8.8.8":{"risk":101}}`, 200, proxyFixtureLookup, ErrUnavailable},
		{"wrong IP", `{"status":"ok","1.1.1.1":{"risk":30}}`, 200, proxyFixtureLookup, ErrUnavailable},
		{"wrong root IP", `{"status":"ok","ip":"1.1.1.1","8.8.8.8":{"risk":30}}`, 200, proxyFixtureLookup, ErrUnavailable},
		{"known BIN missing", `{"success":false,"error":"BIN not found"}`, 200, binFixtureLookup, ErrNotFound},
		{"card BIN rate body", `{"success":false,"error":true,"reason":"rate_limit"}`, 200, cardFixtureLookup, ErrUnavailable},
		{"HTML", "<html>internal error</html>", 200, binFixtureLookup, ErrUnavailable},
		{"mismatched BIN", `{"success":true,"data":{"bin":"123457","brand":"Visa"}}`, 200, binFixtureLookup, ErrUnavailable},
		{"unrecognized BIN shape", `{"success":true,"data":{"brand":"unknown","type":"unknown","bank":"none","country":"ZZ"}}`, 200, binFixtureLookup, ErrUnavailable},
		{"malformed flag", `{"ip":"8.8.8.8","is_vpn":"yes"}`, 200, ipapiFixtureLookup, ErrUnavailable},
		{"wrong echo IP", `{"ip":"1.1.1.1","is_proxy":false}`, 200, ipapiFixtureLookup, ErrUnavailable},
		{"keyless missing fields", `{"ip":"8.8.8.8","country":"United States"}`, 200, ipapiFixtureLookup, ErrUnavailable},
		{"over limit", `{"ip":"8.8.8.8","is_vpn":false}` + strings.Repeat(" ", maxProviderResponse), 200, ipapiFixtureLookup, ErrUnavailable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.provider(providerFixture(t, tc.status, tc.body, nil)); err != tc.want {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func proxyFixtureLookup(client *http.Client) error {
	_, _, err := ProxyCheck(client, "").Lookup(context.Background(), "8.8.8.8")
	return err
}

func binFixtureLookup(client *http.Client) error {
	_, _, err := BINScanner(client).Lookup(context.Background(), "123456")
	return err
}

func cardFixtureLookup(client *http.Client) error {
	_, _, err := CardBINCheck(client).Lookup(context.Background(), "123456")
	return err
}

func ipapiFixtureLookup(client *http.Client) error {
	_, _, err := IPAPIIS(client, "").Lookup(context.Background(), "8.8.8.8")
	return err
}

func TestProviderRejectsInvalidInputAndSanitizesNetworkError(t *testing.T) {
	var calls int
	client := &http.Client{Transport: fixtureProviderTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, fmt.Errorf("network failure for %s", req.URL)
	})}
	_, _, err := BINScanner(client).Lookup(context.Background(), "4111111111111111")
	if !errors.Is(err, ErrRejected) || calls != 0 {
		t.Fatalf("unexpected outbound request for long card input, calls: %d, error: %v", calls, err)
	}
	_, _, err = ProxyCheck(client, "secret").Lookup(context.Background(), "8.8.8.8")
	if err != ErrUnavailable || calls != 1 {
		t.Fatalf("expected sanitized network failure, calls: %d", calls)
	}
}
