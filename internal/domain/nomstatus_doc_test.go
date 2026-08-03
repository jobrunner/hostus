package domain_test

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// nomStatusDocPath is the client-facing reference that renders the rule
// table. Relative to this package's directory.
const nomStatusDocPath = "../../docs/reference/http-api.md"

// docRuleRow matches one rendered table row:
//
//	| `token` | judgement ⚠️ | 1.234 | Bedeutung … |
//
// The judgement cell may carry a trailing open-item marker, and the count is
// written with German thousands separators.
var docRuleRow = regexp.MustCompile(`^\|\s*` + "`" + `([^` + "`" + `]+)` + "`" + `\s*\|\s*(absent|acceptable|disqualifying|unclassified)\s*(⚠️)?\s*\|\s*([0-9.]+)\s*\|`)

// TestNomStatusRulesMatchReferenceDoc pins docs/reference/http-api.md's
// rendered nom_status rule table against domain.NomStatusRules().
//
// The doc states 36 tokens with their judgement and their measured name
// count as a CLIENT-FACING contract — docs/how-to/synonyms-uc5.md points
// callers at it for exactly that. A table maintained by hand next to a Go
// table it is supposed to describe drifts silently, and the failure mode is
// the bad one: a caller reads a token list that no longer decides anything.
// So the two are compared row for row, in order, on the three load-bearing
// fields (token, judgement, count). The German "Bedeutung" column is
// deliberately NOT compared — it is a translation, not a duplicate of the
// Go Note.
func TestNomStatusRulesMatchReferenceDoc(t *testing.T) {
	raw, err := os.ReadFile(nomStatusDocPath)
	if err != nil {
		t.Fatalf("reading %s: %v", nomStatusDocPath, err)
	}

	got := parseDocRuleTable(t, string(raw))
	rules := domain.NomStatusRules()

	if len(got) != len(rules) {
		t.Fatalf("%s renders %d rule rows, domain.NomStatusRules() has %d — every rule needs exactly one documented row",
			nomStatusDocPath, len(got), len(rules))
	}

	for i, rule := range rules {
		want := docRule{
			token:     rule.Fragment,
			judgement: string(rule.Judgement),
			names:     rule.Names,
			openItem:  rule.OpenItem,
		}
		if got[i] != want {
			t.Errorf("row %d: doc has %+v, domain.NomStatusRules()[%d] is %+v", i+1, got[i], i, want)
		}
	}
}

type docRule struct {
	token     string
	judgement string
	names     int
	openItem  bool
}

func (r docRule) String() string {
	return fmt.Sprintf("{token:%q judgement:%s names:%d openItem:%v}", r.token, r.judgement, r.names, r.openItem)
}

// parseDocRuleTable extracts every rendered rule row from the reference doc,
// in document order.
func parseDocRuleTable(t *testing.T, doc string) []docRule {
	t.Helper()
	var out []docRule
	for _, line := range strings.Split(doc, "\n") {
		m := docRuleRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		names, err := strconv.Atoi(strings.ReplaceAll(m[4], ".", ""))
		if err != nil {
			t.Fatalf("row %q: count %q is not a number: %v", line, m[4], err)
		}
		out = append(out, docRule{token: m[1], judgement: m[2], names: names, openItem: m[3] != ""})
	}
	if len(out) == 0 {
		t.Fatalf("no rule rows found in %s — has the table been renamed or removed?", nomStatusDocPath)
	}
	return out
}
