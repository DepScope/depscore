package server_test

import (
	"testing"

	"github.com/depscope/depscope/internal/server"
)

func TestValidateScanURL_Valid(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"github", "https://github.com/org/repo"},
		{"github with path", "https://github.com/org/repo/tree/main"},
		{"gitlab", "https://gitlab.com/org/repo"},
		{"gitlab with ref", "https://gitlab.com/org/repo/-/tree/v2"},
		{"bitbucket", "https://bitbucket.org/org/repo"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := server.ValidateScanURL(tc.url); err != nil {
				t.Errorf("ValidateScanURL(%q) = %v, want nil", tc.url, err)
			}
		})
	}
}

func TestValidateScanURL_Invalid(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"http", "http://github.com/org/repo"},
		{"no scheme", "github.com/org/repo"},
		{"garbage", "not-a-url"},
		{"localhost named", "https://localhost/org/repo"},
		{"localhost subdomain", "https://foo.localhost/org/repo"},
		{"127 loopback", "https://127.0.0.1/org/repo"},
		{"127 other", "https://127.1.2.3/org/repo"},
		{"10.x private", "https://10.0.0.1/org/repo"},
		{"10.x deep", "https://10.255.255.255/org/repo"},
		{"172.16 private", "https://172.16.0.1/org/repo"},
		{"172.31 private", "https://172.31.255.255/org/repo"},
		{"192.168 private", "https://192.168.1.1/org/repo"},
		{"link-local", "https://169.254.169.254/latest/meta-data/"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := server.ValidateScanURL(tc.url); err == nil {
				t.Errorf("ValidateScanURL(%q) = nil, want error", tc.url)
			}
		})
	}
}
