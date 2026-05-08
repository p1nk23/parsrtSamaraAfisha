package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"checker-parser-service/internal/api"
)

// PostgresEventRepository is intentionally implemented through the psql CLI in this prototype.
// It keeps the service free from Go driver dependencies; the Docker image installs postgresql-client.
// For production, replace this file with pgx/sqlc or database/sql + a PostgreSQL driver.
type PostgresEventRepository struct {
	DatabaseURL string
	PSQLBin     string
}

func NewPostgresEventRepository(databaseURL string) (*PostgresEventRepository, error) {
	repo := &PostgresEventRepository{DatabaseURL: databaseURL, PSQLBin: envDefault("PSQL_BIN", "psql")}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := repo.runSQL(ctx, schemaSQL); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *PostgresEventRepository) Close() error { return nil }

func (r *PostgresEventRepository) SaveEvents(events []api.Event) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var b strings.Builder
	b.WriteString("BEGIN;\nDELETE FROM event_sessions;\nDELETE FROM events;\n")
	now := time.Now().UTC().Format(time.RFC3339)
	for i, event := range events {
		id := event.ID
		if id == 0 {
			id = i + 1
		}
		if event.CreatedAt == "" {
			event.CreatedAt = now
		}
		if event.UpdatedAt == "" {
			event.UpdatedAt = now
		}
		if event.Status == "" {
			event.Status = "published"
		}
		b.WriteString(fmt.Sprintf(`INSERT INTO events (id,title,subtitle,short_description,full_description,category,age_restriction,cover_image_url,organizer_name,organizer_description,status,created_at,updated_at) VALUES (%d,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);`+"\n",
			id, sqlString(event.Title), sqlNullableString(event.Subtitle), sqlString(event.ShortDescription), sqlString(event.FullDescription), sqlString(event.Category), sqlNullableInt(event.AgeRestriction), sqlNullableString(event.CoverImageURL), sqlString(event.OrganizerName), sqlString(event.OrganizerDescription), sqlString(event.Status), sqlString(event.CreatedAt), sqlString(event.UpdatedAt)))
		for j, session := range event.EventSessions {
			sid := session.ID
			if sid == 0 {
				sid = id*1000 + j + 1
			}
			if session.Status == "" {
				session.Status = "published"
			}
			if session.CreatedAt == "" {
				session.CreatedAt = event.CreatedAt
			}
			if session.UpdatedAt == "" {
				session.UpdatedAt = event.UpdatedAt
			}
			venueName, venueAddress := "", ""
			if session.Venue != nil {
				venueName = session.Venue.Name
				venueAddress = session.Venue.Address
			}
			b.WriteString(fmt.Sprintf(`INSERT INTO event_sessions (id,event_id,start_at,end_at,is_online,ticket_min_price,ticket_max_price,currency,ticket_url,ticket_service_name,status,created_at,updated_at,venue_name,venue_address) VALUES (%d,%d,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);`+"\n",
				sid, id, sqlString(session.StartAt), sqlNullableStringPtr(session.EndAt), sqlBool(session.IsOnline), sqlNullableInt(session.TicketMinPrice), sqlNullableInt(session.TicketMaxPrice), sqlNullableStringPtr(session.Currency), sqlNullableStringPtr(session.TicketURL), sqlNullableStringPtr(session.TicketServiceName), sqlString(session.Status), sqlString(session.CreatedAt), sqlString(session.UpdatedAt), sqlNullableString(venueName), sqlNullableString(venueAddress)))
		}
	}
	b.WriteString("COMMIT;\n")
	_, err := r.runSQL(ctx, b.String())
	return err
}

func (r *PostgresEventRepository) LoadEvents() ([]api.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := r.runSQL(ctx, eventsJSONSQL)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" || trimmed == "null" {
		return nil, errors.New("no events in database")
	}
	var events []api.Event
	if err := json.Unmarshal([]byte(trimmed), &events); err != nil {
		return nil, fmt.Errorf("decode events JSON from postgres: %w; output=%q", err, firstN(trimmed, 500))
	}
	if len(events) == 0 {
		return nil, errors.New("no events in database")
	}
	return events, nil
}

func (r *PostgresEventRepository) runSQL(ctx context.Context, sql string) (string, error) {
	cmd := exec.CommandContext(ctx, r.PSQLBin, r.DatabaseURL, "-v", "ON_ERROR_STOP=1", "-q", "-t", "-A", "-c", sql)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("psql failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func sqlString(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
func sqlNullableString(s string) string {
	if s == "" {
		return "NULL"
	}
	return sqlString(s)
}
func sqlNullableStringPtr(s *string) string {
	if s == nil || *s == "" {
		return "NULL"
	}
	return sqlString(*s)
}
func sqlNullableInt(v *int) string {
	if v == nil {
		return "NULL"
	}
	return strconv.Itoa(*v)
}
func sqlBool(v bool) string {
	if v {
		return "TRUE"
	}
	return "FALSE"
}
func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
func envDefault(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY,
    title TEXT NOT NULL,
    subtitle TEXT,
    short_description TEXT NOT NULL DEFAULT '',
    full_description TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT 'other',
    age_restriction INTEGER,
    cover_image_url TEXT,
    organizer_name TEXT NOT NULL DEFAULT '',
    organizer_description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'published',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS event_sessions (
    id INTEGER PRIMARY KEY,
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    start_at TEXT NOT NULL,
    end_at TEXT,
    is_online BOOLEAN NOT NULL DEFAULT FALSE,
    ticket_min_price INTEGER,
    ticket_max_price INTEGER,
    currency TEXT,
    ticket_url TEXT,
    ticket_service_name TEXT,
    status TEXT NOT NULL DEFAULT 'published',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    venue_name TEXT,
    venue_address TEXT
);
CREATE INDEX IF NOT EXISTS idx_event_sessions_event_id ON event_sessions(event_id);
`

const eventsJSONSQL = `
SELECT COALESCE(json_agg(event_json ORDER BY id), '[]'::json)::text
FROM (
  SELECT e.id,
         json_build_object(
           'id', e.id,
           'title', e.title,
           'subtitle', COALESCE(e.subtitle, ''),
           'shortDescription', e.short_description,
           'fullDescription', e.full_description,
           'category', e.category,
           'ageRestriction', e.age_restriction,
           'coverImageUrl', COALESCE(e.cover_image_url, ''),
           'organizerName', e.organizer_name,
           'organizerDescription', e.organizer_description,
           'status', e.status,
           'createdAt', e.created_at,
           'updatedAt', e.updated_at,
           'eventSessions', COALESCE((
             SELECT json_agg(json_build_object(
               'id', s.id,
               'startAt', s.start_at,
               'endAt', s.end_at,
               'is_online', s.is_online,
               'ticketMinPrice', s.ticket_min_price,
               'ticketMaxPrice', s.ticket_max_price,
               'currency', s.currency,
               'ticketUrl', s.ticket_url,
               'ticketServiceName', s.ticket_service_name,
               'status', s.status,
               'createdAt', s.created_at,
               'updatedAt', s.updated_at,
               'venue', CASE WHEN s.venue_name IS NULL AND s.venue_address IS NULL THEN NULL ELSE json_build_object('name', COALESCE(s.venue_name, ''), 'address', COALESCE(s.venue_address, '')) END
             ) ORDER BY s.id)
             FROM event_sessions s WHERE s.event_id = e.id
           ), '[]'::json)
         ) AS event_json
  FROM events e
) q;
`
