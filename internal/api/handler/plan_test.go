package handler

import (
	"encoding/json"
	"testing"
)

func TestCreatePlanRequestAcceptsNumericAndStringPrices(t *testing.T) {
	var req CreatePlanRequest
	err := json.Unmarshal([]byte(`{
		"name":"Basic",
		"duration_days":30,
		"price_monthly":"1.5",
		"price_quarterly":2,
		"price_yearly":"2.44"
	}`), &req)
	if err != nil {
		t.Fatalf("unmarshal plan request: %v", err)
	}

	if got := float64(req.PriceMonthly); got != 1.5 {
		t.Fatalf("price_monthly = %v, want 1.5", got)
	}
	if got := float64(req.PriceQuarterly); got != 2 {
		t.Fatalf("price_quarterly = %v, want 2", got)
	}
	if got := float64(req.PriceYearly); got != 2.44 {
		t.Fatalf("price_yearly = %v, want 2.44", got)
	}
}

func TestCreatePlanRequestRejectsInvalidStringPrice(t *testing.T) {
	var req CreatePlanRequest
	err := json.Unmarshal([]byte(`{"name":"Basic","duration_days":30,"price_monthly":"invalid"}`), &req)
	if err == nil {
		t.Fatal("expected invalid string price to fail")
	}
}
