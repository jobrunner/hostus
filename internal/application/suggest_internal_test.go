package application

import "testing"

func TestEffectiveLimit(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		want  int
	}{
		{"zero defaults", 0, defaultSuggestLimit},
		{"negative defaults", -5, defaultSuggestLimit},
		{"within range is unchanged", 5, 5},
		{"exactly the default is unchanged", defaultSuggestLimit, defaultSuggestLimit},
		{"exactly the max is unchanged", maxSuggestLimit, maxSuggestLimit},
		{"one above the max is capped", maxSuggestLimit + 1, maxSuggestLimit},
		{"far above the max is capped", 999, maxSuggestLimit},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := effectiveLimit(c.limit); got != c.want {
				t.Errorf("effectiveLimit(%d) = %d, want %d", c.limit, got, c.want)
			}
		})
	}
}
