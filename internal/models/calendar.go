package models

import (
	"time"

	"github.com/google/uuid"
)

type CalendarEvent struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	Color       string    `json:"color"`
	AllDay      bool      `json:"all_day"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CalendarStats struct {
	Total    int `json:"total"`
	Upcoming int `json:"upcoming"`
	AllDay   int `json:"all_day"`
}
