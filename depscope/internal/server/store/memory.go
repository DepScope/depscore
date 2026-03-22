package store

import (
	"fmt"
	"sync"
	"time"

	"github.com/depscope/depscope/internal/core"
)

// MemoryStore is a sync.Map-based in-memory implementation of ScanStore.
type MemoryStore struct {
	m sync.Map
}

// NewMemoryStore returns a new MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

// Create stores a new ScanJob with status "queued" and CreatedAt set to now.
func (s *MemoryStore) Create(id string, req ScanRequest) error {
	job := &ScanJob{
		ID:        id,
		URL:       req.URL,
		Profile:   req.Profile,
		Status:    "queued",
		CreatedAt: time.Now(),
	}
	s.m.Store(id, job)
	return nil
}

// UpdateStatus changes the status of an existing job.
func (s *MemoryStore) UpdateStatus(id string, status string) error {
	job, err := s.load(id)
	if err != nil {
		return err
	}
	job.Status = status
	s.m.Store(id, job)
	return nil
}

// SaveResult stores the result and sets status to "complete".
func (s *MemoryStore) SaveResult(id string, result *core.ScanResult) error {
	job, err := s.load(id)
	if err != nil {
		return err
	}
	job.Result = result
	job.Status = "complete"
	s.m.Store(id, job)
	return nil
}

// SaveError stores the error message and sets status to "failed".
func (s *MemoryStore) SaveError(id string, errMsg string) error {
	job, err := s.load(id)
	if err != nil {
		return err
	}
	job.Error = errMsg
	job.Status = "failed"
	s.m.Store(id, job)
	return nil
}

// Get retrieves a job by ID. Returns an error if not found.
func (s *MemoryStore) Get(id string) (*ScanJob, error) {
	return s.load(id)
}

func (s *MemoryStore) load(id string) (*ScanJob, error) {
	v, ok := s.m.Load(id)
	if !ok {
		return nil, fmt.Errorf("job not found: %s", id)
	}
	return v.(*ScanJob), nil
}
