package main

import (
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pwatakul/payguard/backend/masterdata"
)

const maxFeedBytes = 32 << 20

type feed struct {
	source, endpoint string
	csv              bool
}

var bundledFeeds = []feed{
	{
		source:   masterdata.SourceValidEmailChecker,
		endpoint: "https://raw.githubusercontent.com/Validemailchecker/disposable-email-domains/08a0de67bb0cce3e46c6843cba07d5776091e4f6/data/domains.txt",
	},
	{
		source:   masterdata.SourceDisposable,
		endpoint: "https://raw.githubusercontent.com/disposable/disposable-email-domains/41c4cec232e520bcda8bc9afeb9f8c42e58213d9/domains.txt",
	},
}

var localFeeds = []feed{
	{
		source:   masterdata.SourceVegaStack,
		endpoint: "https://raw.githubusercontent.com/vegastack/disposable-emails-detector/109659eaa86a1ac753dfd0813cac177688613fca/outputs/disposable_email_domains.csv",
		csv:      true,
	},
	{
		source:   masterdata.SourceTempMailDetector,
		endpoint: "https://raw.githubusercontent.com/TempMailDetector/Temporary-Email-Domain-Blocklist/11b6b72778bdc9543af1e09fd0602916396c7c08/domains.txt",
	},
}

func main() {
	output := flag.String("out", "masterdata/data/domains.txt.gz", "Bundled snapshot output path, relative to the backend module")
	local := flag.Bool("local", false, "Also refresh local-only domains from VegaStack and TempMailDetector")
	flag.Parse()

	// Keep overlays in the Git-ignored directory and outside the bundled output.
	localPath := "masterdata/local/domains.txt.gz"
	bundledPath, err := filepath.Abs(*output)
	if err != nil {
		log.Fatal(err)
	}
	overlayPath, err := filepath.Abs(localPath)
	if err != nil {
		log.Fatal(err)
	}
	if bundledPath == overlayPath {
		log.Fatal("bundled and local output paths must differ")
	}
	client := &http.Client{
		Timeout:       60 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	if err := refresh(client, bundledFeeds, bundledPath, 0644); err != nil {
		log.Fatal(err)
	}
	if *local {
		if err := refresh(client, localFeeds, overlayPath, 0644); err != nil {
			log.Fatal(err)
		}
	}
}

func refresh(client *http.Client, feeds []feed, path string, mode os.FileMode) error {
	domains := make(map[string][]string)
	for _, item := range feeds {
		values, err := readFeed(client, item)
		if err != nil {
			return fmt.Errorf("%s: %w", item.source, err)
		}
		for domain := range values {
			domains[domain] = append(domains[domain], item.source)
		}
		fmt.Printf("%s: %d domains\n", item.source, len(values))
	}
	if err := writeSnapshot(path, domains, mode); err != nil {
		return err
	}
	fmt.Printf("%s: %d distinct domains\n", path, len(domains))
	return nil
}

func readFeed(client *http.Client, item feed) (map[string]struct{}, error) {
	response, err := client.Get(item.endpoint)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned HTTP %d", response.StatusCode)
	}
	input := &io.LimitedReader{R: response.Body, N: maxFeedBytes + 1}
	domains := make(map[string]struct{})
	record := func(value string) {
		if normalized, ok := masterdata.NormalizeDomain(value); ok {
			domains[normalized] = struct{}{}
		}
	}
	if item.csv {
		reader := csv.NewReader(input)
		reader.FieldsPerRecord = 1
		header, err := reader.Read()
		if err != nil || !strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(header[0]), "\ufeff"), "domain") {
			return nil, errors.New("expected a one-column domain CSV")
		}
		for {
			row, err := reader.Read()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			record(row[0])
		}
	} else {
		scanner := bufio.NewScanner(input)
		for scanner.Scan() {
			record(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}
	if input.N <= 0 {
		return nil, errors.New("feed exceeds size limit")
	}
	if len(domains) == 0 {
		return nil, errors.New("feed contains no valid domains")
	}
	return domains, nil
}

func writeSnapshot(path string, domains map[string][]string, mode os.FileMode) error {
	keys := make([]string, 0, len(domains))
	for domain := range domains {
		keys = append(keys, domain)
	}
	slices.Sort(keys)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".domain-snapshot-*.gz")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	compressed, err := gzip.NewWriterLevel(file, gzip.BestCompression)
	if err != nil {
		return err
	}
	text := bufio.NewWriter(compressed)
	for _, domain := range keys {
		sources := domains[domain]
		slices.Sort(sources)
		if _, err := fmt.Fprintf(text, "%s\t%s\n", domain, strings.Join(slices.Compact(sources), ",")); err != nil {
			return err
		}
	}
	if err := text.Flush(); err != nil {
		return err
	}
	if err := compressed.Close(); err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > masterdata.MaxDomainSnapshotBytes {
		return errors.New("compressed snapshot exceeds size limit")
	}
	if err := file.Chmod(mode); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
