package repository

import "checker-parser-service/internal/api"

type EventRepository interface {
	SaveEvents(events []api.Event) error
	LoadEvents() ([]api.Event, error)
}
