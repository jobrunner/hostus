// Hardening Task 6 measurement: does the M6 licensing-bridge conclusion
// still hold now that Task 5's name normalisation has landed?
//
// M6 (docs/research/reality-check.md) measured the REAL usable bridge gain
// (a recovered name whose accepted_taxon link itself resolves in WCVP) at
// 51 EIVE / 3 Tichý / 2 Midolo taxa — but against the PRE-normalisation
// unresolved set (M2.2: 1.815 / 380 / 229 taxa). Task 5 normalisation
// shrank that set to 303 / 105 / 57 taxa (T5.3: 14.830-14.527,
// 8.907-8.802, 6.382-6.325). Whatever of the original 51/3/2 bridged taxa
// are names normalisation ALSO now resolves is double-counted if M6's
// number is quoted unchanged post-normalisation — this mode re-runs the
// identical M6 bridge/link logic against the SMALLER, post-normalisation
// unresolved set to get the number that is actually still on the table.
package main

import "fmt"

// runNormBridge is the --normbridge entry point: the same bridge/
// accepted-link logic as run(), except EVERY resolution check — both the
// vocabulary's "unresolved" set AND whether a recovered name's
// accepted_taxon link lands on a WCVP concept — goes through the
// NameCandidates ladder (idxCount, shared with --norm and --a1diff)
// instead of run()'s single plain Canonicalize lookup (wcvp). That is a
// deliberate, not incidental, difference from M6's original methodology:
// normalisation is now a real part of how hostus resolves a name, so
// checking the bridge's accepted-taxon target the SAME way the vocabulary
// itself gets checked is the fair comparison — using plain-exact only on
// the target side would UNDERSTATE the bridge's remaining gain by
// rejecting links normalisation would in fact resolve.
func runNormBridge(dbPath string, vocabs, lists pathList) error {
	idxCount, err := loadConceptIndex(dbPath)
	if err != nil {
		return err
	}
	fmt.Printf("index: %d distinct canonical_fold keys\n\n", len(idxCount))

	resolvable := func(canon string) bool {
		for _, cand := range NameCandidates(canon) {
			if idxCount[cand.Key] > 0 {
				return true
			}
		}
		return false
	}

	listSets := make(map[string]map[string]bool, len(lists))
	listAcc := make(map[string]map[string]string, len(lists))
	for _, l := range lists {
		s, err := loadColumn(l.path, "taxon")
		if err != nil {
			return err
		}
		a, err := loadAcceptedLinks(l.path)
		if err != nil {
			return err
		}
		listSets[l.name] = s
		listAcc[l.name] = a
	}

	for _, v := range vocabs {
		taxa, err := loadColumn(v.path, "taxon")
		if err != nil {
			return err
		}
		unresolved := map[string]bool{}
		for t := range taxa {
			if !resolvable(t) {
				unresolved[t] = true
			}
		}
		fmt.Printf("## %s (post-normalisation)\n", v.name)
		fmt.Printf("distinct taxa: %d\n", len(taxa))
		fmt.Printf("still unresolved after normalisation: %d (%.2f%%)\n", len(unresolved), pct(len(unresolved), len(taxa)))

		union := map[string]bool{}
		for _, l := range lists {
			gain := intersect(unresolved, listSets[l.name])
			bridged := 0
			for k := range gain {
				if acc, ok := listAcc[l.name][k]; ok && resolvable(acc) {
					bridged++
					union[k] = true
				}
			}
			fmt.Printf("  + %-10s recovers %6d names, of which bridgeable to a WCVP concept via accepted_taxon: %d\n",
				l.name, len(gain), bridged)
		}
		fmt.Printf("  = real bridgeable gain post-normalisation: %d of %d still-unresolved %s taxa (%.2f%% of all %d)\n\n",
			len(union), len(unresolved), v.name, pct(len(union), len(taxa)), len(taxa))
	}
	return nil
}
