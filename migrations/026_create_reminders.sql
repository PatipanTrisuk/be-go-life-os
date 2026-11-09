CREATE TABLE IF NOT EXISTS reminders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    due_at TIMESTAMP NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'done', 'dismissed')),
    notified_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS reminders_user_status_due_idx
    ON reminders (user_id, status, due_at);

CREATE INDEX IF NOT EXISTS reminders_user_due_idx
    ON reminders (user_id, due_at);

CREATE INDEX IF NOT EXISTS reminders_due_check_idx
    ON reminders (status, due_at)
    WHERE status = 'pending' AND notified_at IS NULL;
