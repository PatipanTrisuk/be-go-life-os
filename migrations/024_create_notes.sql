CREATE TABLE IF NOT EXISTS notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    kind VARCHAR(20) NOT NULL DEFAULT 'note'
        CHECK (kind IN ('note', 'journal')),
    mood VARCHAR(50) NOT NULL DEFAULT '',
    entry_date DATE NOT NULL DEFAULT CURRENT_DATE,
    tags TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS notes_user_kind_idx
    ON notes (user_id, kind);

CREATE INDEX IF NOT EXISTS notes_user_entry_date_idx
    ON notes (user_id, entry_date DESC);

CREATE INDEX IF NOT EXISTS notes_user_created_idx
    ON notes (user_id, created_at DESC);
