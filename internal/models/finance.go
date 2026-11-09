package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	FinanceKindIncome  = "income"
	FinanceKindExpense = "expense"
)

type FinanceTransaction struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	Kind       string    `json:"kind"`
	Amount     float64   `json:"amount"`
	Currency   string    `json:"currency"`
	Category   string    `json:"category"`
	Note       string    `json:"note"`
	OccurredOn time.Time `json:"occurred_on"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type FinanceStats struct {
	Income  float64 `json:"income"`
	Expense float64 `json:"expense"`
	Net     float64 `json:"net"`
	Count   int     `json:"count"`
}
