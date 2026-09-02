package main

import (
	"os"
	"time"
)

func main() {
	marker := os.Getenv("TEST_DEBTDRONE_FAKE_TRIVY_MARKER")
	if marker == "" {
		os.Exit(2)
	}
	if err := os.WriteFile(marker, []byte("started"), 0o600); err != nil {
		os.Exit(2)
	}
	// CommandContext normally terminates this process. The deadline prevents an
	// orphaned test helper from surviving indefinitely if cancellation regresses.
	time.Sleep(30 * time.Second)
	os.Exit(3)
}
