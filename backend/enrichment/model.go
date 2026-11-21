package enrichment

import (
	"context"
	"errors"
	"time"

	"github.com/pwatakul/payguard/backend/masterdata"
)

var (
	ErrNotFound    = errors.New("lookup not found")
	ErrUnavailable = errors.New("provider unavailable")
	ErrRejected    = errors.New("provider rejected lookup")
)

type Provider[T any] struct {
	Name   string
	Lookup func(context.Context, string) (T, *time.Time, error)
}

type IPData struct {
	VPN         *bool  `json:"vpn,omitempty"`
	Proxy       *bool  `json:"proxy,omitempty"`
	Tor         *bool  `json:"tor,omitempty"`
	Datacenter  *bool  `json:"datacenter,omitempty"`
	Abuser      *bool  `json:"abuser,omitempty"`
	RiskScore   *int   `json:"riskScore,omitempty"`
	CountryCode string `json:"countryCode,omitempty"`
}

type BINData = masterdata.BINData

type Result[T any] struct {
	Status        string     `json:"status"`
	Reason        string     `json:"reason,omitempty"`
	Provider      string     `json:"provider,omitempty"`
	LookedUpAt    time.Time  `json:"lookedUpAt"`
	DataTimestamp *time.Time `json:"dataTimestamp,omitempty"`
	Cached        bool       `json:"cached"`
	Data          *T         `json:"data,omitempty"`
}
