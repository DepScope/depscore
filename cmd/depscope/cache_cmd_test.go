package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestCachePruneCommandRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"cache", "prune"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "prune" {
		t.Fatalf("expected prune command, got %s", cmd.Name())
	}
}

func TestCachePrune_ParseDuration(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"90d", 90 * 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"0d", 0, false},
		{"2160h", 2160 * time.Hour, false},       // 90 days in hours
		{"24h", 24 * time.Hour, false},
		{"", 0, true},
		{"abc", 0, true},
		{"-1d", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDayDuration(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}

	// Verify 90d == 2160h
	d90, err := parseDayDuration("90d")
	require.NoError(t, err)
	assert.Equal(t, 2160*time.Hour, d90)
}
