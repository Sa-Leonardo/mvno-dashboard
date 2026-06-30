package domain

import (
	"testing"
	"time"
)

func TestComputeNextRecharge(t *testing.T) {
	lastRecharge := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := ComputeNextRecharge(lastRecharge, 90, 0)
	want := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	if !got.Equal(want) {
		t.Fatalf("expected %s, got %s", want.Format("2006-01-02"), got.Format("2006-01-02"))
	}
}

func TestComputeNextRechargeWithSafetyWindow(t *testing.T) {
	lastRecharge := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := ComputeNextRecharge(lastRecharge, 90, 10)
	want := time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)

	if !got.Equal(want) {
		t.Fatalf("expected %s, got %s", want.Format("2006-01-02"), got.Format("2006-01-02"))
	}
}
