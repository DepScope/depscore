package main

import (
	"testing"
)

func TestServerCommandRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "server" {
		t.Fatalf("expected server command, got %s", cmd.Name())
	}
}
