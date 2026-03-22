package main

import (
	"testing"
)

func TestPackageCommandRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"package"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "package" {
		t.Fatalf("expected package command, got %s", cmd.Name())
	}
}

func TestPackageCheckCommandRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"package", "check"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "check" {
		t.Fatalf("expected check command, got %s", cmd.Name())
	}
}

func TestParseNameVersion(t *testing.T) {
	tests := []struct {
		spec    string
		name    string
		version string
		wantErr bool
	}{
		{"requests@2.31.0", "requests", "2.31.0", false},
		{"github.com/some/pkg@v1.2.3", "github.com/some/pkg", "v1.2.3", false},
		{"novat", "", "", true},
		{"@version", "", "", true},
	}
	for _, tt := range tests {
		name, version, err := parseNameVersion(tt.spec)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseNameVersion(%q): expected error, got nil", tt.spec)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseNameVersion(%q): unexpected error: %v", tt.spec, err)
			continue
		}
		if name != tt.name || version != tt.version {
			t.Errorf("parseNameVersion(%q) = (%q, %q), want (%q, %q)",
				tt.spec, name, version, tt.name, tt.version)
		}
	}
}
