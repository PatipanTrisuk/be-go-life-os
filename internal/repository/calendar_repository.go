package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-be/internal/models"
)

type CalendarRepository struct {
	DB *pgxpool.Pool
}

func NewCalendarRepository(db *pgxpool.Pool) *CalendarRepository {
	return &CalendarRepository{DB: db}
}

type CalendarCreateParams struct {
	UserID      string
	Title       string
	Description string
	Location    string
	Color       string
	AllDay      bool
	StartsAt    time.Time
	EndsAt      time.Time
}

type CalendarUpdateParams struct {
	Title       *string
	Description *string
	Location    *string
	Color       *string
	AllDay      *bool
	StartsAt    *time.Time
	EndsAt      *time.Time
}

const calendarSelect = `
	id, user_id, title, description, location, color, all_day,
	starts_at, ends_at, created_at, updated_at
`

func scanCalendarEvent(scanner interface {
	Scan(dest ...any) error
}, e *models.CalendarEvent) error {
	return scanner.Scan(
		&e.ID,
		&e.UserID,
		&e.Title,
		&e.Description,
		&e.Location,
		&e.Color,
		&e.AllDay,
		&e.StartsAt,
		&e.EndsAt,
		&e.CreatedAt,
		&e.UpdatedAt,
	)
}

func (r *CalendarRepository) Create(ctx context.Context, p CalendarCreateParams) (*models.CalendarEvent, error) {
	row := r.DB.QueryRow(ctx, `
		INSERT INTO calendar_events (
			user_id, title, description, location, color, all_day, starts_at, ends_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+calendarSelect,
		p.UserID, p.Title, p.Description, p.Location, p.Color, p.AllDay, p.StartsAt.UTC(), p.EndsAt.UTC(),
	)
	var e models.CalendarEvent
	if err := scanCalendarEvent(row, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *CalendarRepository) GetByID(ctx context.Context, id, userID string) (*models.CalendarEvent, error) {
	row := r.DB.QueryRow(ctx, `
		SELECT `+calendarSelect+`
		FROM calendar_events
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	var e models.CalendarEvent
	if err := scanCalendarEvent(row, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// ListRange returns events overlapping [from, to].
func (r *CalendarRepository) ListRange(ctx context.Context, userID string, from, to time.Time) ([]models.CalendarEvent, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT `+calendarSelect+`
		FROM calendar_events
		WHERE user_id = $1 AND ends_at >= $2 AND starts_at <= $3
		ORDER BY starts_at ASC, created_at ASC
	`, userID, from.UTC(), to.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.CalendarEvent, 0)
	for rows.Next() {
		var e models.CalendarEvent
		if err := scanCalendarEvent(rows, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *CalendarRepository) Stats(ctx context.Context, userID string, from, to, now time.Time) (*models.CalendarStats, error) {
	s := &models.CalendarStats{}
	err := r.DB.QueryRow(ctx, `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE starts_at > $4)::int,
			COUNT(*) FILTER (WHERE all_day)::int
		FROM calendar_events
		WHERE user_id = $1 AND ends_at >= $2 AND starts_at <= $3
	`, userID, from.UTC(), to.UTC(), now.UTC()).Scan(&s.Total, &s.Upcoming, &s.AllDay)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *CalendarRepository) Update(ctx context.Context, id, userID string, p CalendarUpdateParams) (*models.CalendarEvent, error) {
	sets := make([]string, 0)
	args := make([]any, 0)
	argN := 1
	add := func(col string, val any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, argN))
		args = append(args, val)
		argN++
	}

	if p.Title != nil {
		add("title", *p.Title)
	}
	if p.Description != nil {
		add("description", *p.Description)
	}
	if p.Location != nil {
		add("location", *p.Location)
	}
	if p.Color != nil {
		add("color", *p.Color)
	}
	if p.AllDay != nil {
		add("all_day", *p.AllDay)
	}
	if p.StartsAt != nil {
		add("starts_at", p.StartsAt.UTC())
	}
	if p.EndsAt != nil {
		add("ends_at", p.EndsAt.UTC())
	}
	if len(sets) == 0 {
		return r.GetByID(ctx, id, userID)
	}
	sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id, userID)

	row := r.DB.QueryRow(ctx, `
		UPDATE calendar_events
		SET `+strings.Join(sets, ", ")+`
		WHERE id = $`+fmt.Sprintf("%d", argN)+` AND user_id = $`+fmt.Sprintf("%d", argN+1)+`
		RETURNING `+calendarSelect, args...)
	var e models.CalendarEvent
	if err := scanCalendarEvent(row, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *CalendarRepository) Delete(ctx context.Context, id, userID string) error {
	tag, err := r.DB.Exec(ctx, `DELETE FROM calendar_events WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
