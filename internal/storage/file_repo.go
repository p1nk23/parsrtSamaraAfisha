package storage

import (
	"checker-parser-service/internal/api"
)

type FileEventRepository struct {
	Store FileStore[[]api.Event]
}

func NewFileEventRepository(path string) FileEventRepository {
	return FileEventRepository{Store: FileStore[[]api.Event]{Path: path}}
}

func (r FileEventRepository) SaveEvents(events []api.Event) error {
	return r.Store.Save(events)
}

func (r FileEventRepository) LoadEvents() ([]api.Event, error) {
	return r.Store.Load()
}
