package masterdata

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// BINData is the common issuer metadata for both the master and vendor lookups.
// Empty fields represent unavailable metadata.
type BINData struct {
	Scheme      string `json:"scheme,omitempty"`
	Funding     string `json:"funding,omitempty"`
	Issuer      string `json:"issuer,omitempty"`
	CountryCode string `json:"countryCode,omitempty"`
}

func (data BINData) Complete() bool {
	return data.Scheme != "" && data.Funding != "" && data.Issuer != "" && data.CountryCode != ""
}

func NormalizeBINData(data BINData) BINData {
	return BINData{
		Scheme:      normalizeScheme(data.Scheme),
		Funding:     normalizeFunding(data.Funding),
		Issuer:      normalizeIssuer(data.Issuer),
		CountryCode: NormalizeCountryCode(data.CountryCode),
	}
}

var (
	schemeToken      = regexp.MustCompile(`^[a-z0-9]+([_/][a-z0-9]+)*$`)
	schemeSeparators = strings.NewReplacer(" ", "_", "-", "_")
)

func normalizeScheme(scheme string) string {
	parts := strings.Split(strings.ToLower(scheme), "/")
	for i, part := range parts {
		name := strings.Join(strings.Fields(part), " ")
		if missingBINField(name) {
			return ""
		}
		switch name {
		case "master card", "master-card":
			parts[i] = "mastercard"
		case "american express", "american-express":
			parts[i] = "amex"
		case "diners", "diners club", "diners club international":
			parts[i] = "diners_club"
		case "union pay", "china unionpay", "china union pay", "cup":
			parts[i] = "unionpay"
		case "nspk mir":
			parts[i] = "mir"
		case "bankcard(inactive)":
			parts[i] = "bankcard_inactive"
		default:
			parts[i] = schemeSeparators.Replace(name)
		}
	}
	name := strings.Join(parts, "/")
	if len(name) > 64 || !schemeToken.MatchString(name) {
		return ""
	}
	return name
}

func normalizeFunding(funding string) string {
	switch value := strings.ToLower(strings.Join(strings.Fields(funding), " ")); value {
	case "credit", "debit", "prepaid", "charge":
		return value
	case "pre-paid", "pre paid":
		return "prepaid"
	case "charge card":
		return "charge"
	default:
		return ""
	}
}

func normalizeIssuer(issuer string) string {
	issuer = strings.TrimSpace(issuer)
	if missingBINField(strings.ToLower(issuer)) || utf8.RuneCountInString(issuer) > 160 ||
		strings.ContainsAny(issuer, "<>") ||
		strings.ContainsFunc(issuer, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) {
		return ""
	}
	return strings.Join(strings.Fields(issuer), " ")
}

func missingBINField(value string) bool {
	switch value {
	case "", "-", "unknown", "n/a", "n.a", "n.a.", "null", "none", "undefined", "not available", "not reported", "not provided":
		return true
	default:
		return false
	}
}

func NormalizeCountryCode(code string) string {
	code = strings.TrimSpace(code)
	if len(code) != 2 || strings.ContainsFunc(code, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z')
	}) {
		return ""
	}
	code = strings.ToUpper(code)
	if code == "XX" || code == "ZZ" {
		return ""
	}
	return code
}
