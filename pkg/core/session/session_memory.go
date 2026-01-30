package session

import (
	"context"
	"sync"
)

type MemorySession struct {
	data map[string]any
	mu   sync.RWMutex
}

func (s *MemorySession) Get(_ context.Context, key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, exists := s.data[key]
	if !exists {
		return nil, ErrKeyNotFound
	}
	return val, nil
}

func (s *MemorySession) Set(_ context.Context, key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

func (s *MemorySession) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

type MemorySessionManager struct {
	sessions map[int64]*MemorySession
	mu       sync.RWMutex
}

func NewMemorySessionManager() (SessionManager, error) {
	return &MemorySessionManager{
		sessions: make(map[int64]*MemorySession),
	}, nil
}

func (m *MemorySessionManager) Get(_ context.Context, chatID int64) (Session, error) {
	m.mu.RLock()
	session, exists := m.sessions[chatID]
	m.mu.RUnlock()
	if !exists {
		m.mu.Lock()
		session = m.sessions[chatID]
		if session == nil {
			session = &MemorySession{
				data: make(map[string]any),
			}
			m.sessions[chatID] = session
		}
		m.mu.Unlock()
	}
	return session, nil
}
