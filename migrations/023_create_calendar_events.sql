CREATE TABLE IF NOT EXISTS calendar_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    location VARCHAR(200) NOT NULL DEFAULT '',
    color VARCHAR(30) NOT NULL DEFAULT '',
    all_day BOOLEAN NOT NULL DEFAULT FALSE,
    starts_at TIMESTAMP NOT NULL,
    ends_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (ends_at >= starts_at)
);

CREATE INDEX IF NOT EXISTS calendar_events_user_range_idx
    ON calendar_events (user_id, starts_at, ends_at);

CREATE INDEX IF NOT EXISTS calendar_events_user_created_idx
    ON calendar_events (user_id, created_at DESC);
