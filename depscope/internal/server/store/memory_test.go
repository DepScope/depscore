package store_test

import (
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/server/store"
)

func newStore(t *testing.T) *store.MemoryStore {
	t.Helper()
	return store.NewMemoryStore()
}

func TestMemoryStore_CreateAndGet(t *testing.T) {
	s := newStore(t)
	req := store.ScanRequest{URL: "https://github.com/org/repo", Profile: "enterprise"}

	if err := s.Create("job-1", req); err != nil {
		t.Fatalf("Create: %v", err)
	}

	job, err := s.Get("job-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if job.ID != "job-1" {
		t.Errorf("ID: got %q, want %q", job.ID, "job-1")
	}
	if job.URL != req.URL {
		t.Errorf("URL: got %q, want %q", job.URL, req.URL)
	}
	if job.Profile != req.Profile {
		t.Errorf("Profile: got %q, want %q", job.Profile, req.Profile)
	}
	if job.Status != "queued" {
		t.Errorf("Status: got %q, want %q", job.Status, "queued")
	}
	if job.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestMemoryStore_UpdateStatus(t *testing.T) {
	s := newStore(t)
	_ = s.Create("job-2", store.ScanRequest{URL: "https://github.com/org/repo", Profile: "hobby"})

	if err := s.UpdateStatus("job-2", "running"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	job, _ := s.Get("job-2")
	if job.Status != "running" {
		t.Errorf("Status: got %q, want %q", job.Status, "running")
	}
}

func TestMemoryStore_SaveResult(t *testing.T) {
	s := newStore(t)
	_ = s.Create("job-3", store.ScanRequest{URL: "https://github.com/org/repo", Profile: "enterprise"})
	_ = s.UpdateStatus("job-3", "running")

	result := &core.ScanResult{
		Profile:       "enterprise",
		PassThreshold: 70,
		DirectDeps:    2,
	}
	if err := s.SaveResult("job-3", result); err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	job, _ := s.Get("job-3")
	if job.Status != "complete" {
		t.Errorf("Status: got %q, want %q", job.Status, "complete")
	}
	if job.Result == nil {
		t.Fatal("Result should not be nil")
	}
	if job.Result.DirectDeps != 2 {
		t.Errorf("DirectDeps: got %d, want %d", job.Result.DirectDeps, 2)
	}
}

func TestMemoryStore_SaveError(t *testing.T) {
	s := newStore(t)
	_ = s.Create("job-4", store.ScanRequest{URL: "https://github.com/org/repo", Profile: "enterprise"})
	_ = s.UpdateStatus("job-4", "running")

	if err := s.SaveError("job-4", "something went wrong"); err != nil {
		t.Fatalf("SaveError: %v", err)
	}

	job, _ := s.Get("job-4")
	if job.Status != "failed" {
		t.Errorf("Status: got %q, want %q", job.Status, "failed")
	}
	if job.Error != "something went wrong" {
		t.Errorf("Error: got %q, want %q", job.Error, "something went wrong")
	}
}

func TestMemoryStore_GetNotFound(t *testing.T) {
	s := newStore(t)

	_, err := s.Get("does-not-exist")
	if err == nil {
		t.Error("Get on missing ID should return an error")
	}
}
