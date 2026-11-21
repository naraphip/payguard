package masterdata

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeDomain(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{" ExampLe.COM. ", "example.com"},
		{"A.Xn--bcher-kva.example", "a.xn--bcher-kva.example"},
		{"mail01.example.com", "mail01.example.com"},
		{"", ""}, {"localhost", ""}, {"..example.com", ""}, {"example.com..", ""},
		{"a@disposable.example", ""}, {"https://disposable.example", ""}, {"*.example.com", ""},
		{"127.0.0.1", ""}, {"bücher.example", ""}, {"sK.example", ""},
		{"foo_bar.example", ""}, {"bad-.example", ""}, {"-bad.example", ""},
		{strings.Repeat("a", 64) + ".example", ""},
	} {
		got, ok := NormalizeDomain(tc.input)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("NormalizeDomain(%q) = %q, %t, want %q", tc.input, got, ok, tc.want)
		}
	}
}

func TestBundledDomainLookupAndLabelBoundary(t *testing.T) {
	data, err := LoadDomains()
	if err != nil {
		t.Fatal(err)
	}
	if data.Len() < 200_000 {
		t.Fatalf("expected full embedded feeds, got %d rows", data.Len())
	}
	for _, input := range []string{"MAILINATOR.COM.", "delivery.mailinator.com"} {
		match, sources := data.Lookup(input)
		if match != "mailinator.com" || !reflect.DeepEqual(sources, []string{SourceDisposable, SourceValidEmailChecker}) {
			t.Fatalf("Lookup(%q) = %q, %v", input, match, sources)
		}
	}
	for _, input := range []string{"not-mailinator.com", "mailinator.com.example", "user@mailinator.com", "gmail.com"} {
		if match, sources := data.Lookup(input); match != "" || sources != nil {
			t.Fatalf("unexpected match for %q: %q, %v", input, match, sources)
		}
	}
}

func TestLocalDomainOverlayMergesProvenanceAndPrefersSpecificMatch(t *testing.T) {
	data := &Domains{records: map[string][]string{"mailinator.com": {SourceDisposable, SourceValidEmailChecker}}}
	path := filepath.Join(t.TempDir(), "domains.txt.gz")
	fixture := "local.example\ttempmaildetector\nmailinator.com\tvegastack\nmx.mailinator.com\ttempmaildetector\n"
	if err := os.WriteFile(path, gzipDomainFixture(t, fixture), 0600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := data.AddLocal(path); err != nil {
			t.Fatal(err)
		}
	}
	if data.Len() != 3 {
		t.Fatalf("expected three distinct domains, got %d", data.Len())
	}
	match, sources := data.Lookup("child.mx.mailinator.com")
	if match != "mx.mailinator.com" || !reflect.DeepEqual(sources, []string{SourceTempMailDetector}) {
		t.Fatalf("most-specific match = %q, %v", match, sources)
	}
	_, sources = data.Lookup("mailinator.com")
	if !reflect.DeepEqual(sources, []string{SourceDisposable, SourceValidEmailChecker, SourceVegaStack}) {
		t.Fatalf("combined provenance = %v", sources)
	}
	sources[0] = "changed"
	if _, current := data.Lookup("mailinator.com"); current[0] != SourceDisposable {
		t.Fatalf("lookup exposed mutable source records: %v", current)
	}
}

func TestLocalDomainOverlayRejectsInvalidRowsWithoutChangingData(t *testing.T) {
	for _, malformed := range []string{
		"new.example\tdisposable\n",
		"new.example\tvegastack,tempmaildetector\n",
		"new.example\tvegastack,vegastack\n",
		"new.example\tvegastack\nnew.example\tvegastack\n",
		"valid.example\tvegastack\nNEW.example\tvegastack\n",
	} {
		data := &Domains{records: make(map[string][]string)}
		path := filepath.Join(t.TempDir(), "domains.txt.gz")
		if err := os.WriteFile(path, gzipDomainFixture(t, malformed), 0600); err != nil {
			t.Fatal(err)
		}
		if err := data.AddLocal(path); err == nil || data.Len() != 0 {
			t.Fatalf("expected atomic rejection for fixture %q, got %d records: %v", malformed, data.Len(), err)
		}
	}
}

func gzipDomainFixture(t *testing.T, content string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
