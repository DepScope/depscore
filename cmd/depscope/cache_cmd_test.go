package main

import (
	"testing"
)

func TestCacheCommandRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"cache"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "cache" {
		t.Fatalf("expected cache command, got %s", cmd.Name())
	}
}

func TestCacheStatusCommandRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"cache", "status"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "status" {
		t.Fatalf("expected status command, got %s", cmd.Name())
	}
}

func TestCacheClearCommandRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"cache", "clear"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "clear" {
		t.Fatalf("expected clear command, got %s", cmd.Name())
	}
}
