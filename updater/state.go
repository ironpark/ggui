package updater

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const stateFileName = "updater-state.json"

type persistentState struct {
	SkippedVersion string    `json:"skipped_version,omitempty"`
	LastCheck      time.Time `json:"last_check,omitempty"`
	AutoCheck      *bool     `json:"auto_check,omitempty"`
	RolloutSeed    float64   `json:"rollout_seed"`
	HasRolloutSeed bool      `json:"has_rollout_seed"`
}

type stateStore struct {
	mu    sync.Mutex
	path  string
	state persistentState
}

func loadState(stateDir string) (*stateStore, error) {
	store := &stateStore{
		path: filepath.Join(stateDir, stateFileName),
	}

	data, err := os.ReadFile(store.path)
	switch {
	case err == nil:
		if unmarshalErr := json.Unmarshal(data, &store.state); unmarshalErr != nil {
			// Corrupt state file: start clean and overwrite on save.
			store.state = persistentState{}
		}
	case errors.Is(err, os.ErrNotExist):
		// Fresh state.
	default:
		return nil, fmt.Errorf("updater: reading state file: %w", err)
	}

	if !store.state.HasRolloutSeed || math.IsNaN(store.state.RolloutSeed) || store.state.RolloutSeed < 0 || store.state.RolloutSeed >= 1 {
		seed, seedErr := randomUnitFloat64()
		if seedErr != nil {
			return nil, fmt.Errorf("updater: generating rollout seed: %w", seedErr)
		}
		store.state.RolloutSeed = seed
		store.state.HasRolloutSeed = true
		// Persist the seed best-effort so future launches keep the same rollout bucket.
		_ = store.saveLocked()
	}

	return store, nil
}

func (s *stateStore) skippedVersion() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.SkippedVersion
}

func (s *stateStore) setSkippedVersion(version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.SkippedVersion = version
	return s.saveLocked()
}

func (s *stateStore) lastCheck() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.LastCheck
}

func (s *stateStore) setLastCheck(t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.LastCheck = t
	return s.saveLocked()
}

func (s *stateStore) autoCheck() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.AutoCheck == nil {
		return true
	}
	return *s.state.AutoCheck
}

func (s *stateStore) setAutoCheck(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := enabled
	s.state.AutoCheck = &v
	return s.saveLocked()
}

func (s *stateStore) rolloutSeed() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.RolloutSeed
}

func (s *stateStore) setRolloutSeed(seed float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.RolloutSeed = seed
	s.state.HasRolloutSeed = true
	return s.saveLocked()
}

func (s *stateStore) saveLocked() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("updater: creating state directory: %w", err)
	}
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("updater: encoding state: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".updater-state-*.tmp")
	if err != nil {
		return fmt.Errorf("updater: creating temp state file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("updater: writing temp state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("updater: closing temp state file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("updater: replacing state file: %w", err)
	}
	cleanup = false
	return nil
}

// randomUnitFloat64 returns a cryptographically random float64 uniformly
// distributed in [0, 1).
func randomUnitFloat64() (float64, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, err
	}
	v := binary.BigEndian.Uint64(buf[:]) >> 11 // 53 random bits
	return float64(v) / float64(1<<53), nil
}
