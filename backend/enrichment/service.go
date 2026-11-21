package enrichment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	RequestTimeout = 800 * time.Millisecond
	cacheTimeout   = 100 * time.Millisecond
)

type Cache interface {
	Get(context.Context, string) ([]byte, error)
	Set(context.Context, string, []byte, time.Duration) error
}

type RedisCache struct {
	Client *redis.Client
}

func (cache RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	return cache.Client.Get(ctx, key).Bytes()
}

func (cache RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return cache.Client.Set(ctx, key, value, ttl).Err()
}

type Service struct {
	IPProviders  []Provider[IPData]
	BINProviders []Provider[BINData]
	Cache        Cache
	CacheSecret  []byte
	Timeout      time.Duration
}

type Response struct {
	IP  *Result[IPData]  `json:"ip,omitempty"`
	BIN *Result[BINData] `json:"bin,omitempty"`
}

func (s Service) Enrich(ctx context.Context, tenant string, ip netip.Addr, bin string) Response {
	var result Response
	var pending sync.WaitGroup
	if ip.IsValid() {
		pending.Go(func() {
			addr := ip.Unmap()
			if !publicIP(addr) {
				result.IP = &Result[IPData]{Status: "UNKNOWN", Reason: "NON_PUBLIC_IP", LookedUpAt: time.Now().UTC()}
				return
			}
			result.IP = lookup(ctx, s, tenant, "ip", addr.String(), s.IPProviders, false, ipTTL)
		})
	}
	if bin != "" {
		pending.Go(func() {
			result.BIN = lookup(ctx, s, tenant, "bin", bin, s.BINProviders, true, func(BINData) time.Duration { return 24 * time.Hour })
		})
	}
	pending.Wait()
	return result
}

func ipTTL(data IPData) time.Duration {
	for _, flag := range []*bool{data.VPN, data.Proxy, data.Tor, data.Datacenter, data.Abuser} {
		if flag != nil && *flag {
			return 5 * time.Minute
		}
	}
	if data.RiskScore != nil && *data.RiskScore >= 60 {
		return 5 * time.Minute
	}
	return 15 * time.Minute
}

func (s Service) cacheKey(tenant, kind, input string) string {
	digest := hmac.New(sha256.New, s.CacheSecret)
	fmt.Fprintf(digest, "%s\x00%s\x00%s", tenant, kind, input)
	return "payguard:enrichment:v2:" + kind + ":" + hex.EncodeToString(digest.Sum(nil))
}

func lookup[T any](ctx context.Context, s Service, tenant, kind, input string, providers []Provider[T], fallbackNotFound bool, ttl func(T) time.Duration) *Result[T] {
	var key string
	if s.Cache != nil && len(s.CacheSecret) > 0 {
		key = s.cacheKey(tenant, kind, input)
		cacheCtx, cancel := context.WithTimeout(ctx, cacheTimeout)
		payload, err := s.Cache.Get(cacheCtx, key)
		cancel()
		var cached Result[T]
		if err == nil && json.Unmarshal(payload, &cached) == nil &&
			cached.Status == "OK" && cached.Provider != "" && cached.Data != nil && !cached.LookedUpAt.IsZero() {
			cached.Cached = true
			return &cached
		}
	}

	timeout := s.Timeout
	if timeout <= 0 {
		timeout = RequestTimeout
	}
	result := &Result[T]{Status: "UNKNOWN", Reason: "UNAVAILABLE"}
	for _, provider := range providers {
		if ctx.Err() != nil {
			break
		}
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		data, timestamp, err := provider.Lookup(callCtx, input)
		if callCtx.Err() != nil {
			err = ErrUnavailable
		}
		cancel()
		result.Provider = provider.Name
		if err == nil {
			result.Status, result.Reason = "OK", ""
			result.LookedUpAt, result.DataTimestamp, result.Data = time.Now().UTC(), timestamp, &data
			if key != "" && ctx.Err() == nil {
				if payload, err := json.Marshal(result); err == nil {
					cacheCtx, cancel := context.WithTimeout(ctx, cacheTimeout)
					_ = s.Cache.Set(cacheCtx, key, payload, ttl(data))
					cancel()
				}
			}
			return result
		}
		switch {
		case errors.Is(err, ErrNotFound):
			result.Reason = "NOT_FOUND"
			if fallbackNotFound {
				continue
			}
		case errors.Is(err, ErrUnavailable):
			result.Reason = "UNAVAILABLE"
			continue
		default:
			result.Reason = "REJECTED"
		}
		break
	}
	result.LookedUpAt = time.Now().UTC()
	return result
}

var reservedIPRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
}

var ipv6Global = netip.MustParsePrefix("2000::/3")

func publicIP(ip netip.Addr) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.Zone() != "" || (ip.Is6() && !ipv6Global.Contains(ip)) {
		return false
	}
	for _, reserved := range reservedIPRanges {
		if reserved.Contains(ip) {
			return false
		}
	}
	return true
}
