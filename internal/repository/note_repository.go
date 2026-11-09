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

type NoteRepository struct {
	DB *pgxpool.Pool
}

func NewNoteRepository(db *pgxpool.Pool) *NoteRepository {
	return &NoteRepository{DB: db}
}

type NoteListFilter struct {
	Kind   string
	Search string
	Page   int
	Limit  int
}

type NoteCreateParams struct {
	UserID    string
	Title     string
	Body      string
	Kind      string
	Mood      string
	EntryDate time.Time
	Tags      string
}

type NoteUpdateParams struct {
	Title     *string
	Body      *string
	Kind      *string
	Mood      *string
	EntryDate *time.Time
	Tags      *string
}

const noteSelect = `
	id, user_id, title, body, kind, mood, entry_date, tags, created_at, updated_at
`

func scanNote(scanner interface {
	Scan(dest ...any) error
}, n *models.Note) error {
	return scanner.Scan(
		&n.ID,
		&n.UserID,
		&n.Title,
		&n.Body,
		&n.Kind,
		&n.Mood,
		&n.EntryDate,
		&n.Tags,
		&n.CreatedAt,
		&n.UpdatedAt,
	)
}

func (r *NoteRepository) Create(ctx context.Context, p NoteCreateParams) (*models.Note, error) {
	row := r.DB.QueryRow(ctx, `
		INSERT INTO notes (user_id, title, body, kind, mood, entry_date, tags)
		VALUES ($1, $2, $3, $4, $5, $6::date, $7)
		RETURNING `+noteSelect,
		p.UserID, p.Title, p.Body, p.Kind, p.Mood, p.EntryDate.Format("2006-01-02"), p.Tags,
	)
	var n models.Note
	if err := scanNote(row, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *NoteRepository) GetByID(ctx context.Context, id, userID string) (*models.Note, error) {
	row := r.DB.QueryRow(ctx, `
		SELECT `+noteSelect+`
		FROM notes
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	var n models.Note
	if err := scanNote(row, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *NoteRepository) List(ctx context.Context, userID string, f NoteListFilter) ([]models.Note, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 {
		f.Limit = 20
	}
	if f.Limit > 100 {
		f.Limit = 100
	}

	where := []string{"user_id = $1"}
	args := []any{userID}
	argN := 2

	if f.Kind != "" {
		where = append(where, fmt.Sprintf("kind = $%d", argN))
		args = append(args, f.Kind)
		argN++
	}
	if f.Search != "" {
		where = append(where, fmt.Sprintf("(title ILIKE $%d OR body ILIKE $%d)", argN, argN))
		args = append(args, "%"+f.Search+"%")
		argN++
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := r.DB.QueryRow(ctx, `SELECT COUNT(*) FROM notes WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (f.Page - 1) * f.Limit
	args = append(args, f.Limit, offset)
	rows, err := r.DB.Query(ctx, `
		SELECT `+noteSelect+`
		FROM notes
		WHERE `+whereSQL+`
		ORDER BY entry_date DESC, created_at DESC
		LIMIT $`+fmt.Sprintf("%d", argN)+` OFFSET $`+fmt.Sprintf("%d", argN+1),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]models.Note, 0)
	for rows.Next() {
		var n models.Note
		if err := scanNote(rows, &n); err != nil {
			return nil, 0, err
		}
		items = append(items, n)
	}
	return items, total, rows.Err()
}

func (r *NoteRepository) Stats(ctx context.Context, userID string) (*models.NoteStats, error) {
	s := &models.NoteStats{}
	err := r.DB.QueryRow(ctx, `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE kind = 'note')::int,
			COUNT(*) FILTER (WHERE kind = 'journal')::int
		FROM notes
		WHERE user_id = $1
	`, userID).Scan(&s.Total, &s.Notes, &s.Journal)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *NoteRepository) Update(ctx context.Context, id, userID string, p NoteUpdateParams) (*models.Note, error) {
	sets := make([]string, 0)
	args := make([]any, 0)
	argN := 1
	add := func(expr string, val any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", expr, argN))
		args = append(args, val)
		argN++
	}

	if p.Title != nil {
		add("title", *p.Title)
	}
	if p.Body != nil {
		add("body", *p.Body)
	}
	if p.Kind != nil {
		add("kind", *p.Kind)
	}
	if p.Mood != nil {
		add("mood", *p.Mood)
	}
	if p.EntryDate != nil {
		add("entry_date", p.EntryDate.Format("2006-01-02"))
		sets[len(sets)-1] = fmt.Sprintf("entry_date = $%d::date", argN-1)
	}
	if p.Tags != nil {
		add("tags", *p.Tags)
	}
	if len(sets) == 0 {
		return r.GetByID(ctx, id, userID)
	}
	sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id, userID)

	row := r.DB.QueryRow(ctx, `
		UPDATE notes
		SET `+strings.Join(sets, ", ")+`
		WHERE id = $`+fmt.Sprintf("%d", argN)+` AND user_id = $`+fmt.Sprintf("%d", argN+1)+`
		RETURNING `+noteSelect, args...)
	var n models.Note
	if err := scanNote(row, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *NoteRepository) Delete(ctx context.Context, id, userID string) error {
	tag, err := r.DB.Exec(ctx, `DELETE FROM notes WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
