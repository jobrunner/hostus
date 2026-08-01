// Command latency answers M4 of the Reality-Check: the p50/p95 of
// GET /v1/suggest against a running hostus server holding the full index.
//
// It drives the REAL HTTP endpoint (not an in-process benchmark) so the
// numbers include routing, middleware, JSON encoding and the loopback
// socket — i.e. what a frontend would actually see, minus the WAN.
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// prefixes is the realistic autosuggest prefix set: 2-5 characters, biased
// towards genera that are common in Central-European vegetation, plus a few
// deliberately broad 2-char stems (worst case for FTS5 prefix matching).
var prefixes = []string{
	"ac", "ag", "al", "be", "ca", "ce", "fe", "ga", "po", "qu", "ra", "sa", "th", "tr", "ve",
	"ace", "ach", "arte", "betu", "cala", "care", "cent", "fest", "gali", "hier",
	"potent", "querc", "ranu", "salix", "thymu", "trifo", "veron", "viola",
	"abies", "acer", "picea", "pinus", "rubus",
}

func main() {
	base := flag.String("base", "http://127.0.0.1:8099", "base URL of the running hostus server")
	area := flag.String("area", "", "area query parameter (empty = no area filter)")
	reps := flag.Int("reps", 20, "measured repetitions per prefix")
	warmup := flag.Int("warmup", 3, "unmeasured warmup requests per prefix")
	limit := flag.Int("limit", 10, "limit query parameter")
	pace := flag.Duration("pace", 100*time.Millisecond, "minimum delay between requests; the server rate-limits at 20 req/s (internal/adapters/http/router.go defaultRateLimitPerSecond), so an unpaced probe gets 429s. The pause is NOT part of the measured duration.")
	flag.Parse()

	if err := run(*base, *area, *reps, *warmup, *limit, *pace); err != nil {
		fmt.Fprintln(os.Stderr, "latency:", err)
		os.Exit(1)
	}
}

func run(base, area string, reps, warmup, limit int, pace time.Duration) error {
	client := &http.Client{Timeout: 30 * time.Second}

	var all []time.Duration
	perPrefix := make(map[string][]time.Duration, len(prefixes))

	for _, p := range prefixes {
		u := fmt.Sprintf("%s/v1/suggest?q=%s&limit=%d", base, url.QueryEscape(p), limit)
		if area != "" {
			u += "&area=" + url.QueryEscape(area)
		}
		for i := 0; i < warmup; i++ {
			if _, err := once(client, u); err != nil {
				return err
			}
			time.Sleep(pace)
		}
		for i := 0; i < reps; i++ {
			d, err := once(client, u)
			if err != nil {
				return err
			}
			time.Sleep(pace)
			all = append(all, d)
			perPrefix[p] = append(perPrefix[p], d)
		}
	}

	fmt.Printf("area=%q prefixes=%d reps/prefix=%d warmup/prefix=%d pace=%s samples=%d\n",
		area, len(prefixes), reps, warmup, pace, len(all))
	fmt.Printf("overall: p50=%s p90=%s p95=%s p99=%s min=%s max=%s\n",
		q(all, 0.50), q(all, 0.90), q(all, 0.95), q(all, 0.99), q(all, 0), q(all, 1))
	fmt.Println()
	fmt.Println("| Prefix | p50 | p95 | max |")
	fmt.Println("|---|---|---|---|")
	keys := make([]string, 0, len(perPrefix))
	for k := range perPrefix {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) < len(keys[j])
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		fmt.Printf("| `%s` | %s | %s | %s |\n", k, q(perPrefix[k], 0.50), q(perPrefix[k], 0.95), q(perPrefix[k], 1))
	}
	return nil
}

func once(client *http.Client, u string) (time.Duration, error) {
	start := time.Now()
	resp, err := client.Get(u) //nolint:noctx // measurement probe
	if err != nil {
		return 0, err
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	d := time.Since(start)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%s: HTTP %d: %s", u, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return d, nil
}

// q returns the p-quantile (nearest-rank) of ds. p=0 gives the minimum,
// p=1 the maximum. It rounds to microseconds so the report stays readable.
func q(ds []time.Duration, p float64) time.Duration {
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	i := int(p * float64(len(s)-1))
	return s[i].Round(time.Microsecond)
}
