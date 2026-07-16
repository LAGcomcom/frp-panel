package handler

import (
	"encoding/json"
	"testing"

	"github.com/frp-panel/frp-panel/internal/model"
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

func TestPlanDurationDaysUsesConfiguredBaseDuration(t *testing.T) {
	plan := &model.Plan{DurationDays: 15}
	for durationType, want := range map[string]int{"monthly": 15, "quarterly": 45, "yearly": 180} {
		if got := planDurationDays(plan, durationType); got != want {
			t.Errorf("planDurationDays(%s) = %d, want %d", durationType, got, want)
		}
	}
	if got := planDurationDays(&model.Plan{}, "monthly"); got != 30 {
		t.Fatalf("legacy zero duration fallback = %d, want 30", got)
	}
}

func TestCreatePlanRequestRejectsInvalidStringPrice(t *testing.T) {
	var req CreatePlanRequest
	err := json.Unmarshal([]byte(`{"name":"Basic","duration_days":30,"price_monthly":"invalid"}`), &req)
	if err == nil {
		t.Fatal("expected invalid string price to fail")
	}
}
