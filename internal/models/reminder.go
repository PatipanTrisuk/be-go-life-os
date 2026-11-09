package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	ReminderStatusPending   = "pending"
	ReminderStatusDone      = "done"
	ReminderStatusDismissed = "dismissed"
)

type Reminder struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	DueAt      time.Time  `json:"due_at"`
	Status     string     `json:"status"`
	NotifiedAt *time.Time `json:"notified_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type ReminderStats struct {
	Pending   int `json:"pending"`
	Done      int `json:"done"`
	Dismissed int `json:"dismissed"`
	Overdue   int `json:"overdue"`
}
