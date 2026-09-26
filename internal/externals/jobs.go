package externals

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type DiscoveryJob struct {
	ID           string `json:"id"`
	ConnectionID string `json:"connectionId"`
	Status       string `json:"status"`
	StartedAt    string `json:"startedAt"`
	FinishedAt   string `json:"finishedAt,omitempty"`
	Error        string `json:"error,omitempty"`
}

func (s *Store) SaveJob(j DiscoveryJob) error {
	l, err := s.lock(j.ConnectionID)
	if err != nil {
		return err
	}
	defer l.Unlock()
	return atomicJSON(filepath.Join(s.dir(j.ConnectionID), "job.json"), j)
}
func (s *Store) Job(id string) (DiscoveryJob, error) {
	var j DiscoveryJob
	if err := s.CatalogID(id); err != nil {
		return j, err
	}
	b, err := os.ReadFile(filepath.Join(s.dir(id), "job.json"))
	if errors.Is(err, os.ErrNotExist) {
		return DiscoveryJob{ConnectionID: id, Status: "idle"}, nil
	}
	if err != nil {
		return j, err
	}
	if len(b) > 16384 {
		return j, errors.New("探索任务记录过大")
	}
	err = json.Unmarshal(b, &j)
	return j, err
}
func (s *Store) CatalogID(id string) error { _, err := s.Read(id); return err }
