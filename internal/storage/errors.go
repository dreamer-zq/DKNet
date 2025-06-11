package storage

import "errors"

var (
	// ErrNotFound is returned when a key is not found in storage
	ErrNotFound = errors.New("key not found")
	
	// ErrStorageClosed is returned when attempting to use a closed storage
	ErrStorageClosed = errors.New("storage is closed")
	
	// ErrInvalidKey is returned when a key is invalid
	ErrInvalidKey = errors.New("invalid key")
) 