package main

import "testing"

func TestGatewayModuleAndMerchantPolicyAttribution(t *testing.T) {
	c := config{Product: "payment_gateway", Scenario: "normal", Accounts: 1, Rate: 1, Quantity: 1}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	events, _ := generate(c, "gateway-test", []byte("secret"))
	if events[0].Stage != "PAYMENT" || events[1].Stage != "CHECKOUT" {
		t.Fatalf("gateway stages: %v", events)
	}
	c.Delivery = "instant"
	if c.validate() == nil {
		t.Fatal("gateway accepted delivery")
	}
	flight := event{Product: "flight_ticket", Stage: "PRE_PAYMENT", Domain: "restricted.test"}
	if d := assess(flight, nil, true, false); d.Merchant != "BLOCK" || d.Source != "MERCHANT_POLICY" {
		t.Fatalf("expected exact merchant policy attribution: %+v", d)
	}
	flight.Domain = "sub.restricted.test"
	if d := assess(flight, nil, true, false); d.Merchant != "PASS" || d.Source != "PLATFORM" {
		t.Fatalf("subdomain should use platform decision: %+v", d)
	}
}
