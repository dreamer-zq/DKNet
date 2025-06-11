package storage

import (
	"context"
)

// Storage defines the interface for persistent storage
type Storage interface {
	// Save stores a key-value pair
	Save(ctx context.Context, key string, value []byte) error
	
	// Load retrieves the value for a given key
	Load(ctx context.Context, key string) ([]byte, error)
	
	// Delete removes a key-value pair
	Delete(ctx context.Context, key string) error
	
	// List returns all keys with the given prefix
	List(ctx context.Context, prefix string) ([]string, error)
	
	// Exists checks if a key exists
	Exists(ctx context.Context, key string) (bool, error)
	
	// Close closes the storage
	Close() error
}