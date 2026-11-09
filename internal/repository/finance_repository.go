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

type FinanceRepository struct {
	DB *pgxpool.Pool
}

func NewFinanceRepository(db *pgxpool.Pool) *FinanceRepository {
	return &FinanceRepository{DB: db}
}

type FinanceListFilter struct {
	Kind     string
	Category string
	From     *time.Time
	To       *time.Time
	Page     int
	Limit    int
}

type FinanceCreateParams struct {
	UserID     string
	Kind       string
	Amount     float64
	Currency   string
	Category   string
	Note       string
	OccurredOn time.Time
}

type FinanceUpdateParams struct {
	Kind       *string
	Amount     *float64
	Currency   *string
	Category   *string
	Note       *string
	OccurredOn *time.Time
}

const financeSelect = `
	id, user_id, kind, amount, currency, category, note, occurred_on, created_at, updated_at
`

func scanFinance(scanner interface {
	Scan(dest ...any) error
}, t *models.FinanceTransaction) error {
	return scanner.Scan(
		&t.ID,
		&t.UserID,
		&t.Kind,
		&t.Amount,
		&t.Currency,
		&t.Category,
		&t.Note,
		&t.OccurredOn,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
}

func (r *FinanceRepository) Create(ctx context.Context, p FinanceCreateParams) (*models.FinanceTransaction, error) {
	row := r.DB.QueryRow(ctx, `
		INSERT INTO finance_transactions (user_id, kind, amount, currency, category, note, occurred_on)
		VALUES ($1, $2, $3, $4, $5, $6, $7::date)
		RETURNING `+financeSelect,
		p.UserID, p.Kind, p.Amount, p.Currency, p.Category, p.Note, p.OccurredOn.Format("2006-01-02"),
	)
	var t models.FinanceTransaction
	if err := scanFinance(row, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *FinanceRepository) GetByID(ctx context.Context, id, userID string) (*models.FinanceTransaction, error) {
	row := r.DB.QueryRow(ctx, `
		SELECT `+financeSelect+`
		FROM finance_transactions
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	var t models.FinanceTransaction
	if err := scanFinance(row, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *FinanceRepository) List(ctx context.Context, userID string, f FinanceListFilter) ([]models.FinanceTransaction, int, error) {
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
	if f.Category != "" {
		where = append(where, fmt.Sprintf("LOWER(category) = LOWER($%d)", argN))
		args = append(args, f.Category)
		argN++
	}
	if f.From != nil {
		where = append(where, fmt.Sprintf("occurred_on >= $%d::date", argN))
		args = append(args, f.From.Format("2006-01-02"))
		argN++
	}
	if f.To != nil {
		where = append(where, fmt.Sprintf("occurred_on <= $%d::date", argN))
		args = append(args, f.To.Format("2006-01-02"))
		argN++
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := r.DB.QueryRow(ctx, `SELECT COUNT(*) FROM finance_transactions WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (f.Page - 1) * f.Limit
	args = append(args, f.Limit, offset)
	rows, err := r.DB.Query(ctx, `
		SELECT `+financeSelect+`
		FROM finance_transactions
		WHERE `+whereSQL+`
		ORDER BY occurred_on DESC, created_at DESC
		LIMIT $`+fmt.Sprintf("%d", argN)+` OFFSET $`+fmt.Sprintf("%d", argN+1),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]models.FinanceTransaction, 0)
	for rows.Next() {
		var t models.FinanceTransaction
		if err := scanFinance(rows, &t); err != nil {
			return nil, 0, err
		}
		items = append(items, t)
	}
	return items, total, rows.Err()
}

func (r *FinanceRepository) Stats(ctx context.Context, userID string, from, to *time.Time) (*models.FinanceStats, error) {
	where := []string{"user_id = $1"}
	args := []any{userID}
	argN := 2
	if from != nil {
		where = append(where, fmt.Sprintf("occurred_on >= $%d::date", argN))
		args = append(args, from.Format("2006-01-02"))
		argN++
	}
	if to != nil {
		where = append(where, fmt.Sprintf("occurred_on <= $%d::date", argN))
		args = append(args, to.Format("2006-01-02"))
		argN++
	}

	s := &models.FinanceStats{}
	err := r.DB.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(amount) FILTER (WHERE kind = 'income'), 0)::float8,
			COALESCE(SUM(amount) FILTER (WHERE kind = 'expense'), 0)::float8,
			COUNT(*)::int
		FROM finance_transactions
		WHERE `+strings.Join(where, " AND "), args...).Scan(&s.Income, &s.Expense, &s.Count)
	if err != nil {
		return nil, err
	}
	s.Net = s.Income - s.Expense
	return s, nil
}

func (r *FinanceRepository) Update(ctx context.Context, id, userID string, p FinanceUpdateParams) (*models.FinanceTransaction, error) {
	sets := make([]string, 0)
	args := make([]any, 0)
	argN := 1
	add := func(expr string, val any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", expr, argN))
		args = append(args, val)
		argN++
	}

	if p.Kind != nil {
		add("kind", *p.Kind)
	}
	if p.Amount != nil {
		add("amount", *p.Amount)
	}
	if p.Currency != nil {
		add("currency", *p.Currency)
	}
	if p.Category != nil {
		add("category", *p.Category)
	}
	if p.Note != nil {
		add("note", *p.Note)
	}
	if p.OccurredOn != nil {
		add("occurred_on", p.OccurredOn.Format("2006-01-02"))
		sets[len(sets)-1] = fmt.Sprintf("occurred_on = $%d::date", argN-1)
	}
	if len(sets) == 0 {
		return r.GetByID(ctx, id, userID)
	}
	sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id, userID)

	row := r.DB.QueryRow(ctx, `
		UPDATE finance_transactions
		SET `+strings.Join(sets, ", ")+`
		WHERE id = $`+fmt.Sprintf("%d", argN)+` AND user_id = $`+fmt.Sprintf("%d", argN+1)+`
		RETURNING `+financeSelect, args...)
	var t models.FinanceTransaction
	if err := scanFinance(row, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *FinanceRepository) Delete(ctx context.Context, id, userID string) error {
	tag, err := r.DB.Exec(ctx, `DELETE FROM finance_transactions WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
