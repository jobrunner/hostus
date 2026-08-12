package httpx_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
)

// TestRoutesMatchOpenAPISpec is the routes<->spec contract test named by
// scripts/doc-drift-check.sh. It closes the SP6 known-gap "316 Zeilen
// handgepflegtes OpenAPI ohne Drift-Prüfung": the spec at
// api/openapi/openapi.yaml is hand-maintained, and nothing stopped it from
// silently drifting away from the routes the router actually serves. This test
// pins them together in BOTH directions — every mounted API route must have an
// OpenAPI path+method and vice versa — so adding a route without documenting it
// (or documenting one without mounting it) fails CI instead of rotting.
//
// It compares the API surface only: the router is built UI-disabled, so "/"
// and "/assets/..." (deliberately undocumented in the API spec) are absent.
func TestRoutesMatchOpenAPISpec(t *testing.T) {
	routes := routerAPISurface(t)
	spec := openAPISurface(t)

	for key := range routes {
		if !spec[key] {
			t.Errorf("route %q is mounted but MISSING from api/openapi/openapi.yaml — document it (or the spec drifted)", key)
		}
	}
	for key := range spec {
		if !routes[key] {
			t.Errorf("OpenAPI path %q has no mounted route — the spec documents an endpoint the router does not serve", key)
		}
	}
}

// routerAPISurface returns the set of "METHOD /path" the router mounts, with a
// non-nil Repo (so the /v1 routes mount) and UI disabled (so the console
// routes, which are not part of the API contract, do not).
func routerAPISurface(t *testing.T) map[string]bool {
	t.Helper()
	r := httpx.NewRouter(httpx.Deps{Repo: stubSecRepo{}, UIEnabled: false})

	got := map[string]bool{}
	err := r.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		tmpl, err := route.GetPathTemplate()
		if err != nil {
			return nil // a route without a path template is not part of the URL contract
		}
		methods, err := route.GetMethods()
		if err != nil {
			// No explicit methods: not one of our endpoints (all use .Methods()).
			return nil
		}
		for _, m := range methods {
			got[strings.ToUpper(m)+" "+tmpl] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking router: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("router walk yielded no routes")
	}
	return got
}

var (
	reTopLevelKey = regexp.MustCompile(`^[A-Za-z]`)
	rePathKey     = regexp.MustCompile(`^  (/\S*):\s*$`)
	reMethodKey   = regexp.MustCompile(`^    (get|post|put|patch|delete|head):\s*$`)
)

// openAPISurface parses api/openapi/openapi.yaml and returns the set of
// "METHOD /path" it documents. Deliberately a small line scanner rather than a
// YAML dependency: the file's shape (2-space path keys under `paths:`,
// 4-space method keys) is regular, and the contract test must not pull a new
// import into the hexagon just to read it.
func openAPISurface(t *testing.T) map[string]bool {
	t.Helper()
	specPath := filepath.Join(repoRoot(t), "api", "openapi", "openapi.yaml")
	f, err := os.Open(filepath.Clean(specPath))
	if err != nil {
		t.Fatalf("opening %s: %v", specPath, err)
	}
	defer func() { _ = f.Close() }()

	want := map[string]bool{}
	inPaths := false
	curPath := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "paths:":
			inPaths = true
			curPath = ""
		case inPaths && reTopLevelKey.MatchString(line):
			// A new top-level key (components:, tags:, ...) ends the paths block.
			inPaths = false
		case inPaths:
			if m := rePathKey.FindStringSubmatch(line); m != nil {
				curPath = m[1]
			} else if m := reMethodKey.FindStringSubmatch(line); m != nil && curPath != "" {
				want[strings.ToUpper(m[1])+" "+curPath] = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanning %s: %v", specPath, err)
	}
	if len(want) == 0 {
		t.Fatalf("no paths parsed from %s — the scanner or the spec shape changed", specPath)
	}
	return want
}

// repoRoot walks up from the test's working directory (the package dir) to the
// module root, identified by go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from the test directory")
		}
		dir = parent
	}
}
