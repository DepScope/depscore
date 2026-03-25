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

func TestScanOnlyFlagRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"scan"})
	if err != nil {
		t.Fatal(err)
	}
	flag := cmd.Flags().Lookup("only")
	if flag == nil {
		t.Fatal("expected --only flag to be registered on scan command")
	}
	if flag.Value.Type() != "stringSlice" {
		t.Fatalf("expected --only flag type stringSlice, got %s", flag.Value.Type())
	}
}
