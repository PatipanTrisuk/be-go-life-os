CREATE TABLE IF NOT EXISTS planner_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_date DATE NOT NULL,
    title VARCHAR(200) NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'todo'
        CHECK (status IN ('todo', 'done', 'skipped')),
    priority VARCHAR(20) NOT NULL DEFAULT 'medium'
        CHECK (priority IN ('low', 'medium', 'high')),
    start_time VARCHAR(5) NOT NULL DEFAULT '',
    end_time VARCHAR(5) NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0,
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS planner_tasks_user_date_idx
    ON planner_tasks (user_id, plan_date, sort_order, created_at);

CREATE INDEX IF NOT EXISTS planner_tasks_user_status_idx
    ON planner_tasks (user_id, status);
