package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	NoteKindNote    = "note"
	NoteKindJournal = "journal"
)

type Note struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Kind      string    `json:"kind"`
	Mood      string    `json:"mood"`
	EntryDate time.Time `json:"entry_date"`
	Tags      string    `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type NoteStats struct {
	Total   int `json:"total"`
	Notes   int `json:"notes"`
	Journal int `json:"journal"`
}
