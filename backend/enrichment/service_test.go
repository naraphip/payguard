package enrichment

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type testCache struct {
	key   string
	value []byte
	ttl   time.Duration
	fail  bool
}

func (c *testCache) Get(_ context.Context, key string) ([]byte, error) {
	if c.fail || c.key != key {
		return nil, errors.New("cache miss")
	}
	return c.value, nil
}

func (c *testCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	if c.fail {
		return errors.New("cache unavailable")
	}
	c.key, c.value, c.ttl = key, value, ttl
	return nil
}

func TestBINLookupFallsBackAndCachesNormalizedResult(t *testing.T) {
	backupCalls := 0
	cache := &testCache{}
	sourceDate := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	service := Service{
		Cache: cache, CacheSecret: []byte("test-secret"),
		BINProviders: []Provider[BINData]{
			{"primary", func(context.Context, string) (BINData, *time.Time, error) {
				return BINData{}, nil, ErrNotFound
			}},
			{"backup", func(context.Context, string) (BINData, *time.Time, error) {
				backupCalls++
				return BINData{Scheme: "visa"}, &sourceDate, nil
			}},
		},
	}
	first := service.Enrich(context.Background(), "one", netip.Addr{}, "424242").BIN
	second := service.Enrich(context.Background(), "one", netip.Addr{}, "424242").BIN
	if first.Status != "OK" || first.Provider != "backup" || first.Cached ||
		first.DataTimestamp == nil || !first.DataTimestamp.Equal(sourceDate) ||
		second.Data == nil || *second.Data != *first.Data || !second.Cached ||
		!second.LookedUpAt.Equal(first.LookedUpAt) || backupCalls != 1 || cache.ttl != 24*time.Hour {
		t.Fatalf("unexpected fallback/cache results: %#v %#v, backup calls %d, TTL %v", first, second, backupCalls, cache.ttl)
	}
	if strings.Contains(cache.key, "424242") || strings.Contains(cache.key, "one") {
		t.Fatal("raw lookup value or tenant found in cache key")
	}
	if other := service.Enrich(context.Background(), "two", netip.Addr{}, "424242").BIN; other.Cached || backupCalls != 2 {
		t.Fatal("cache unexpectedly shared across tenants")
	}
}

func TestIPTimeoutUsesBackupAndDoesNotLoseKnownFalse(t *testing.T) {
	vpn := false
	cache := &testCache{fail: true}
	service := Service{
		Cache: cache, CacheSecret: []byte("test-secret"), Timeout: 10 * time.Millisecond,
		IPProviders: []Provider[IPData]{
			{"slow", func(ctx context.Context, _ string) (IPData, *time.Time, error) {
				<-ctx.Done()
				return IPData{}, nil, ctx.Err()
			}},
			{"backup", func(context.Context, string) (IPData, *time.Time, error) {
				return IPData{VPN: &vpn}, nil, nil
			}},
		},
	}
	result := service.Enrich(context.Background(), "one", netip.MustParseAddr("8.8.8.8"), "").IP
	if result.Status != "OK" || result.Provider != "backup" || result.Data == nil ||
		result.Data.VPN == nil || *result.Data.VPN || result.Data.Proxy != nil {
		t.Fatalf("unexpected backup result: %#v", result)
	}
	vpn = true
	cache.fail = false
	result = service.Enrich(context.Background(), "one", netip.MustParseAddr("8.8.8.8"), "").IP
	if result.Status != "OK" || cache.ttl != 5*time.Minute {
		t.Fatalf("positive IP should get the shorter TTL, got %#v and %v", result, cache.ttl)
	}
}

func TestPrivateReservedAndRejectedInputsDoNotUseAnotherProvider(t *testing.T) {
	calls := 0
	service := Service{IPProviders: []Provider[IPData]{
		{"primary", func(context.Context, string) (IPData, *time.Time, error) {
			calls++
			return IPData{}, nil, ErrRejected
		}},
		{"backup", func(context.Context, string) (IPData, *time.Time, error) {
			calls++
			return IPData{}, nil, nil
		}},
	}}
	for _, address := range []string{"127.0.0.1", "192.0.2.1", "198.51.100.25", "::ffff:203.0.113.12", "10.0.0.1", "100.64.0.2", "2001:db8::1", "3fff::1"} {
		result := service.Enrich(context.Background(), "one", netip.MustParseAddr(address), "").IP
		if result.Status != "UNKNOWN" || result.Reason != "NON_PUBLIC_IP" || result.Data != nil {
			t.Errorf("unexpected response for %s: %#v", address, result)
		}
	}
	if calls != 0 {
		t.Fatal("non-public IPs reached providers")
	}
	result := service.Enrich(context.Background(), "one", netip.MustParseAddr("8.8.8.8"), "").IP
	if result.Status != "UNKNOWN" || result.Reason != "REJECTED" || result.Data != nil || calls != 1 {
		t.Fatalf("rejected input should not retry: %#v; calls %d", result, calls)
	}
}
