package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jobrunner/hostus/internal/app"
	"github.com/jobrunner/hostus/internal/config"
)

// serveFlagBinds maps a dotted viper config key to the pflag name carrying
// its CLI override. Kept as a single table so newServeCmd's flag
// registration and bindServeFlags' viper wiring cannot drift apart.
var serveFlagBinds = map[string]string{
	"logging.level":  "log-level",
	"logging.format": "log-format",
	"server.host":    "host",
	"server.port":    "port",
	"ui.enabled":     "ui",
}

// newServeCmd builds the explicit "hostus serve" alias. Its flags and RunE
// are shared with the root command (see newRootCmd/addServeFlags), so
// `hostus --port 9000` and `hostus serve --port 9000` behave identically.
func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the hostus HTTP server",
		RunE:  runServe,
	}
	addServeFlags(cmd)
	return cmd
}

// addServeFlags registers the flags serve needs on cmd. Defaults mirror
// config.Defaults() so a flag left untouched never disagrees with the
// config-file/env defaults it's bound on top of.
func addServeFlags(cmd *cobra.Command) {
	cmd.Flags().String("log-level", "info", "log level (debug, info, warn, error)")
	cmd.Flags().String("log-format", "json", "log format (json, text)")
	cmd.Flags().String("host", "0.0.0.0", "server host")
	cmd.Flags().Int("port", 8080, "server port")
	// pflag's Bool accepts both bare --ui and explicit --ui=false. The
	// explicit form is the point: a presence-only switch could turn the
	// console on but never off, leaving the flag tier unable to override
	// HOSTUS_UI_ENABLED=true.
	cmd.Flags().Bool("ui", true, "serve the embedded test console at / (--ui=false disables it)")
}

// runServe loads configuration, layers CLI flag overrides on top via
// viper, and runs the composition root until ctx (the signal-context from
// main) is done.
func runServe(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if err := bindServeFlags(cmd, cfg); err != nil {
		return fmt.Errorf("applying flag overrides: %w", err)
	}

	return app.Run(cmd.Context(), cfg, app.WithVersion(Version))
}

// bindServeFlags binds cmd's flags into viper and re-unmarshals cfg so any
// flag the user actually passed (pflag.Changed) overrides the value
// config.Load already resolved from file/env/defaults. It must run AFTER
// config.Load: Load calls viper.Reset(), which would otherwise wipe out
// bindings registered beforehand. Re-validates cfg afterwards so a bad
// flag value (e.g. --port 99999) is rejected the same way a bad config
// file value would be.
func bindServeFlags(cmd *cobra.Command, cfg *config.Config) error {
	for key, flagName := range serveFlagBinds {
		if err := viper.BindPFlag(key, cmd.Flags().Lookup(flagName)); err != nil {
			return err
		}
	}
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("unmarshaling config: %w", err)
	}
	return cfg.Validate()
}
