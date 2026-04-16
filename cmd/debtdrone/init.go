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

			// 1. Check if config file already exists
			if _, err := os.Stat(configFilename); err == nil {
				return fmt.Errorf(".debtdrone.yaml already exists in this directory")
			}

			// 2. Create default configuration content
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

			// 3. Write to file
			if err := os.WriteFile(configFilename, []byte(defaultConfig), 0644); err != nil {
				return fmt.Errorf("failed to write .debtdrone.yaml: %w", err)
			}

			fmt.Println("Initialized .debtdrone.yaml successfully.")
			return nil
		},
	}
}
