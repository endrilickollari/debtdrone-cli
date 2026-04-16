package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type configItem struct {
	Key         string
	Value       string
	Type        string
	Description string
}

// mockConfigItems replicates the in-memory settings used in the TUI
func mockConfigItems() []configItem {
	return []configItem{
		{"Output Format", "text", "string", "Render mode for scan results (text/json)"},
		{"Auto-Update Checks", "true", "bool", "Check for newer releases on startup"},
		{"Fail on Severity", "high", "string", "Min severity for non-zero exit code"},
		{"Max Complexity", "15", "int", "Cyclomatic-complexity threshold per function"},
		{"Security Scan", "true", "bool", "Run Trivy vulnerability detection"},
		{"Show Line Numbers", "true", "bool", "Include line:col in the results list"},
		{"Max Results", "500", "int", "Cap on issues rendered per scan"},
	}
}

func newConfigCmd() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage DebtDrone configuration",
	}

	configCmd.AddCommand(newConfigListCmd())
	configCmd.AddCommand(newConfigSetCmd())

	return configCmd
}

func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configuration keys and their values",
		RunE: func(cmd *cobra.Command, args []string) error {
			items := mockConfigItems()

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "KEY\tVALUE\tTYPE\tDESCRIPTION")
			fmt.Fprintln(w, "---\t-----\t----\t-----------")

			for _, item := range items {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", item.Key, item.Value, item.Type, item.Description)
			}

			return w.Flush()
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set [key] [value]",
		Short: "Update a configuration key with a new value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			
			// Mock the update logic — in a real implementation this would write to 
			// a global ~/.debtdrone/config.json file.
			fmt.Printf("✅ Successfully set %q to %q\n", key, value)
			return nil
		},
	}
}
