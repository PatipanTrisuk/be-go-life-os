package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-be/internal/models"
)

type ReminderRepository struct {
	DB *pgxpool.Pool
}

func NewReminderRepository(db *pgxpool.Pool) *ReminderRepository {
	return &ReminderRepository{DB: db}
}

type ReminderCreateParams struct {
	UserID string
	Title  string
	Body   string
	DueAt  time.Time
	Status string
}

type ReminderUpdateParams struct {
	Title  *string
	Body   *string
	DueAt  *time.Time
	Status *string
	// ResetNotified clears notified_at (used when due_at is rescheduled or reopened).
	ResetNotified bool
}

const reminderSelect = `
	id, user_id, title, body, due_at, status, notified_at, created_at, updated_at
`

func scanReminder(scanner interface {
	Scan(dest ...any) error
}, rm *models.Reminder) error {
	var notified sql.NullTime
	if err := scanner.Scan(
		&rm.ID,
		&rm.UserID,
		&rm.Title,
		&rm.Body,
		&rm.DueAt,
		&rm.Status,
		&notified,
		&rm.CreatedAt,
		&rm.UpdatedAt,
	); err != nil {
		return err
	}
	if notified.Valid {
		v := notified.Time
		rm.NotifiedAt = &v
	}
	return nil
}

func (r *ReminderRepository) Create(ctx context.Context, p ReminderCreateParams) (*models.Reminder, error) {
	row := r.DB.QueryRow(ctx, `
		INSERT INTO reminders (user_id, title, body, due_at, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+reminderSelect,
		p.UserID, p.Title, p.Body, p.DueAt.UTC(), p.Status,
	)
	var rm models.Reminder
	if err := scanReminder(row, &rm); err != nil {
		return nil, err
	}
	return &rm, nil
}

func (r *ReminderRepository) GetByID(ctx context.Context, id, userID string) (*models.Reminder, error) {
	row := r.DB.QueryRow(ctx, `
		SELECT `+reminderSelect+`
		FROM reminders
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	var rm models.Reminder
	if err := scanReminder(row, &rm); err != nil {
		return nil, err
	}
	return &rm, nil
}

func (r *ReminderRepository) List(ctx context.Context, userID, status string) ([]models.Reminder, error) {
	where := "user_id = $1"
	args := []any{userID}
	if status != "" {
		where += " AND status = $2"
		args = append(args, status)
	}
	rows, err := r.DB.Query(ctx, `
		SELECT `+reminderSelect+`
		FROM reminders
		WHERE `+where+`
		ORDER BY
			CASE status WHEN 'pending' THEN 0 WHEN 'done' THEN 1 ELSE 2 END,
			due_at ASC,
			created_at ASC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.Reminder, 0)
	for rows.Next() {
		var rm models.Reminder
		if err := scanReminder(rows, &rm); err != nil {
			return nil, err
		}
		out = append(out, rm)
	}
	return out, rows.Err()
}

// ListDue returns pending reminders with due_at <= now that have not been notified yet.
func (r *ReminderRepository) ListDue(ctx context.Context, userID string, now time.Time) ([]models.Reminder, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT `+reminderSelect+`
		FROM reminders
		WHERE user_id = $1 AND status = 'pending' AND due_at <= $2 AND notified_at IS NULL
		ORDER BY due_at ASC
	`, userID, now.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.Reminder, 0)
	for rows.Next() {
		var rm models.Reminder
		if err := scanReminder(rows, &rm); err != nil {
			return nil, err
		}
		out = append(out, rm)
	}
	return out, rows.Err()
}

// MarkNotified sets notified_at only if it is still NULL. Returns the updated row,
// or pgx.ErrNoRows if it was already notified (or not found).
func (r *ReminderRepository) MarkNotified(ctx context.Context, id, userID string, at time.Time) (*models.Reminder, error) {
	row := r.DB.QueryRow(ctx, `
		UPDATE reminders
		SET notified_at = $3, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND user_id = $2 AND notified_at IS NULL
		RETURNING `+reminderSelect, id, userID, at.UTC())
	var rm models.Reminder
	if err := scanReminder(row, &rm); err != nil {
		return nil, err
	}
	return &rm, nil
}

func (r *ReminderRepository) Update(ctx context.Context, id, userID string, p ReminderUpdateParams) (*models.Reminder, error) {
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
	if p.Body != nil {
		add("body", *p.Body)
	}
	if p.DueAt != nil {
		add("due_at", p.DueAt.UTC())
	}
	if p.Status != nil {
		add("status", *p.Status)
	}
	if p.ResetNotified {
		sets = append(sets, "notified_at = NULL")
	}
	if len(sets) == 0 {
		return r.GetByID(ctx, id, userID)
	}
	sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id, userID)

	row := r.DB.QueryRow(ctx, `
		UPDATE reminders
		SET `+strings.Join(sets, ", ")+`
		WHERE id = $`+fmt.Sprintf("%d", argN)+` AND user_id = $`+fmt.Sprintf("%d", argN+1)+`
		RETURNING `+reminderSelect, args...)
	var rm models.Reminder
	if err := scanReminder(row, &rm); err != nil {
		return nil, err
	}
	return &rm, nil
}

func (r *ReminderRepository) Delete(ctx context.Context, id, userID string) error {
	tag, err := r.DB.Exec(ctx, `DELETE FROM reminders WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
