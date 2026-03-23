package main

import (
	"testing"
)

func TestScanCommandRegistered(t *testing.T) {
	// Verify the scan command is registered on rootCmd.
	cmd, _, err := rootCmd.Find([]string{"scan"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "scan" {
		t.Fatalf("expected scan command, got %s", cmd.Name())
	}
}
