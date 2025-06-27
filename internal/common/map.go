package common

import "sync"

// SafeMap is a thread-safe map
type SafeMap[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

// New creates a new SafeMap
func New[K comparable, V any]() *SafeMap[K, V] {
	return &SafeMap[K, V]{
		m: make(map[K]V),
	}
}

// Set stores a key-value pair in the map
func (s *SafeMap[K, V]) Set(key K, value V) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
}

// Get loads a value from the map
func (s *SafeMap[K, V]) Get(key K) (V, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.m[key]
	return val, ok
}

// Delete deletes a key-value pair from the map
func (s *SafeMap[K, V]) Delete(key K) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
}

// Range iterates over the map
func (s *SafeMap[K, V]) Range(f func(K, V) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for k, v := range s.m {
		if !f(k, v) {
			break
		}
	}
}

// Len returns the number of key-value pairs in the map
func (s *SafeMap[K, V]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.m)
}

// Has checks if a key exists in the map
func (s *SafeMap[K, V]) Has(key K) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.m[key]
	return ok
}