package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a .debtdrone.yaml configuration file in the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			const configFilename = ".debtdrone.yaml"

			if _, err := os.Stat(configFilename); err == nil {
				return fmt.Errorf(".debtdrone.yaml already exists in this directory")
			}

			defaultConfig := `quality_gate:
  fail_on: high

thresholds:
  max_complexity: 15
  security_scan: true

ignore_paths:
  - "node_modules"
  - "vendor"
  - "dist"
  - ".git"
`

			if err := os.WriteFile(configFilename, []byte(defaultConfig), 0644); err != nil {
				return fmt.Errorf("failed to write .debtdrone.yaml: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Initialized .debtdrone.yaml successfully.")
			return nil
		},
	}
}
