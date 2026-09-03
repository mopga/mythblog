package storage

import (
	"context"
	"sync"
)

type Memory struct {
	mu      sync.RWMutex
	objects map[string][]byte
	baseURL string
}

func NewMemory(baseURL string) *Memory {
	return &Memory{objects: map[string][]byte{}, baseURL: baseURL}
}
func (m *Memory) Put(_ context.Context, key string, data []byte, _ string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = append([]byte(nil), data...)
	return m.URL(key), nil
}
func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}
func (m *Memory) URL(key string) string { return m.baseURL + "/" + key }
func (m *Memory) Bytes(key string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.objects[key]
	return append([]byte(nil), v...), ok
}
func (m *Memory) Count() int { m.mu.RLock(); defer m.mu.RUnlock(); return len(m.objects) }
