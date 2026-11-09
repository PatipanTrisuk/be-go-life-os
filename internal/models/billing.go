package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	PlanCodeFree = "free"
	PlanCodePro  = "pro"

	SubscriptionStatusActive = "active"

	FeatureLimitGoals         = "limit.goals"
	FeatureLimitHabits        = "limit.habits"
	FeatureLimitPlannerPerDay = "limit.planner_per_day"
	FeatureLimitCalendar      = "limit.calendar"
	FeatureLimitNotes         = "limit.notes"
	FeatureLimitFinance       = "limit.finance"
	FeatureLimitReminders     = "limit.reminders"
)

type Plan struct {
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PlanEntitlement struct {
	PlanCode   string `json:"plan_code"`
	FeatureKey string `json:"feature_key"`
	LimitValue int    `json:"limit_value"`
}

type Subscription struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	PlanCode  string    `json:"plan_code"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UsageItem struct {
	Feature string `json:"feature"`
	Usage   int    `json:"usage"`
	Limit   int    `json:"limit"`
}
