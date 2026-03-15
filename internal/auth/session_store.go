package auth

import (
	"sync"

	"github.com/zerodha/simplesessions/v3"
)

// memoryStore is a lightweight in-memory session store used in PocketBase-only mode.
type memoryStore struct {
	mu   sync.RWMutex
	data map[string]map[string]any
}

func newMemoryStore() *memoryStore {
	return &memoryStore{data: map[string]map[string]any{}}
}

func (s *memoryStore) Create(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data[id]; !ok {
		s.data[id] = map[string]any{}
	}
	return nil
}

func (s *memoryStore) Get(id, key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.data[id]
	if !ok {
		return nil, simplesessions.ErrInvalidSession
	}
	return m[key], nil
}

func (s *memoryStore) GetMulti(id string, keys ...string) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.data[id]
	if !ok {
		return nil, simplesessions.ErrInvalidSession
	}

	out := make(map[string]any, len(keys))
	for _, k := range keys {
		out[k] = m[k]
	}
	return out, nil
}

func (s *memoryStore) GetAll(id string) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.data[id]
	if !ok {
		return nil, simplesessions.ErrInvalidSession
	}

	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out, nil
}

func (s *memoryStore) Set(id, key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.data[id]
	if !ok {
		return simplesessions.ErrInvalidSession
	}
	m[key] = value
	return nil
}

func (s *memoryStore) SetMulti(id string, data map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.data[id]
	if !ok {
		return simplesessions.ErrInvalidSession
	}
	for k, v := range data {
		m[k] = v
	}
	return nil
}

func (s *memoryStore) Delete(id string, keys ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.data[id]
	if !ok {
		return simplesessions.ErrInvalidSession
	}
	for _, k := range keys {
		delete(m, k)
	}
	return nil
}

func (s *memoryStore) Clear(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data[id]; !ok {
		return simplesessions.ErrInvalidSession
	}
	s.data[id] = map[string]any{}
	return nil
}

func (s *memoryStore) Destroy(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data[id]; !ok {
		return simplesessions.ErrInvalidSession
	}
	delete(s.data, id)
	return nil
}

func (s *memoryStore) Int(r any, err error) (int, error) {
	if err != nil {
		return 0, err
	}
	switch v := r.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	default:
		return 0, simplesessions.ErrAssertType
	}
}

func (s *memoryStore) Int64(r any, err error) (int64, error) {
	if err != nil {
		return 0, err
	}
	switch v := r.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	default:
		return 0, simplesessions.ErrAssertType
	}
}

func (s *memoryStore) UInt64(r any, err error) (uint64, error) {
	if err != nil {
		return 0, err
	}
	switch v := r.(type) {
	case int:
		return uint64(v), nil
	case int64:
		return uint64(v), nil
	case float64:
		return uint64(v), nil
	default:
		return 0, simplesessions.ErrAssertType
	}
}

func (s *memoryStore) Float64(r any, err error) (float64, error) {
	if err != nil {
		return 0, err
	}
	switch v := r.(type) {
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case float64:
		return v, nil
	default:
		return 0, simplesessions.ErrAssertType
	}
}

func (s *memoryStore) String(r any, err error) (string, error) {
	if err != nil {
		return "", err
	}
	v, ok := r.(string)
	if !ok {
		return "", simplesessions.ErrAssertType
	}
	return v, nil
}

func (s *memoryStore) Bytes(r any, err error) ([]byte, error) {
	v, err := s.String(r, err)
	if err != nil {
		return nil, err
	}
	return []byte(v), nil
}

func (s *memoryStore) Bool(r any, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	v, ok := r.(bool)
	if !ok {
		return false, simplesessions.ErrAssertType
	}
	return v, nil
}

