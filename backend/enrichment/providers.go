package enrichment

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"time"
	"unicode/utf8"

	"github.com/pwatakul/payguard/backend/masterdata"
)

const (
	ipapiISEndpoint      = "https://api.ipapi.is/"
	proxyCheckEndpoint   = "https://proxycheck.io/v3/"
	binScannerEndpoint   = "https://binscan.live/api/bin/"
	cardBINCheckEndpoint = "https://cardbincheck.com/api/bin/"
	maxProviderResponse  = 64 << 10
)

// decodeProviderResponse returns categorical errors so request URLs, API keys and
// upstream response bodies never leave the adapters.
func decodeProviderResponse(ctx context.Context, client *http.Client, method, endpoint string, body []byte, target any) error {
	if client == nil {
		return ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return ErrRejected
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	switch status := response.StatusCode; {
	case status == http.StatusNotFound:
		return ErrNotFound
	case status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status >= 500:
		return ErrUnavailable
	case status < 200 || status >= 300:
		return ErrRejected
	}

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxProviderResponse+1))
	if err != nil || len(raw) > maxProviderResponse || !utf8.Valid(raw) || json.Unmarshal(raw, target) != nil {
		return ErrUnavailable
	}
	return nil
}

func providerAddress(text string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(text)
	if err != nil || addr.Zone() != "" {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func sameProviderAddress(text string, expected netip.Addr) bool {
	actual, valid := providerAddress(text)
	return valid && actual == expected
}

func hasIPData(data IPData) bool {
	return data.VPN != nil || data.Proxy != nil || data.Tor != nil || data.Datacenter != nil ||
		data.Abuser != nil || data.RiskScore != nil || data.CountryCode != ""
}

// IPAPIIS uses the documented single-address POST to keep the credential out of the URL.
// See https://ipapi.is/developers.html.
func IPAPIIS(client *http.Client, key string) Provider[IPData] {
	return Provider[IPData]{Name: "ipapi.is", Lookup: func(ctx context.Context, input string) (IPData, *time.Time, error) {
		address, valid := providerAddress(input)
		if !valid {
			return IPData{}, nil, ErrRejected
		}
		parameters := struct {
			Query string `json:"q"`
			Key   string `json:"key,omitempty"`
		}{address.String(), key}
		body, err := json.Marshal(parameters)
		if err != nil {
			return IPData{}, nil, ErrRejected
		}
		var response struct {
			IP        *string `json:"ip"`
			Error     string  `json:"error"`
			ErrorCode string  `json:"error_code"`
			VPN       *bool   `json:"is_vpn"`
			Proxy     *bool   `json:"is_proxy"`
			Tor       *bool   `json:"is_tor"`
			Hosting   *bool   `json:"is_datacenter"`
			Abuser    *bool   `json:"is_abuser"`
			Location  struct {
				CountryCode string `json:"country_code"`
			} `json:"location"`
		}
		if err := decodeProviderResponse(ctx, client, http.MethodPost, ipapiISEndpoint, body, &response); err != nil {
			return IPData{}, nil, err
		}
		switch response.ErrorCode {
		case "":
			if response.Error != "" {
				return IPData{}, nil, ErrUnavailable
			}
		case "ERR_QUOTA_EXCEEDED", "ERR_FREE_TIER_EXHAUSTED", "ERR_INTERNAL":
			return IPData{}, nil, ErrUnavailable
		default:
			return IPData{}, nil, ErrRejected
		}
		if response.IP != nil && !sameProviderAddress(*response.IP, address) {
			return IPData{}, nil, ErrUnavailable
		}
		result := IPData{
			VPN: response.VPN, Proxy: response.Proxy, Tor: response.Tor, Datacenter: response.Hosting,
			Abuser: response.Abuser, CountryCode: masterdata.NormalizeCountryCode(response.Location.CountryCode),
		}
		if !hasIPData(result) {
			return IPData{}, nil, ErrUnavailable
		}
		return result, nil, nil
	}}
}

// ProxyCheck uses the stable v3 API result keyed by the queried address.
// See https://proxycheck.io/api/?db=1 and https://proxycheck.io/resources/proxycheck-openapi.yaml.
func ProxyCheck(client *http.Client, key string) Provider[IPData] {
	return Provider[IPData]{Name: "proxycheck.io", Lookup: func(ctx context.Context, input string) (IPData, *time.Time, error) {
		address, valid := providerAddress(input)
		if !valid {
			return IPData{}, nil, ErrRejected
		}
		query := url.Values{"tag": {"0"}}
		if key != "" {
			query.Set("key", key)
		}
		endpoint := proxyCheckEndpoint + url.PathEscape(address.String()) + "?" + query.Encode()
		var response map[string]json.RawMessage
		if err := decodeProviderResponse(ctx, client, http.MethodGet, endpoint, nil, &response); err != nil {
			return IPData{}, nil, err
		}
		var status string
		if json.Unmarshal(response["status"], &status) != nil {
			return IPData{}, nil, ErrUnavailable
		}
		switch status {
		case "ok", "warning":
		case "denied", "error":
			return IPData{}, nil, ErrRejected
		default:
			return IPData{}, nil, ErrUnavailable
		}
		if echo, exists := response["ip"]; exists && string(echo) != "null" {
			var text string
			if json.Unmarshal(echo, &text) != nil || !sameProviderAddress(text, address) {
				return IPData{}, nil, ErrUnavailable
			}
		}
		raw, found := response[address.String()]
		if !found {
			for candidate, payload := range response {
				if sameProviderAddress(candidate, address) {
					raw = payload
					break
				}
			}
		}
		var data struct {
			Risk       *int       `json:"risk"`
			Updated    *time.Time `json:"last_updated"`
			Detections struct {
				VPN     *bool `json:"vpn"`
				Proxy   *bool `json:"proxy"`
				Tor     *bool `json:"tor"`
				Hosting *bool `json:"hosting"`
			} `json:"detections"`
			Location struct {
				CountryCode string `json:"isocode"`
			} `json:"location"`
		}
		if json.Unmarshal(raw, &data) != nil || (data.Risk != nil && (*data.Risk < 0 || *data.Risk > 100)) {
			return IPData{}, nil, ErrUnavailable
		}
		result := IPData{
			VPN: data.Detections.VPN, Proxy: data.Detections.Proxy, Tor: data.Detections.Tor,
			Datacenter: data.Detections.Hosting, RiskScore: data.Risk,
			CountryCode: masterdata.NormalizeCountryCode(data.Location.CountryCode),
		}
		if !hasIPData(result) {
			return IPData{}, nil, ErrUnavailable
		}
		if data.Updated != nil {
			utc := data.Updated.UTC()
			data.Updated = &utc
		}
		return result, data.Updated, nil
	}}
}

func normalizedBIN(bin, scheme, funding, issuer, country string, echo *string) (BINData, error) {
	if (echo == nil && len(bin) > 6) || (echo != nil && *echo != bin) {
		return BINData{}, ErrUnavailable
	}
	data := masterdata.NormalizeBINData(BINData{
		Scheme: scheme, Funding: funding, Issuer: issuer, CountryCode: country,
	})
	if data == (BINData{}) {
		return BINData{}, ErrUnavailable
	}
	return data, nil
}

// BINScanner maps the success envelope and brand, type, bank and country fields
// documented at https://binscan.live/api.
func BINScanner(client *http.Client) Provider[BINData] {
	return Provider[BINData]{Name: "binscan.live", Lookup: func(ctx context.Context, bin string) (BINData, *time.Time, error) {
		if !masterdata.ValidBIN(bin) {
			return BINData{}, nil, ErrRejected
		}
		var response struct {
			Success *bool  `json:"success"`
			Error   string `json:"error"`
			Data    struct {
				BIN     *string `json:"bin"`
				Brand   string  `json:"brand"`
				Funding string  `json:"type"`
				Bank    string  `json:"bank"`
				Country string  `json:"country"`
			} `json:"data"`
		}
		if err := decodeProviderResponse(ctx, client, http.MethodGet, binScannerEndpoint+bin, nil, &response); err != nil {
			return BINData{}, nil, err
		}
		if response.Success == nil {
			return BINData{}, nil, ErrUnavailable
		}
		if !*response.Success {
			switch response.Error {
			case "BIN not found":
				return BINData{}, nil, ErrNotFound
			case "BIN must be 6-8 digits":
				return BINData{}, nil, ErrRejected
			default:
				return BINData{}, nil, ErrUnavailable
			}
		}
		if response.Error != "" {
			return BINData{}, nil, ErrUnavailable
		}
		data, err := normalizedBIN(bin, response.Data.Brand, response.Data.Funding, response.Data.Bank, response.Data.Country, response.Data.BIN)
		return data, nil, err
	}}
}

// CardBINCheck maps the success and error envelopes defined at
// https://cardbincheck.com/openapi.json.
func CardBINCheck(client *http.Client) Provider[BINData] {
	return Provider[BINData]{Name: "cardbincheck.com", Lookup: func(ctx context.Context, bin string) (BINData, *time.Time, error) {
		if !masterdata.ValidBIN(bin) {
			return BINData{}, nil, ErrRejected
		}
		var response struct {
			Success *bool  `json:"success"`
			Error   bool   `json:"error"`
			Reason  string `json:"reason"`
			Data    struct {
				BIN         *string `json:"bin"`
				Scheme      string  `json:"scheme"`
				Funding     string  `json:"funding"`
				Issuer      string  `json:"issuer"`
				CountryCode string  `json:"country_code"`
			} `json:"data"`
		}
		if err := decodeProviderResponse(ctx, client, http.MethodGet, cardBINCheckEndpoint+bin, nil, &response); err != nil {
			return BINData{}, nil, err
		}
		if response.Success == nil {
			return BINData{}, nil, ErrUnavailable
		}
		if !*response.Success {
			switch response.Reason {
			case "not_found":
				return BINData{}, nil, ErrNotFound
			case "rate_limit", "daily_limit":
				return BINData{}, nil, ErrUnavailable
			case "invalid_input":
				return BINData{}, nil, ErrRejected
			default:
				return BINData{}, nil, ErrUnavailable
			}
		}
		if response.Error || response.Reason != "" {
			return BINData{}, nil, ErrUnavailable
		}
		data, err := normalizedBIN(bin, response.Data.Scheme, response.Data.Funding, response.Data.Issuer, response.Data.CountryCode, response.Data.BIN)
		return data, nil, err
	}}
}
