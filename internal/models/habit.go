package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	HabitStatusActive   = "active"
	HabitStatusPaused   = "paused"
	HabitStatusArchived = "archived"

	HabitFrequencyDaily  = "daily"
	HabitFrequencyWeekly = "weekly"
)

type Habit struct {
	ID            uuid.UUID  `json:"id"`
	UserID        uuid.UUID  `json:"user_id"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	Status        string     `json:"status"`
	Frequency     string     `json:"frequency"`
	TargetCount   int        `json:"target_count"`
	Category      string     `json:"category"`
	Color         string     `json:"color"`
	CurrentStreak int        `json:"current_streak"`
	BestStreak    int        `json:"best_streak"`
	ArchivedAt    *time.Time `json:"archived_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type HabitLog struct {
	ID        uuid.UUID `json:"id"`
	HabitID   uuid.UUID `json:"habit_id"`
	UserID    uuid.UUID `json:"user_id"`
	LogDate   time.Time `json:"log_date"`
	Count     int       `json:"count"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type HabitStats struct {
	Total          int `json:"total"`
	Active         int `json:"active"`
	Paused         int `json:"paused"`
	Archived       int `json:"archived"`
	CompletedToday int `json:"completed_today"`
	PendingToday   int `json:"pending_today"`
	AvgStreak      int `json:"avg_streak"`
	BestStreak     int `json:"best_streak"`
}

type DayCount struct {
	Date  string `json:"date"` // YYYY-MM-DD
	Count int    `json:"count"`
}

type HabitDashboard struct {
	Stats         HabitStats       `json:"stats"`
	ByStatus      []NamedCount     `json:"by_status"`
	ByFrequency   []NamedCount     `json:"by_frequency"`
	ByCategory    []NamedCount     `json:"by_category"`
	CheckinTrend  []DayCount       `json:"checkin_trend"`
	StreakBuckets []ProgressBucket `json:"streak_buckets"`
}
