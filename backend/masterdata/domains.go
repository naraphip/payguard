package masterdata

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"slices"
	"strings"
)

const (
	SourceValidEmailChecker = "validemailchecker"
	SourceDisposable        = "disposable"
	SourceVegaStack         = "vegastack"
	SourceTempMailDetector  = "tempmaildetector"
	MaxDomainSnapshotBytes  = 5 << 20
	maxDomainTextBytes      = 32 << 20
)

//go:embed data/domains.txt.gz
var bundledDomains []byte

type Domains struct {
	records map[string][]string
}

func LoadDomains() (*Domains, error) {
	records, err := readDomainSnapshot(bytes.NewReader(bundledDomains), SourceDisposable, SourceValidEmailChecker)
	if err != nil {
		return nil, fmt.Errorf("bundled disposable domains: %w", err)
	}
	return &Domains{records: records}, nil
}

// AddLocal merges a local-only domain snapshot. Call before serving lookups.
func (d *Domains) AddLocal(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	overlay, err := readDomainSnapshot(file, SourceTempMailDetector, SourceVegaStack)
	if err != nil {
		return fmt.Errorf("local disposable domains: %w", err)
	}
	for domain, sources := range overlay {
		combined := append(d.records[domain], sources...)
		slices.Sort(combined)
		d.records[domain] = slices.Compact(combined)
	}
	return nil
}

func (d *Domains) Len() int {
	return len(d.records)
}

// Lookup returns the most specific record and that record's source IDs.
func (d *Domains) Lookup(domain string) (string, []string) {
	candidate, ok := NormalizeDomain(domain)
	if !ok {
		return "", nil
	}
	for {
		if sources, found := d.records[candidate]; found {
			return candidate, slices.Clone(sources)
		}
		_, suffix, found := strings.Cut(candidate, ".")
		if !found {
			return "", nil
		}
		candidate = suffix
	}
}

func NormalizeDomain(raw string) (string, bool) {
	domain := strings.TrimSuffix(strings.TrimSpace(raw), ".")
	if len(domain) > 253 || !strings.Contains(domain, ".") {
		return "", false
	}
	if _, err := netip.ParseAddr(domain); err == nil {
		return "", false
	}
	for label := range strings.SplitSeq(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-') {
				return "", false
			}
		}
	}
	return strings.ToLower(domain), true
}

func readDomainSnapshot(input io.Reader, allowedSources ...string) (map[string][]string, error) {
	compressed := &io.LimitedReader{R: input, N: MaxDomainSnapshotBytes + 1}
	reader, err := gzip.NewReader(compressed)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	text := &io.LimitedReader{R: reader, N: maxDomainTextBytes + 1}
	scanner := bufio.NewScanner(text)
	records := make(map[string][]string)
	previous := ""
	for scanner.Scan() {
		domain, provenance, sep := strings.Cut(scanner.Text(), "\t")
		if normalized, ok := NormalizeDomain(domain); !sep || !ok || normalized != domain || domain <= previous {
			return nil, fmt.Errorf("invalid domain record at row %d", len(records)+1)
		}
		sources := strings.Split(provenance, ",")
		priorSource := ""
		for _, source := range sources {
			if source <= priorSource || !slices.Contains(allowedSources, source) {
				return nil, fmt.Errorf("invalid source provenance at row %d", len(records)+1)
			}
			priorSource = source
		}
		records[domain] = sources
		previous = domain
	}
	if compressed.N <= 0 || text.N <= 0 {
		return nil, errors.New("domain snapshot exceeds size limit")
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("domain snapshot is empty")
	}
	return records, nil
}
