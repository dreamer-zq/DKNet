package storage

import (
	"context"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// LevelDBStorage implements Storage interface using LevelDB
type LevelDBStorage struct {
	db *leveldb.DB
}

// NewLevelDBStorage creates a new LevelDB storage instance
func NewLevelDBStorage(path string) (*LevelDBStorage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}

	return &LevelDBStorage{
		db: db,
	}, nil
}

// Save stores a key-value pair
func (s *LevelDBStorage) Save(ctx context.Context, key string, value []byte) error {
	return s.db.Put([]byte(key), value, nil)
}

// Load retrieves the value for a given key
func (s *LevelDBStorage) Load(ctx context.Context, key string) ([]byte, error) {
	data, err := s.db.Get([]byte(key), nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return data, nil
}

// Delete removes a key-value pair
func (s *LevelDBStorage) Delete(ctx context.Context, key string) error {
	return s.db.Delete([]byte(key), nil)
}

// List returns all keys with the given prefix
func (s *LevelDBStorage) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string

	iter := s.db.NewIterator(util.BytesPrefix([]byte(prefix)), nil)
	defer iter.Release()

	for iter.Next() {
		keys = append(keys, string(iter.Key()))
	}

	if err := iter.Error(); err != nil {
		return nil, err
	}

	return keys, nil
}

// Exists checks if a key exists
func (s *LevelDBStorage) Exists(ctx context.Context, key string) (bool, error) {
	has, err := s.db.Has([]byte(key), nil)
	return has, err
}

// Close closes the storage
func (s *LevelDBStorage) Close() error {
	return s.db.Close()
}
