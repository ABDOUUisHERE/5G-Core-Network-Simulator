package config

import (
	"testing"
	"time"
)

func TestDurationRejectsInvalidTimeouts(t *testing.T) {
	for _, value := range []string{"", "invalid", "-1s", "0s"} {
		t.Setenv("TEST_TIMEOUT", value)
		if got := Duration("TEST_TIMEOUT", 2*time.Second); got != 2*time.Second {
			t.Fatalf("%q: %s", value, got)
		}
	}
	t.Setenv("TEST_TIMEOUT", "250ms")
	if got := Duration("TEST_TIMEOUT", time.Second); got != 250*time.Millisecond {
		t.Fatal(got)
	}
}

func TestLoad(t *testing.T) {
	t.Setenv("SERVICE", "amf")
	t.Setenv("SEED_SUBSCRIBERS", "-4")
	c := Load("udm")
	if c.Service != "amf" || c.SeedSubscribers != 1000 || c.RequestTimeout <= 0 {
		t.Fatalf("invalid config: %+v", c)
	}
}
