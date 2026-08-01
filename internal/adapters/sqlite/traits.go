package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jobrunner/hostus/internal/domain"
)

// traitSetKey identifies one (vocab, vocab_version) group within Traits'
// result — the unit domain.TraitSet is built per, never merged across
// vocabularies (PoC P10; see domain.TraitSet's doc comment).
type traitSetKey struct {
	vocab   string
	version string
}

// Traits returns every domain.TraitSet conceptID has trait_value rows for,
// grouped per (vocab, vocab_version) and joined against trait_vocabulary for
// VocabVersion/Taxonomy. vocabs restricts the vocabularies considered; nil
// or empty means every vocabulary. Returns domain.ErrNotFound (wrapped) if
// conceptID does not exist in taxon_concept; an existing concept with no
// trait_value rows returns an empty, non-nil-error slice.
func (db *DB) Traits(ctx context.Context, conceptID string, vocabs []domain.TraitVocab) ([]domain.TraitSet, error) {
	exists, err := db.conceptExists(ctx, conceptID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: checking concept %q exists: %w", conceptID, err)
	}
	if !exists {
		return nil, fmt.Errorf("sqlite: concept %q: %w", conceptID, domain.ErrNotFound)
	}

	query, args := traitsQuery(conceptID, vocabs)
	rows, err := db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying trait values for concept %q: %w", conceptID, err)
	}
	defer func() { _ = rows.Close() }()

	var (
		order []traitSetKey
		sets  = map[traitSetKey]*domain.TraitSet{}
	)
	for rows.Next() {
		var (
			vocab, vocabVersion, dim string
			value                    float64
			nicheWidth               sql.NullFloat64
			nSystems                 sql.NullInt64
			taxonomy                 string
		)
		if err := rows.Scan(&vocab, &vocabVersion, &dim, &value, &nicheWidth, &nSystems, &taxonomy); err != nil {
			return nil, fmt.Errorf("sqlite: scanning trait value for concept %q: %w", conceptID, err)
		}

		key := traitSetKey{vocab: vocab, version: vocabVersion}
		set, ok := sets[key]
		if !ok {
			set = &domain.TraitSet{
				Vocab:        domain.TraitVocab(vocab),
				VocabVersion: vocabVersion,
				Taxonomy:     taxonomy,
			}
			sets[key] = set
			order = append(order, key)
		}

		tv := domain.TraitValue{
			Vocab:        domain.TraitVocab(vocab),
			VocabVersion: vocabVersion,
			Dim:          domain.TraitDim(dim),
			Value:        value,
		}
		if nicheWidth.Valid {
			w := nicheWidth.Float64
			tv.NicheWidth = &w
		}
		if nSystems.Valid {
			n := int(nSystems.Int64)
			tv.NSystems = &n
		}
		set.Values = append(set.Values, tv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating trait values for concept %q: %w", conceptID, err)
	}

	out := make([]domain.TraitSet, 0, len(order))
	for _, key := range order {
		out = append(out, *sets[key])
	}
	return out, nil
}

// traitsQuery builds the query+args Traits runs: every trait_value row for
// conceptID (optionally restricted to vocabs), joined onto trait_vocabulary
// for Taxonomy, ordered by vocab then dim so both the grouping above and the
// caller-visible Values order are deterministic.
func traitsQuery(conceptID string, vocabs []domain.TraitVocab) (string, []any) {
	query := `
		SELECT tv.vocab, tv.vocab_version, tv.dim, tv.value, tv.niche_width, tv.n_systems, COALESCE(vc.taxonomy, '')
		FROM trait_value tv
		LEFT JOIN trait_vocabulary vc ON vc.vocab = tv.vocab AND vc.version = tv.vocab_version
		WHERE tv.concept_id = ?`
	args := []any{conceptID}
	if len(vocabs) > 0 {
		placeholders := placeholdersFor(len(vocabs))
		query += ` AND tv.vocab IN (` + placeholders + `)`
		for _, v := range vocabs {
			args = append(args, string(v))
		}
	}
	query += ` ORDER BY tv.vocab, tv.vocab_version, tv.dim`
	return query, args
}

// conceptExists reports whether id is a known taxon_concept, so Traits can
// distinguish "concept exists but has no trait values" (empty slice, nil
// error) from "concept unknown" (domain.ErrNotFound).
func (db *DB) conceptExists(ctx context.Context, id string) (bool, error) {
	var one int
	err := db.sql.QueryRowContext(ctx, `SELECT 1 FROM taxon_concept WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// TraitVocabularies lists every ingested (vocab, version) metadata row.
func (db *DB) TraitVocabularies(ctx context.Context) ([]domain.TraitVocabMeta, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT vocab, version, taxonomy, COALESCE(license, ''), COALESCE(source_url, ''), redistribution
		FROM trait_vocabulary
		ORDER BY vocab, version`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying trait_vocabulary: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.TraitVocabMeta
	for rows.Next() {
		var vocab, redistribution string
		var meta domain.TraitVocabMeta
		if err := rows.Scan(&vocab, &meta.Version, &meta.Taxonomy, &meta.License, &meta.SourceURL, &redistribution); err != nil {
			return nil, fmt.Errorf("sqlite: scanning trait_vocabulary row: %w", err)
		}
		meta.Vocab = domain.TraitVocab(vocab)
		meta.Redistribution = domain.Redistribution(redistribution)
		out = append(out, meta)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating trait_vocabulary rows: %w", err)
	}
	return out, nil
}
