CREATE TABLE IF NOT EXISTS finance_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind VARCHAR(20) NOT NULL
        CHECK (kind IN ('income', 'expense')),
    amount DOUBLE PRECISION NOT NULL
        CHECK (amount > 0),
    currency VARCHAR(10) NOT NULL DEFAULT 'THB',
    category VARCHAR(100) NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    occurred_on DATE NOT NULL DEFAULT CURRENT_DATE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS finance_transactions_user_occurred_idx
    ON finance_transactions (user_id, occurred_on DESC);

CREATE INDEX IF NOT EXISTS finance_transactions_user_kind_idx
    ON finance_transactions (user_id, kind);

CREATE INDEX IF NOT EXISTS finance_transactions_user_category_idx
    ON finance_transactions (user_id, category);
