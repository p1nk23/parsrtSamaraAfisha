package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type FileStore[T any] struct{ Path string }

func (s FileStore[T]) Save(v T) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, b, 0644)
}

func (s FileStore[T]) Load() (T, error) {
	var v T
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return v, os.ErrNotExist
		}
		return v, err
	}
	return v, json.Unmarshal(b, &v)
}
