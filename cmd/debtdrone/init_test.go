package main

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func createRootWithInit() *cobra.Command {
	root := &cobra.Command{Use: "debtdrone"}
	root.AddCommand(newInitCmd())
	return root
}

func TestInitCmd(t *testing.T) {
	// 1. Create a fresh temp directory and switch working dir
	tmpDir := t.TempDir()
	originalWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(originalWd)

	const configName = ".debtdrone.yaml"

	t.Run("Initialize new config", func(t *testing.T) {
		root := createRootWithInit()
		output, err := executeCommand(root, "init")
		
		if err != nil {
			t.Fatalf("Expected no error, got %v. Output: %s", err, output)
		}

		if !strings.Contains(output, "Initialized .debtdrone.yaml successfully") {
			t.Errorf("Unexpected output: %s", output)
		}

		// Verify file existence and content
		if _, err := os.Stat(configName); os.IsNotExist(err) {
			t.Fatal("Config file was not created")
		}

		data, _ := os.ReadFile(configName)
		if !strings.Contains(string(data), "quality_gate") {
			t.Errorf("Config file content looks wrong:\n%s", string(data))
		}
	})

	t.Run("Fail when config already exists", func(t *testing.T) {
		root := createRootWithInit()
		output, err := executeCommand(root, "init")
		
		if err == nil {
			t.Fatal("Expected error when running init twice, but got nil")
		}

		expectedErr := ".debtdrone.yaml already exists"
		if !strings.Contains(err.Error(), expectedErr) {
			t.Errorf("Expected error to contain %q, but got %q. Output: %s", expectedErr, err.Error(), output)
		}
	})
}
