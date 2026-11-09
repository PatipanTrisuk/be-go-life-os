package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	PlannerStatusTodo    = "todo"
	PlannerStatusDone    = "done"
	PlannerStatusSkipped = "skipped"

	PlannerPriorityLow    = "low"
	PlannerPriorityMedium = "medium"
	PlannerPriorityHigh   = "high"
)

type PlannerTask struct {
	ID          uuid.UUID  `json:"id"`
	UserID      uuid.UUID  `json:"user_id"`
	PlanDate    time.Time  `json:"plan_date"`
	Title       string     `json:"title"`
	Notes       string     `json:"notes"`
	Status      string     `json:"status"`
	Priority    string     `json:"priority"`
	StartTime   string     `json:"start_time"`
	EndTime     string     `json:"end_time"`
	SortOrder   int        `json:"sort_order"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type PlannerDayStats struct {
	Date       string `json:"date"`
	Total      int    `json:"total"`
	Todo       int    `json:"todo"`
	Done       int    `json:"done"`
	Skipped    int    `json:"skipped"`
	High       int    `json:"high"`
	Completion int    `json:"completion"` // percent
}
