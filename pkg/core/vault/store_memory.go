package vault

import (
	"sync"

	"github.com/pkg/errors"
)

var ErrKeyNotFound = errors.New("key not found")

type MemoryStore struct {
	m  map[string]any
	mu sync.RWMutex
}

func NewMemoryStore() Store {
	return &MemoryStore{
		m: make(map[string]any),
	}
}

func (s *MemoryStore) Get(key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, exists := s.m[key]
	if !exists {
		return nil, ErrKeyNotFound
	}
	return val, nil
}

func (s *MemoryStore) Set(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.m[key] = value
	return nil
}

func (s *MemoryStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.m, key)
	return nil
}
