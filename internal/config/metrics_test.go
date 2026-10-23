package config

import (
	"strings"
	"testing"
)

func TestMetricsCredentialValidation(t *testing.T) {
	for _, test := range []struct {
		value string
		valid bool
	}{
		{"", true}, {strings.Repeat("s", 32), true}, {strings.Repeat("s", 128), true}, {strings.Repeat("s", 31), false}, {strings.Repeat("s", 129), false}, {strings.Repeat("s", 32) + "\n", false}, {strings.Repeat("s", 32) + " ", false}, {strings.Repeat("界", 32), false},
	} {
		cfg, err := Load(func(key string) string {
			if key == "SUBLANE_METRICS_TOKEN" {
				return test.value
			}
			return ""
		})
		if (err == nil) != test.valid {
			t.Fatalf("validation result: %v", err)
		}
		if err == nil && cfg.MetricsToken != test.value {
			t.Fatal("credential changed")
		}
	}
}
