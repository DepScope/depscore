package scanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSemver(t *testing.T) {
	tests := []struct {
		input                string
		major, minor, patch int
		ok                  bool
	}{
		{"1.14.1", 1, 14, 1, true},
		{"0.30.4", 0, 30, 4, true},
		{"4.18.2", 4, 18, 2, true},
		{"1.0.0-beta.1", 1, 0, 0, true},
		{"v1.14.1", 1, 14, 1, true},  // v prefix
		{"not-a-version", 0, 0, 0, false},
		{"", 0, 0, 0, false},
	}
	for _, tt := range tests {
		v, err := parseSemver(tt.input)
		if !tt.ok {
			assert.Error(t, err, tt.input)
			continue
		}
		assert.NoError(t, err, tt.input)
		assert.Equal(t, tt.major, v.major, tt.input)
		assert.Equal(t, tt.minor, v.minor, tt.input)
		assert.Equal(t, tt.patch, v.patch, tt.input)
	}
}

func TestSemverSatisfies(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		want       bool
	}{
		{"1.14.1", "1.14.1", true},
		{"1.14.1", "1.14.2", false},
		{"^1.14.0", "1.14.1", true},
		{"^1.14.0", "1.99.99", true},
		{"^1.14.0", "2.0.0", false},
		{"^1.14.0", "1.13.0", false},
		{"^0.30.0", "0.30.4", true},
		{"^0.30.0", "0.31.0", false},
		{"^0.30.0", "0.29.9", false},
		{"~1.14.0", "1.14.1", true},
		{"~1.14.0", "1.15.0", false},
		{"~0.30.0", "0.30.4", true},
		{"~0.30.0", "0.31.0", false},
		{">=1.14.0", "1.14.1", true},
		{">=1.14.0", "1.13.0", false},
		{">=1.14.0", "2.0.0", true},
		{">1.14.0", "1.14.1", true},
		{">1.14.0", "1.14.0", false},
		{"<=1.14.1", "1.14.1", true},
		{"<=1.14.1", "1.14.2", false},
		{"<2.0.0", "1.99.99", true},
		{"<2.0.0", "2.0.0", false},
		{">=0.30.0,<0.31.0", "0.30.4", true},
		{">=0.30.0,<0.31.0", "0.31.0", false},
		{"*", "1.14.1", true},
		{"latest", "1.14.1", true},
		// Caret ^0.0.x — locks to exact patch
		{"^0.0.3", "0.0.3", true},
		{"^0.0.3", "0.0.4", false},
		{"^0.0.3", "0.1.0", false},
		// = prefix
		{"=1.14.1", "1.14.1", true},
		{"=1.14.1", "1.14.2", false},
	}
	for _, tt := range tests {
		got := semverSatisfies(tt.constraint, tt.version)
		assert.Equal(t, tt.want, got, "%s satisfies %s", tt.version, tt.constraint)
	}
}

func TestSemverRangeContainsVersion(t *testing.T) {
	tests := []struct {
		consumerConstraint string
		compromisedVersion string
		want               bool
	}{
		{"^1.14.0", "1.14.1", true},
		{"~1.14.0", "1.14.1", true},
		{"^1.0.0", "1.14.1", true},
		{"^2.0.0", "1.14.1", false},
		{"^0.30.0", "0.30.4", true},
		{"1.14.0", "1.14.1", false},
		{"*", "1.14.1", true},
	}
	for _, tt := range tests {
		got := semverSatisfies(tt.consumerConstraint, tt.compromisedVersion)
		assert.Equal(t, tt.want, got, "constraint %s contains %s", tt.consumerConstraint, tt.compromisedVersion)
	}
}
