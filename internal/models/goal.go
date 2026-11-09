package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	GoalStatusDraft     = "draft"
	GoalStatusActive    = "active"
	GoalStatusCompleted = "completed"
	GoalStatusCancelled = "cancelled"
	GoalStatusArchived  = "archived"

	GoalPriorityLow    = "low"
	GoalPriorityMedium = "medium"
	GoalPriorityHigh   = "high"
)

type Goal struct {
	ID           uuid.UUID  `json:"id"`
	UserID       uuid.UUID  `json:"user_id"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Status       string     `json:"status"`
	Priority     string     `json:"priority"`
	Category     string     `json:"category"`
	Progress     int        `json:"progress"`
	TargetValue  *float64   `json:"target_value,omitempty"`
	CurrentValue float64    `json:"current_value"`
	Unit         string     `json:"unit"`
	StartDate    *time.Time `json:"start_date,omitempty"`
	TargetDate   *time.Time `json:"target_date,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	ArchivedAt   *time.Time `json:"archived_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type GoalMilestone struct {
	ID          uuid.UUID  `json:"id"`
	GoalID      uuid.UUID  `json:"goal_id"`
	UserID      uuid.UUID  `json:"user_id"`
	Title       string     `json:"title"`
	IsCompleted bool       `json:"is_completed"`
	SortOrder   int        `json:"sort_order"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type GoalStats struct {
	Total       int `json:"total"`
	Draft       int `json:"draft"`
	Active      int `json:"active"`
	Completed   int `json:"completed"`
	Cancelled   int `json:"cancelled"`
	Archived    int `json:"archived"`
	Overdue     int `json:"overdue"`
	AvgProgress int `json:"avg_progress"`
}

type NamedCount struct {
	Key   string `json:"key"`
	Label string `json:"label,omitempty"`
	Count int    `json:"count"`
}

type MonthCount struct {
	Month     string `json:"month"` // YYYY-MM
	Created   int    `json:"created"`
	Completed int    `json:"completed"`
}

type ProgressBucket struct {
	Bucket string `json:"bucket"` // e.g. "0-20"
	Count  int    `json:"count"`
}

type GoalDashboard struct {
	Stats           GoalStats        `json:"stats"`
	ByStatus        []NamedCount     `json:"by_status"`
	ByPriority      []NamedCount     `json:"by_priority"`
	ByCategory      []NamedCount     `json:"by_category"`
	CreatedTrend    []MonthCount     `json:"created_trend"`
	ProgressBuckets []ProgressBucket `json:"progress_buckets"`
}
