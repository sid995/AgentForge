package config

import "testing"

func TestSchedulerStrategyIsBounded(t *testing.T) {
	strategy, err := SchedulerStrategy(func(string) (string, bool) { return "", false })
	if err != nil || strategy != "least-loaded" {
		t.Fatalf("default strategy=%q err=%v", strategy, err)
	}
	strategy, err = SchedulerStrategy(func(string) (string, bool) { return "region-affinity", true })
	if err != nil || strategy != "region-affinity" {
		t.Fatalf("configured strategy=%q err=%v", strategy, err)
	}
	if _, err := SchedulerStrategy(func(string) (string, bool) { return "random", true }); err == nil {
		t.Fatal("unsupported strategy accepted")
	}
}
