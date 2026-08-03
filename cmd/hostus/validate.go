package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jobrunner/hostus/internal/adapters/manifest"
)

// validateCmdName is shared with tests so the "validate" literal only needs
// to be spelled once outside of _test.go files.
const validateCmdName = "validate"

// newValidateCmd builds "hostus validate --dataset dataset.yaml": it
// parses and schema-validates the manifest and reports the result, without
// ever opening (let alone writing to) a database.
func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   validateCmdName,
		Short: "Validate a dataset.yaml manifest against the embedded schema",
		RunE:  runValidate,
	}
	cmd.Flags().String("dataset", "", "path to the dataset.yaml manifest to validate")
	return cmd
}

// runValidate parses+validates the manifest at --dataset and prints a short
// summary on success. manifest.Parse itself already returns a descriptive
// error (mentioning "schema validation" or the strict-decode failure) on an
// invalid manifest; runValidate returns that error as-is so main's
// "hostus: %v" wrapper puts it on stderr with a non-zero exit, exactly the
// contract "validate" promises — no separate stderr write needed here.
func runValidate(cmd *cobra.Command, _ []string) error {
	path, err := cmd.Flags().GetString("dataset")
	if err != nil {
		return err
	}
	if path == "" {
		return errors.New("validate: --dataset is required")
	}

	ds, err := manifest.Parse(path)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "OK: %s is valid (%d backbones, %d trait vocabularies, manifest_sha=%s)\n",
		path, len(ds.Backbones), len(ds.TraitVocabularies), ds.ManifestSHA)
	return err
}
