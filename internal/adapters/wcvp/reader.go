// Package wcvp reads the World Checklist of Vascular Plants (WCVP) bulk
// archive: a Darwin Core Archive (DwC-A), not ColDP, despite earlier spec
// wording — see poc/P02-findings.md. The three data files are pipe (|)
// delimited with no quoting, and wcvp_taxon.csv ships with two typo'd
// headers (scientfiicname, scientfiicnameauthorship) baked into the vendor
// data itself.
package wcvp

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// TaxonRow is one row of wcvp_taxon.csv (the DwC-A core, DwC Taxon rowType).
// Fields map 1:1 to columns; TaxonRank/TaxonomicStatus are kept verbatim
// (mixed-case, WCVP's own value set) rather than normalized here — that
// belongs to a later mapping stage, not the raw reader.
type TaxonRow struct {
	TaxonID              string
	Family               string
	Genus                string
	SpecificEpithet      string
	InfraspecificEpithet string
	Canonical            string // from the typo'd "scientfiicname" column
	Authorship           string // from the typo'd "scientfiicnameauthorship" column
	Rank                 string
	Status               string
	AcceptedNameUsageID  string // empty => this row is accepted; else points at the accepted taxonid
	ParentNameUsageID    string
	OriginalNameUsageID  string // basionym link
	PublishedIn          string
	NomenclaturalStatus  string
	TaxonRemarks         string
	ScientificNameID     string // unreliable IPNI-id source; prefer POWOID()
	DynamicProperties    string // raw JSON, see POWOID()
	References           string
}

// POWOID parses the DynamicProperties JSON blob and returns its "powoid"
// field, WCVP's reliable IPNI-id source (scientificnameid is empty in
// ~30-40% of real rows). Returns "" if DynamicProperties is empty or not
// valid JSON; Read already validates this JSON eagerly and skips rows that
// fail to parse, so a TaxonRow obtained from Read never hits that path.
func (t TaxonRow) POWOID() string {
	if t.DynamicProperties == "" {
		return ""
	}
	var props struct {
		POWOID string `json:"powoid"`
	}
	if err := json.Unmarshal([]byte(t.DynamicProperties), &props); err != nil {
		return ""
	}
	return props.POWOID
}

// DistributionRow is one row of wcvp_distribution.csv (DwC-A extension,
// GBIF Distribution rowType), joined to TaxonRow via CoreID == TaxonID.
type DistributionRow struct {
	CoreID             string
	Locality           string
	EstablishmentMeans string // "introduced" when non-native; empty (not "native") otherwise
	LocationID         string // "TDWG:<L3 code>"
	OccurrenceStatus   string
	ThreatStatus       string
}

// AreaCode strips the "TDWG:" prefix from LocationID, returning the bare
// WGSRPD level-3 area code.
func (d DistributionRow) AreaCode() string {
	return strings.TrimPrefix(d.LocationID, "TDWG:")
}

// ReplacementRow is one row of wcvp_replacementNames.csv (DwC-A extension;
// its rowType is a ColDP NameRelation term despite the DwC-A container).
type ReplacementRow struct {
	TaxonID            string
	RelatedNameUsageID string
	RelationType       string
	Remarks            string
}

// Dataset is the parsed WCVP archive contents. Errors collects non-fatal,
// per-row problems (malformed field count, invalid dynamicproperties JSON):
// such rows are skipped rather than causing Read to fail outright, since a
// bulk taxonomic archive is expected to have a small number of dirty rows.
type Dataset struct {
	Taxa          []TaxonRow
	Distributions []DistributionRow
	Replacements  []ReplacementRow
	Errors        []error
}

var taxonColumns = []string{
	"taxonid", "family", "genus", "specificepithet", "infraspecificepithet",
	"scientfiicname", "scientfiicnameauthorship", "taxonrank", "taxonomicstatus",
	"acceptednameusageid", "parentnameusageid", "originalnameusageid",
	"namepublishedin", "nomenclaturalstatus", "taxonremarks",
	"scientificnameid", "dynamicproperties", "references",
}

var distributionColumns = []string{
	"coreid", "locality", "establishmentmeans", "locationid", "occurrencestatus", "threatstatus",
}

var replacementColumns = []string{
	"taxonid", "relatednameusageid", "relationtype", "remarks",
}

// Read parses the three WCVP DwC-A data files (wcvp_taxon.csv,
// wcvp_distribution.csv, wcvp_replacementNames.csv) from dir.
func Read(dir string) (*Dataset, error) {
	var errs []error

	taxa, err := readTaxa(dir, &errs)
	if err != nil {
		return nil, err
	}

	dist, err := readDistributions(dir, &errs)
	if err != nil {
		return nil, err
	}

	repl, err := readReplacements(dir, &errs)
	if err != nil {
		return nil, err
	}

	return &Dataset{Taxa: taxa, Distributions: dist, Replacements: repl, Errors: errs}, nil
}

func readTaxa(dir string, errs *[]error) ([]TaxonRow, error) {
	var taxa []TaxonRow
	err := readCSV(filepath.Join(dir, "wcvp_taxon.csv"), taxonColumns, func(idx map[string]int, row []string) error {
		dp := row[idx["dynamicproperties"]]
		if dp != "" {
			var probe map[string]any
			if err := json.Unmarshal([]byte(dp), &probe); err != nil {
				return fmt.Errorf("invalid dynamicproperties JSON: %w", err)
			}
		}
		taxa = append(taxa, TaxonRow{
			TaxonID:              row[idx["taxonid"]],
			Family:               row[idx["family"]],
			Genus:                row[idx["genus"]],
			SpecificEpithet:      row[idx["specificepithet"]],
			InfraspecificEpithet: row[idx["infraspecificepithet"]],
			Canonical:            row[idx["scientfiicname"]],
			Authorship:           row[idx["scientfiicnameauthorship"]],
			Rank:                 row[idx["taxonrank"]],
			Status:               row[idx["taxonomicstatus"]],
			AcceptedNameUsageID:  row[idx["acceptednameusageid"]],
			ParentNameUsageID:    row[idx["parentnameusageid"]],
			OriginalNameUsageID:  row[idx["originalnameusageid"]],
			PublishedIn:          row[idx["namepublishedin"]],
			NomenclaturalStatus:  row[idx["nomenclaturalstatus"]],
			TaxonRemarks:         row[idx["taxonremarks"]],
			ScientificNameID:     row[idx["scientificnameid"]],
			DynamicProperties:    dp,
			References:           row[idx["references"]],
		})
		return nil
	}, errs)
	return taxa, err
}

func readDistributions(dir string, errs *[]error) ([]DistributionRow, error) {
	var dist []DistributionRow
	err := readCSV(filepath.Join(dir, "wcvp_distribution.csv"), distributionColumns, func(idx map[string]int, row []string) error {
		dist = append(dist, DistributionRow{
			CoreID:             row[idx["coreid"]],
			Locality:           row[idx["locality"]],
			EstablishmentMeans: row[idx["establishmentmeans"]],
			LocationID:         row[idx["locationid"]],
			OccurrenceStatus:   row[idx["occurrencestatus"]],
			ThreatStatus:       row[idx["threatstatus"]],
		})
		return nil
	}, errs)
	return dist, err
}

func readReplacements(dir string, errs *[]error) ([]ReplacementRow, error) {
	var repl []ReplacementRow
	err := readCSV(filepath.Join(dir, "wcvp_replacementNames.csv"), replacementColumns, func(idx map[string]int, row []string) error {
		repl = append(repl, ReplacementRow{
			TaxonID:            row[idx["taxonid"]],
			RelatedNameUsageID: row[idx["relatednameusageid"]],
			RelationType:       row[idx["relationtype"]],
			Remarks:            row[idx["remarks"]],
		})
		return nil
	}, errs)
	return repl, err
}

// readCSV opens a pipe-delimited, unquoted WCVP data file, builds a
// name->index map from its header (tolerating WCVP's verbatim typos since
// the caller passes the exact expected names), and invokes fn once per data
// row. Rows with fewer fields than the header, or that fn rejects via a
// returned error, are skipped and recorded in errs rather than aborting the
// whole read.
func readCSV(path string, wantHeader []string, fn func(idx map[string]int, row []string) error, errs *[]error) error {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("wcvp: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	r.Comma = '|'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		return fmt.Errorf("wcvp: read header of %s: %w", path, err)
	}
	idx := make(map[string]int, len(header))
	for i, name := range header {
		idx[name] = i
	}
	for _, want := range wantHeader {
		if _, ok := idx[want]; !ok {
			return fmt.Errorf("wcvp: %s: missing expected column %q in header %v", path, want, header)
		}
	}

	minFields := len(wantHeader)
	line := 1 // header was line 1
	for {
		line++
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			*errs = append(*errs, fmt.Errorf("wcvp: %s:%d: %w", path, line, err))
			continue
		}
		if len(row) < minFields {
			*errs = append(*errs, fmt.Errorf("wcvp: %s:%d: short row: got %d fields, want at least %d", path, line, len(row), minFields))
			continue
		}
		if err := fn(idx, row); err != nil {
			*errs = append(*errs, fmt.Errorf("wcvp: %s:%d: %w", path, line, err))
			continue
		}
	}
	return nil
}
