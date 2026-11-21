package main

import (
	"errors"
	"strings"
	"time"
)

type simulationProductKind string

const (
	travelProduct  simulationProductKind = "travel"
	storeProduct   simulationProductKind = "store"
	gatewayProduct simulationProductKind = "gateway"
)

type simulationProductModule struct {
	kind        simulationProductKind
	firstStage  string
	secondStage string
}

var simulationProducts = map[string]simulationProductModule{
	"flight_ticket":   {travelProduct, "BOOKING", "PRE_PAYMENT"},
	"hotel_room":      {travelProduct, "BOOKING", "PRE_PAYMENT"},
	"physical":        {storeProduct, "CHECKOUT", "PAYMENT"},
	"digital":         {storeProduct, "CHECKOUT", "PAYMENT"},
	"payment_gateway": {gatewayProduct, "CHECKOUT", "PAYMENT"},
}

func validateSimulationProduct(c config) error {
	switch simulationProducts[c.Product].kind {
	case travelProduct:
		if _, err := time.Parse("2006-01-02", c.Date); err != nil || c.Delivery != "" {
			return errors.New("OTA requires a valid booking date and no delivery type")
		}
	case storeProduct:
		if c.Date != "" || (c.Product == "physical" && c.Delivery != "standard" && c.Delivery != "express") ||
			(c.Product == "digital" && c.Delivery != "instant") {
			return errors.New("select a valid delivery type for the product")
		}
	case gatewayProduct:
		if c.Date != "" || c.Delivery != "" {
			return errors.New("gateway requires no booking date or delivery type")
		}
	default:
		return errors.New("unknown product")
	}
	return nil
}

// Resolve merchant policy independently from the platform risk decision.
func resolveSimulationMerchantPolicy(e event, enabled bool) string {
	if !enabled || e.Product != "flight_ticket" || !paymentStage(e.Stage) {
		return "NOT_APPLICABLE"
	}
	if strings.EqualFold(e.Domain, "restricted.test") {
		return "BLOCK"
	}
	return "PASS"
}
