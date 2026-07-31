package wcvp_test

import (
	"os"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/wcvp"
)

const fixtureDir = "testdata/wcvp-sample"

func loadFixture(t *testing.T) *wcvp.Dataset {
	t.Helper()
	ds, err := wcvp.Read(fixtureDir)
	if err != nil {
		t.Fatalf("Read(%q): unexpected error: %v", fixtureDir, err)
	}
	return ds
}

func findTaxon(t *testing.T, ds *wcvp.Dataset, taxonID string) *wcvp.TaxonRow {
	t.Helper()
	for i := range ds.Taxa {
		if ds.Taxa[i].TaxonID == taxonID {
			return &ds.Taxa[i]
		}
	}
	t.Fatalf("taxonid %q not found in Taxa", taxonID)
	return nil
}

func TestRead_RowCounts(t *testing.T) {
	ds := loadFixture(t)

	if got, want := len(ds.Taxa), 20; got != want {
		t.Errorf("len(Taxa) = %d, want %d", got, want)
	}
	if got, want := len(ds.Distributions), 27; got != want {
		t.Errorf("len(Distributions) = %d, want %d", got, want)
	}
	if got, want := len(ds.Replacements), 2; got != want {
		t.Errorf("len(Replacements) = %d, want %d", got, want)
	}
	if len(ds.Errors) != 0 {
		t.Errorf("Errors = %v, want none for a clean fixture", ds.Errors)
	}
}

func TestRead_AcceptedTaxon(t *testing.T) {
	ds := loadFixture(t)
	corynephorus := findTaxon(t, ds, "405825")

	if got, want := corynephorus.Canonical, "Corynephorus canescens"; got != want {
		t.Errorf("Canonical = %q, want %q", got, want)
	}
	if got, want := corynephorus.Authorship, "(L.) P.Beauv."; got != want {
		t.Errorf("Authorship = %q, want %q", got, want)
	}
	if got, want := corynephorus.Rank, "Species"; got != want {
		t.Errorf("Rank = %q, want %q", got, want)
	}
	if got, want := corynephorus.Status, "Accepted"; got != want {
		t.Errorf("Status = %q, want %q", got, want)
	}
	if got, want := corynephorus.POWOID(), "396681-1"; got != want {
		t.Errorf("POWOID() = %q, want %q", got, want)
	}
}

func TestRead_SynonymTaxon(t *testing.T) {
	ds := loadFixture(t)
	synonym := findTaxon(t, ds, "543929")

	if got, want := synonym.AcceptedNameUsageID, "405825"; got != want {
		t.Errorf("AcceptedNameUsageID = %q, want %q", got, want)
	}
}

func TestRead_DistributionAreaCode(t *testing.T) {
	ds := loadFixture(t)

	found := false
	for _, d := range ds.Distributions {
		if d.CoreID != "405825" {
			continue
		}
		found = true
		if !strings.HasPrefix(d.LocationID, "TDWG:") {
			t.Errorf("LocationID = %q, want TDWG: prefix", d.LocationID)
			continue
		}
		if got, want := d.AreaCode(), strings.TrimPrefix(d.LocationID, "TDWG:"); got != want {
			t.Errorf("AreaCode() = %q, want %q", got, want)
		}
	}
	if !found {
		t.Fatal("no distribution rows found for coreid 405825")
	}
}

func TestRead_MissingDirectory(t *testing.T) {
	if _, err := wcvp.Read("testdata/does-not-exist"); err == nil {
		t.Fatal("Read(missing dir): expected error, got nil")
	}
}

const emptyDistributionCSV = "coreid|locality|establishmentmeans|locationid|occurrencestatus|threatstatus\n"

const emptyReplacementCSV = "taxonid|relatednameusageid|relationtype|remarks\n"

const oneValidTaxonRowCSV = "taxonid|family|genus|specificepithet|infraspecificepithet|scientfiicname|scientfiicnameauthorship|taxonrank|taxonomicstatus|acceptednameusageid|parentnameusageid|originalnameusageid|namepublishedin|nomenclaturalstatus|taxonremarks|scientificnameid|dynamicproperties|references\n" +
	`1|Poaceae|Corynephorus|canescens||Corynephorus canescens|(L.) P.Beauv.|Species|Accepted|1|||||||{"powoid":"396681-1"}|https://example.org/1` + "\n"

func TestRead_MissingDistributionFile(t *testing.T) {
	dir := t.TempDir()
	writeFixtureFile(t, dir, "wcvp_taxon.csv", oneValidTaxonRowCSV)
	writeFixtureFile(t, dir, "wcvp_replacementNames.csv", emptyReplacementCSV)
	// wcvp_distribution.csv intentionally absent.

	if _, err := wcvp.Read(dir); err == nil {
		t.Fatal("Read: expected error for missing wcvp_distribution.csv, got nil")
	}
}

func TestRead_MissingReplacementFile(t *testing.T) {
	dir := t.TempDir()
	writeFixtureFile(t, dir, "wcvp_taxon.csv", oneValidTaxonRowCSV)
	writeFixtureFile(t, dir, "wcvp_distribution.csv", emptyDistributionCSV)
	// wcvp_replacementNames.csv intentionally absent.

	if _, err := wcvp.Read(dir); err == nil {
		t.Fatal("Read: expected error for missing wcvp_replacementNames.csv, got nil")
	}
}

func TestRead_MalformedRowsAreSkippedNotPanicking(t *testing.T) {
	dir := t.TempDir()
	writeFixtureFile(t, dir, "wcvp_taxon.csv", strings.Join([]string{
		"taxonid|family|genus|specificepithet|infraspecificepithet|scientfiicname|scientfiicnameauthorship|taxonrank|taxonomicstatus|acceptednameusageid|parentnameusageid|originalnameusageid|namepublishedin|nomenclaturalstatus|taxonremarks|scientificnameid|dynamicproperties|references",
		// valid row (line 2)
		`1|Poaceae|Corynephorus|canescens||Corynephorus canescens|(L.) P.Beauv.|Species|Accepted|1|||||||{"powoid":"396681-1"}|https://example.org/1`,
		// short row: missing trailing fields (line 3)
		`2|Poaceae|Corynephorus|canescens`,
		// invalid JSON in dynamicproperties (line 4)
		`3|Poaceae|Corynephorus|canescens||Corynephorus canescens|(L.) P.Beauv.|Species|Accepted|3|||||||{not-json}|https://example.org/3`,
	}, "\n")+"\n")
	writeFixtureFile(t, dir, "wcvp_distribution.csv", emptyDistributionCSV)
	writeFixtureFile(t, dir, "wcvp_replacementNames.csv", emptyReplacementCSV)

	ds, err := wcvp.Read(dir)
	if err != nil {
		t.Fatalf("Read(%q): unexpected fatal error: %v", dir, err)
	}
	if got, want := len(ds.Taxa), 1; got != want {
		t.Fatalf("len(Taxa) = %d, want %d (only the clean row should survive)", got, want)
	}
	if got, want := len(ds.Errors), 2; got != want {
		t.Fatalf("len(Errors) = %d, want %d (short row + bad JSON row)", got, want)
	}
	if !strings.Contains(ds.Errors[0].Error(), ":3:") {
		t.Errorf("Errors[0] = %q, want it to reference source line 3 (the short row)", ds.Errors[0])
	}
	if !strings.Contains(ds.Errors[1].Error(), ":4:") {
		t.Errorf("Errors[1] = %q, want it to reference source line 4 (the bad-JSON row)", ds.Errors[1])
	}
}

func writeFixtureFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(dir+"/"+name, []byte(content), 0o600); err != nil {
		t.Fatalf("writeFixtureFile(%s): %v", name, err)
	}
}
