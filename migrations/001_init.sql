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
