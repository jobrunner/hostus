package domain_test

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

func TestParseRedistribution_ValidValues(t *testing.T) {
	cases := []struct {
		in   string
		want domain.Redistribution
	}{
		{"allowed", domain.RedistributionAllowed},
		{"restricted", domain.RedistributionRestricted},
		{"unknown", domain.RedistributionUnknown},
		{" Allowed ", domain.RedistributionAllowed},
		{"RESTRICTED", domain.RedistributionRestricted},
		{"Unknown", domain.RedistributionUnknown},
	}
	for _, c := range cases {
		got, err := domain.ParseRedistribution(c.in)
		if err != nil {
			t.Errorf("ParseRedistribution(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseRedistribution(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseRedistribution_InvalidValue(t *testing.T) {
	for _, in := range []string{"", "maybe", "ALLOWED_ISH", "  "} {
		if _, err := domain.ParseRedistribution(in); err == nil {
			t.Errorf("ParseRedistribution(%q): want error, got nil", in)
		}
	}
}
