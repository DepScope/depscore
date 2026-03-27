// internal/core/orgscore_test.go
package core

import "testing"

func TestClassifyOrg_OwnOrg(t *testing.T) {
	got := ClassifyOrg("github.com/my-company/repo", []string{"github.com/my-company"})
	if got != "own" {
		t.Errorf("want %q, got %q", "own", got)
	}
}

func TestClassifyOrg_Corporate(t *testing.T) {
	got := ClassifyOrg("github.com/google/repo", nil)
	if got != "corporate" {
		t.Errorf("want %q, got %q", "corporate", got)
	}
}

func TestClassifyOrg_Individual(t *testing.T) {
	got := ClassifyOrg("github.com/someuser/repo", nil)
	if got != "individual" {
		t.Errorf("want %q, got %q", "individual", got)
	}
}

func TestApplyOrgTrust_Own(t *testing.T) {
	got := ApplyOrgTrust(50, "own", 80)
	if got != 80 {
		t.Errorf("want 80, got %d", got)
	}
}

func TestApplyOrgTrust_OwnAboveFloor(t *testing.T) {
	got := ApplyOrgTrust(90, "own", 80)
	if got != 90 {
		t.Errorf("want 90 (already above floor), got %d", got)
	}
}

func TestApplyOrgTrust_Corporate(t *testing.T) {
	got := ApplyOrgTrust(70, "corporate", 80)
	if got != 75 {
		t.Errorf("want 75, got %d", got)
	}
}

func TestApplyOrgTrust_CorporateCap(t *testing.T) {
	got := ApplyOrgTrust(98, "corporate", 80)
	if got != 100 {
		t.Errorf("want 100 (capped), got %d", got)
	}
}

func TestApplyOrgTrust_Individual(t *testing.T) {
	got := ApplyOrgTrust(55, "individual", 80)
	if got != 55 {
		t.Errorf("want 55 (unchanged), got %d", got)
	}
}
