CREATE TABLE IF NOT EXISTS plans (
    code VARCHAR(32) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS plan_entitlements (
    plan_code VARCHAR(32) NOT NULL REFERENCES plans(code) ON DELETE CASCADE,
    feature_key VARCHAR(64) NOT NULL,
    limit_value INT NOT NULL,
    PRIMARY KEY (plan_code, feature_key)
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    plan_code VARCHAR(32) NOT NULL REFERENCES plans(code),
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active')),
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS subscriptions_plan_code_idx
    ON subscriptions (plan_code);

INSERT INTO plans (code, name, description, is_active)
VALUES
    ('free', 'Free', 'Core LifeOS with monthly item caps', TRUE),
    ('pro', 'Pro', 'Unlimited LifeOS items', TRUE)
ON CONFLICT (code) DO NOTHING;

INSERT INTO plan_entitlements (plan_code, feature_key, limit_value)
VALUES
    ('free', 'limit.goals', 5),
    ('free', 'limit.habits', 5),
    ('free', 'limit.planner_per_day', 20),
    ('free', 'limit.calendar', 30),
    ('free', 'limit.notes', 30),
    ('free', 'limit.finance', 50),
    ('free', 'limit.reminders', 15),
    ('pro', 'limit.goals', -1),
    ('pro', 'limit.habits', -1),
    ('pro', 'limit.planner_per_day', -1),
    ('pro', 'limit.calendar', -1),
    ('pro', 'limit.notes', -1),
    ('pro', 'limit.finance', -1),
    ('pro', 'limit.reminders', -1)
ON CONFLICT (plan_code, feature_key) DO NOTHING;

INSERT INTO subscriptions (user_id, plan_code, status)
SELECT id, 'free', 'active'
FROM users
ON CONFLICT (user_id) DO NOTHING;
