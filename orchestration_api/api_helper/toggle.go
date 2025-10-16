package api_helper

import (
	"fmt"
	"sync"
)

type ToggleStore struct {
	mu      sync.RWMutex
	toggles map[string]bool
}

func NewToggleStore(tokens []string) *ToggleStore {
	m := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		m[t] = false
	}
	return &ToggleStore{toggles: m}
}

func (s *ToggleStore) Toggle(token string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.toggles[token]
	if !ok {
		return false, fmt.Errorf("unknown token: %s", token)
	}

	s.toggles[token] = !s.toggles[token]
	return s.toggles[token], nil
}

func (s *ToggleStore) Get(token string) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.toggles[token]
	return v, ok
}

func (s *ToggleStore) Snapshot() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// copy so caller can’t mutate internal state
	cp := make(map[string]bool, len(s.toggles))
	for k, v := range s.toggles {
		cp[k] = v
	}
	return cp
}
