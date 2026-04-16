package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func createRootWithConfigAndHistory() *cobra.Command {
	root := &cobra.Command{Use: "debtdrone"}
	root.AddCommand(newConfigCmd(), newHistoryCmd())
	return root
}

func TestConfigCmd(t *testing.T) {
	root := createRootWithConfigAndHistory()

	t.Run("config list", func(t *testing.T) {
		output, err := executeCommand(root, "config", "list")
		if err != nil {
			t.Fatalf("config list failed: %v", err)
		}

		headers := []string{"KEY", "VALUE", "TYPE"}
		for _, h := range headers {
			if !strings.Contains(output, h) {
				t.Errorf("config list missing header %q", h)
			}
		}

		if !strings.Contains(output, "Max Complexity") {
			t.Errorf("Expected configuration key not found in output")
		}
	})

	t.Run("config set", func(t *testing.T) {
		output, err := executeCommand(root, "config", "set", "Max Complexity", "20")
		if err != nil {
			t.Fatalf("config set failed: %v", err)
		}

		if !strings.Contains(output, "Successfully set") {
			t.Errorf("Success message not found in output: %s", output)
		}
	})
}

func TestHistoryCmd(t *testing.T) {
	root := createRootWithConfigAndHistory()

	t.Run("history table output", func(t *testing.T) {
		output, err := executeCommand(root, "history")
		if err != nil {
			t.Fatalf("history failed: %v", err)
		}

		headers := []string{"DATE", "REPOSITORY", "ISSUES"}
		for _, h := range headers {
			if !strings.Contains(output, h) {
				t.Errorf("history table missing header %q", h)
			}
		}
	})

	t.Run("history json output", func(t *testing.T) {
		output, err := executeCommand(root, "history", "--format", "json")
		if err != nil {
			t.Fatalf("history json failed: %v", err)
		}

		if !json.Valid([]byte(output)) {
			t.Errorf("history json output invalid: %s", output)
		}
	})
}
