CREATE TABLE IF NOT EXISTS habits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'paused', 'archived')),
    frequency VARCHAR(20) NOT NULL DEFAULT 'daily'
        CHECK (frequency IN ('daily', 'weekly')),
    target_count INT NOT NULL DEFAULT 1
        CHECK (target_count >= 1 AND target_count <= 100),
    category VARCHAR(100) NOT NULL DEFAULT '',
    color VARCHAR(30) NOT NULL DEFAULT '',
    current_streak INT NOT NULL DEFAULT 0
        CHECK (current_streak >= 0),
    best_streak INT NOT NULL DEFAULT 0
        CHECK (best_streak >= 0),
    archived_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS habits_user_status_idx
    ON habits (user_id, status);

CREATE INDEX IF NOT EXISTS habits_user_created_idx
    ON habits (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS habit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    habit_id UUID NOT NULL REFERENCES habits(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    log_date DATE NOT NULL,
    count INT NOT NULL DEFAULT 1
        CHECK (count >= 1 AND count <= 100),
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (habit_id, log_date)
);

CREATE INDEX IF NOT EXISTS habit_logs_habit_date_idx
    ON habit_logs (habit_id, log_date DESC);

CREATE INDEX IF NOT EXISTS habit_logs_user_date_idx
    ON habit_logs (user_id, log_date DESC);
