package main

import (
	"cmp"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand/v2"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	generatorVersion = "generator-v1"
	ruleVersion      = "rules-v1"
	jevVersion       = "jev-mock-v1"
	merchantID       = "demo-merchant"
)

var contactDomainPattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+test$`)

type config struct {
	Seed               uint64 `json:"seed"`
	Product            string `json:"product"`
	Scenario           string `json:"scenario"`
	Accounts           int    `json:"accounts"`
	Rate               int    `json:"rate"`
	Quantity           int    `json:"quantity"`
	Date               string `json:"date"`
	Delivery           string `json:"delivery"`
	RotateIP           bool   `json:"rotateIp"`
	SharedIP           bool   `json:"sharedIp"`
	SharedDevice       bool   `json:"sharedDevice"`
	SharedToken        bool   `json:"sharedToken"`
	NewAccount         bool   `json:"newAccount"`
	Retry              bool   `json:"retry"`
	MissingToken       bool   `json:"missingToken"`
	JevTimeout         bool   `json:"jevTimeout"`
	MerchantPolicy     bool   `json:"merchantPolicy"`
	ContactEmailDomain string `json:"contactEmailDomain,omitempty"`
}

func (c config) validate() error {
	if c.Seed > 9007199254740991 || c.Accounts < 1 || c.Accounts > 100 ||
		c.Rate < 1 || c.Rate > 60 || c.Quantity < 1 || c.Quantity > 20 {
		return errors.New("seed, accounts, event rate or quantity is outside its allowed range")
	}
	if domain := c.ContactEmailDomain; domain != "" && (len(domain) > 253 || !contactDomainPattern.MatchString(domain)) {
		return errors.New("Contact email must use a valid .test domain.")
	}
	switch c.Scenario {
	case "normal", "fraud", "mixed":
	default:
		return errors.New("choose normal, fraud or mixed")
	}
	switch c.Product {
	case "flight_ticket", "hotel_room":
		if _, err := time.Parse("2006-01-02", c.Date); err != nil || c.Delivery != "" {
			return errors.New("OTA requires a valid booking date and no delivery type")
		}
	case "physical", "digital":
		if c.Date != "" || (c.Product == "physical" && c.Delivery != "standard" && c.Delivery != "express") ||
			(c.Product == "digital" && c.Delivery != "instant") {
			return errors.New("select a valid delivery type for the product")
		}
	default:
		return errors.New("unknown product")
	}
	return nil
}

type entity struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

type event struct {
	ID       string   `json:"id"`
	OrderID  string   `json:"orderId"`
	Stage    string   `json:"stage"`
	At       int64    `json:"at"`
	Product  string   `json:"product"`
	Quantity int      `json:"quantity"`
	Date     string   `json:"date,omitempty"`
	Delivery string   `json:"delivery,omitempty"`
	Domain   string   `json:"domain"`
	AgeDays  int      `json:"ageDays"`
	Entities []entity `json:"entities"`
}

type label struct {
	EventID string `json:"eventId"`
	Fraud   bool   `json:"fraud"`
	RingID  string `json:"ringId,omitempty"`
}

// Labels travel on a distinct path. The scorer receives only events.
func generate(c config, tenant string, secret []byte) ([]event, []label) {
	random := rand.New(rand.NewPCG(c.Seed, 41))
	start := int64(1795219200) + int64(c.Seed%86400)
	events := make([]event, 0, c.Accounts*3)
	labels := make([]label, 0, c.Accounts)
	order := random.Perm(c.Accounts)
	fraudCount := c.Accounts / 2
	for i, rank := range order {
		fraud := c.Scenario == "fraud" || (c.Scenario == "mixed" && rank < fraudCount)
		device, token := fmt.Sprintf("device-%d", i), fmt.Sprintf("token-%d", i)
		if fraud && c.SharedDevice {
			device = "shared-device"
		}
		if fraud && c.SharedToken {
			token = "shared-token"
		}
		ip := fmt.Sprintf("192.0.2.%d", i+1)
		if fraud {
			ip = "198.51.100.250"
			if c.RotateIP {
				ip = fmt.Sprintf("198.51.100.%d", i+1)
			}
		} else if !fraud && c.SharedIP {
			ip = "203.0.113.42"
		}
		group := []entity{
			{"account", entityKey(secret, tenant, "account", fmt.Sprint(c.Seed, "/", i))},
			{"device", entityKey(secret, tenant, "device", fmt.Sprint(c.Seed, "/", device))},
			{"ip", entityKey(secret, tenant, "ip", ip)},
		}
		paymentGroup := slices.Clone(group)
		if fraud && c.RotateIP {
			paymentIP := fmt.Sprintf("198.51.100.%d", i+101)
			paymentGroup[2].Key = entityKey(secret, tenant, "ip", paymentIP)
		}
		if !c.MissingToken {
			paymentGroup = append(paymentGroup, entity{"token", entityKey(secret, tenant, "token", fmt.Sprint(c.Seed, "/", token))})
		}
		domain := "traveler.test"
		if i%4 == 0 {
			domain = "restricted.test"
		}
		if i == 0 && c.ContactEmailDomain != "" {
			domain = c.ContactEmailDomain
		}
		age := 110 + random.IntN(250)
		if c.NewAccount {
			age = random.IntN(6)
		}
		orderID := fmt.Sprintf("order-%x-%03d", c.Seed, i+1)
		first, second := "BOOKING", "PRE_PAYMENT"
		if c.Product == "physical" || c.Product == "digital" {
			first, second = "CHECKOUT", "PAYMENT"
		}
		base := event{
			ID: orderID + "-01", OrderID: orderID, Stage: first, At: start + int64(i*60/c.Rate),
			Product: c.Product, Quantity: c.Quantity, Date: c.Date, Delivery: c.Delivery,
			Domain: domain, AgeDays: age, Entities: group,
		}
		payment := base
		payment.ID, payment.Stage, payment.At, payment.Entities = orderID+"-02", second, base.At+1, paymentGroup
		events = append(events, base, payment)
		if c.Retry {
			events = append(events, payment)
		}
		truth := label{EventID: payment.ID, Fraud: fraud}
		if fraud {
			truth.RingID = fmt.Sprintf("ring-%x", c.Seed)
		}
		labels = append(labels, truth)
	}
	// Deliver in reverse order to exercise event-time ordering on every run.
	slices.Reverse(events)
	return events, labels
}

func entityKey(secret []byte, tenant, kind, value string) string {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s\x00%s\x00%s", tenant, kind, value)
	return hex.EncodeToString(mac.Sum(nil))
}

type feature struct {
	Kind   string `json:"kind"`
	Orders int64  `json:"orders"`
}

type decision struct {
	Event     event     `json:"event"`
	Action    string    `json:"action"`
	Source    string    `json:"actionSource"`
	Platform  string    `json:"platformDecision"`
	Merchant  string    `json:"merchantPolicyDecision"`
	RuleHits  []string  `json:"ruleHits"`
	JevStatus string    `json:"jevStatus"`
	JevAnswer string    `json:"jevAnswer"`
	Missing   []string  `json:"missing"`
	Features  []feature `json:"features"`
}

type velocityCounter interface {
	count(context.Context, event) ([]feature, error)
}

func evaluate(ctx context.Context, counter velocityCounter, incoming []event, policy, timeout bool) ([]decision, error) {
	slices.SortFunc(incoming, func(a, b event) int {
		return cmp.Or(cmp.Compare(a.At, b.At), cmp.Compare(a.ID, b.ID))
	})
	seen := make(map[string]event)
	results := make([]decision, 0, len(incoming))
	for _, e := range incoming {
		if prior, ok := seen[e.ID]; ok {
			if !slices.Equal(prior.Entities, e.Entities) || !sameEvent(prior, e) {
				return nil, fmt.Errorf("conflicting duplicate event %s", e.ID)
			}
			continue
		}
		seen[e.ID] = e
		features, err := counter.count(ctx, e)
		if err != nil {
			return nil, err
		}
		results = append(results, assess(e, features, policy, timeout))
	}
	return results, nil
}

func sameEvent(a, b event) bool {
	return a.OrderID == b.OrderID && a.Stage == b.Stage && a.At == b.At &&
		a.Product == b.Product && a.Quantity == b.Quantity && a.Date == b.Date &&
		a.Delivery == b.Delivery && a.Domain == b.Domain && a.AgeDays == b.AgeDays
}

func paymentStage(stage string) bool { return stage == "PRE_PAYMENT" || stage == "PAYMENT" }

func assess(e event, features []feature, policy, timeout bool) decision {
	d := decision{
		Event: e, Action: "ALLOW", Source: "PLATFORM", Platform: "ALLOW", Merchant: "NOT_APPLICABLE",
		JevStatus: "MOCK", JevAnswer: "LOW", RuleHits: []string{}, Missing: []string{}, Features: features,
	}
	var device, token int64
	for _, f := range features {
		switch f.Kind {
		case "device":
			device = f.Orders
		case "token":
			token = f.Orders
		}
	}
	if paymentStage(e.Stage) && token == 0 {
		d.Missing = append(d.Missing, "payment_token")
	}
	if token >= 2 && device >= 2 {
		d.RuleHits = append(d.RuleHits, "SHARED_DEVICE_AND_TOKEN")
		d.Platform, d.JevAnswer = "REVIEW", "HIGH"
	} else if (token >= 2 || device >= 2) && e.AgeDays <= 7 {
		d.RuleHits = append(d.RuleHits, "NEW_ACCOUNT_SHARED_INSTRUMENT_OR_DEVICE")
		d.Platform, d.JevAnswer = "CHALLENGE", "MEDIUM"
	} else if len(d.Missing) > 0 {
		d.RuleHits = append(d.RuleHits, "PAYMENT_TOKEN_MISSING")
		d.Platform, d.JevAnswer = "REVIEW", "INSUFFICIENT_EVIDENCE"
	}
	d.Action = d.Platform
	if timeout {
		d.JevStatus, d.JevAnswer, d.Action, d.Source = "TIMEOUT", "UNAVAILABLE", "UNSUCCESSFUL", "JEV_TIMEOUT"
	}
	if policy && e.Product == "flight_ticket" && paymentStage(e.Stage) {
		d.Merchant = "PASS"
		if strings.EqualFold(e.Domain, "restricted.test") {
			d.Merchant = "BLOCK"
			if !timeout {
				d.Action, d.Source = "BLOCK", "MERCHANT_POLICY"
			}
		}
	}
	return d
}
