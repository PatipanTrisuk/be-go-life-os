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

type GoalRepository struct {
	DB *pgxpool.Pool
}

func NewGoalRepository(db *pgxpool.Pool) *GoalRepository {
	return &GoalRepository{DB: db}
}

type GoalListFilter struct {
	Status   string
	Priority string
	Category string
	Search   string
	Page     int
	Limit    int
}

type GoalCreateParams struct {
	UserID       string
	Title        string
	Description  string
	Status       string
	Priority     string
	Category     string
	Progress     int
	TargetValue  *float64
	CurrentValue float64
	Unit         string
	StartDate    *time.Time
	TargetDate   *time.Time
}

type GoalUpdateParams struct {
	Title        *string
	Description  *string
	Status       *string
	Priority     *string
	Category     *string
	Progress     *int
	TargetValue  **float64 // nil = omit; *nil = clear; *value = set
	CurrentValue *float64
	Unit         *string
	StartDate    **time.Time
	TargetDate   **time.Time
	CompletedAt  **time.Time
	ArchivedAt   **time.Time
}

const goalSelect = `
	id, user_id, title, description, status, priority, category,
	progress, target_value, current_value, unit,
	start_date, target_date, completed_at, archived_at,
	created_at, updated_at
`

const milestoneSelect = `
	id, goal_id, user_id, title, is_completed, sort_order,
	completed_at, created_at, updated_at
`

func scanGoal(scanner interface {
	Scan(dest ...any) error
}, g *models.Goal) error {
	var (
		targetValue             sql.NullFloat64
		startDate, targetDate   sql.NullTime
		completedAt, archivedAt sql.NullTime
	)
	if err := scanner.Scan(
		&g.ID,
		&g.UserID,
		&g.Title,
		&g.Description,
		&g.Status,
		&g.Priority,
		&g.Category,
		&g.Progress,
		&targetValue,
		&g.CurrentValue,
		&g.Unit,
		&startDate,
		&targetDate,
		&completedAt,
		&archivedAt,
		&g.CreatedAt,
		&g.UpdatedAt,
	); err != nil {
		return err
	}
	if targetValue.Valid {
		v := targetValue.Float64
		g.TargetValue = &v
	}
	if startDate.Valid {
		t := startDate.Time
		g.StartDate = &t
	}
	if targetDate.Valid {
		t := targetDate.Time
		g.TargetDate = &t
	}
	if completedAt.Valid {
		t := completedAt.Time
		g.CompletedAt = &t
	}
	if archivedAt.Valid {
		t := archivedAt.Time
		g.ArchivedAt = &t
	}
	return nil
}

func scanMilestone(scanner interface {
	Scan(dest ...any) error
}, m *models.GoalMilestone) error {
	var completedAt sql.NullTime
	if err := scanner.Scan(
		&m.ID,
		&m.GoalID,
		&m.UserID,
		&m.Title,
		&m.IsCompleted,
		&m.SortOrder,
		&completedAt,
		&m.CreatedAt,
		&m.UpdatedAt,
	); err != nil {
		return err
	}
	if completedAt.Valid {
		t := completedAt.Time
		m.CompletedAt = &t
	}
	return nil
}

func (r *GoalRepository) Create(ctx context.Context, p GoalCreateParams) (*models.Goal, error) {
	g := &models.Goal{}
	err := scanGoal(r.DB.QueryRow(ctx, `
		INSERT INTO goals (
			user_id, title, description, status, priority, category,
			progress, target_value, current_value, unit, start_date, target_date
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING `+goalSelect,
		p.UserID, p.Title, p.Description, p.Status, p.Priority, p.Category,
		p.Progress, p.TargetValue, p.CurrentValue, p.Unit, p.StartDate, p.TargetDate,
	), g)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (r *GoalRepository) GetByID(ctx context.Context, id, userID string) (*models.Goal, error) {
	g := &models.Goal{}
	err := scanGoal(r.DB.QueryRow(ctx, `
		SELECT `+goalSelect+`
		FROM goals
		WHERE id = $1 AND user_id = $2
	`, id, userID), g)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (r *GoalRepository) List(ctx context.Context, userID string, f GoalListFilter) ([]models.Goal, int, error) {
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

	if f.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argN))
		args = append(args, f.Status)
		argN++
	}
	if f.Priority != "" {
		where = append(where, fmt.Sprintf("priority = $%d", argN))
		args = append(args, f.Priority)
		argN++
	}
	if f.Category != "" {
		where = append(where, fmt.Sprintf("LOWER(category) = LOWER($%d)", argN))
		args = append(args, f.Category)
		argN++
	}
	if f.Search != "" {
		where = append(where, fmt.Sprintf("(title ILIKE $%d OR description ILIKE $%d)", argN, argN))
		args = append(args, "%"+f.Search+"%")
		argN++
	}

	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := r.DB.QueryRow(ctx, `SELECT COUNT(*) FROM goals WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (f.Page - 1) * f.Limit
	args = append(args, f.Limit, offset)
	rows, err := r.DB.Query(ctx, `
		SELECT `+goalSelect+`
		FROM goals
		WHERE `+whereSQL+`
		ORDER BY
			CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END,
			target_date NULLS LAST,
			created_at DESC
		LIMIT $`+fmt.Sprintf("%d", argN)+` OFFSET $`+fmt.Sprintf("%d", argN+1),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]models.Goal, 0)
	for rows.Next() {
		var g models.Goal
		if err := scanGoal(rows, &g); err != nil {
			return nil, 0, err
		}
		items = append(items, g)
	}
	return items, total, rows.Err()
}

func (r *GoalRepository) Update(ctx context.Context, id, userID string, p GoalUpdateParams) (*models.Goal, error) {
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
	if p.Status != nil {
		add("status", *p.Status)
	}
	if p.Priority != nil {
		add("priority", *p.Priority)
	}
	if p.Category != nil {
		add("category", *p.Category)
	}
	if p.Progress != nil {
		add("progress", *p.Progress)
	}
	if p.TargetValue != nil {
		add("target_value", *p.TargetValue)
	}
	if p.CurrentValue != nil {
		add("current_value", *p.CurrentValue)
	}
	if p.Unit != nil {
		add("unit", *p.Unit)
	}
	if p.StartDate != nil {
		add("start_date", *p.StartDate)
	}
	if p.TargetDate != nil {
		add("target_date", *p.TargetDate)
	}
	if p.CompletedAt != nil {
		add("completed_at", *p.CompletedAt)
	}
	if p.ArchivedAt != nil {
		add("archived_at", *p.ArchivedAt)
	}

	if len(sets) == 0 {
		return r.GetByID(ctx, id, userID)
	}

	sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id, userID)

	q := fmt.Sprintf(`
		UPDATE goals
		SET %s
		WHERE id = $%d AND user_id = $%d
		RETURNING %s
	`, strings.Join(sets, ", "), argN, argN+1, goalSelect)

	g := &models.Goal{}
	if err := scanGoal(r.DB.QueryRow(ctx, q, args...), g); err != nil {
		return nil, err
	}
	return g, nil
}

func (r *GoalRepository) Delete(ctx context.Context, id, userID string) error {
	tag, err := r.DB.Exec(ctx, `DELETE FROM goals WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *GoalRepository) Stats(ctx context.Context, userID string) (*models.GoalStats, error) {
	s := &models.GoalStats{}
	err := r.DB.QueryRow(ctx, `
		SELECT
			COUNT(*)::int AS total,
			COUNT(*) FILTER (WHERE status = 'draft')::int AS draft,
			COUNT(*) FILTER (WHERE status = 'active')::int AS active,
			COUNT(*) FILTER (WHERE status = 'completed')::int AS completed,
			COUNT(*) FILTER (WHERE status = 'cancelled')::int AS cancelled,
			COUNT(*) FILTER (WHERE status = 'archived')::int AS archived,
			COUNT(*) FILTER (
				WHERE status = 'active'
				  AND target_date IS NOT NULL
				  AND target_date < CURRENT_DATE
			)::int AS overdue,
			COALESCE(ROUND(AVG(progress) FILTER (WHERE status IN ('draft','active'))), 0)::int AS avg_progress
		FROM goals
		WHERE user_id = $1
	`, userID).Scan(
		&s.Total, &s.Draft, &s.Active, &s.Completed, &s.Cancelled, &s.Archived, &s.Overdue, &s.AvgProgress,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *GoalRepository) CountsByStatus(ctx context.Context, userID string) ([]models.NamedCount, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT status, COUNT(*)::int
		FROM goals
		WHERE user_id = $1
		GROUP BY status
		ORDER BY
			CASE status
				WHEN 'active' THEN 0
				WHEN 'draft' THEN 1
				WHEN 'completed' THEN 2
				WHEN 'cancelled' THEN 3
				ELSE 4
			END
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.NamedCount, 0)
	for rows.Next() {
		var item models.NamedCount
		if err := rows.Scan(&item.Key, &item.Count); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *GoalRepository) CountsByPriority(ctx context.Context, userID string) ([]models.NamedCount, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT priority, COUNT(*)::int
		FROM goals
		WHERE user_id = $1
		GROUP BY priority
		ORDER BY
			CASE priority
				WHEN 'high' THEN 0
				WHEN 'medium' THEN 1
				ELSE 2
			END
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.NamedCount, 0)
	for rows.Next() {
		var item models.NamedCount
		if err := rows.Scan(&item.Key, &item.Count); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *GoalRepository) CountsByCategory(ctx context.Context, userID string, limit int) ([]models.NamedCount, error) {
	if limit < 1 {
		limit = 6
	}
	rows, err := r.DB.Query(ctx, `
		SELECT
			CASE WHEN TRIM(category) = '' THEN 'uncategorized' ELSE category END AS category,
			COUNT(*)::int
		FROM goals
		WHERE user_id = $1
		GROUP BY 1
		ORDER BY COUNT(*) DESC, category ASC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.NamedCount, 0)
	for rows.Next() {
		var item models.NamedCount
		if err := rows.Scan(&item.Key, &item.Count); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *GoalRepository) MonthlyTrend(ctx context.Context, userID string, months int) ([]models.MonthCount, error) {
	if months < 1 {
		months = 6
	}
	rows, err := r.DB.Query(ctx, `
		WITH months AS (
			SELECT TO_CHAR(d, 'YYYY-MM') AS month
			FROM generate_series(
				date_trunc('month', CURRENT_DATE) - (($1::int - 1) || ' months')::interval,
				date_trunc('month', CURRENT_DATE),
				'1 month'::interval
			) AS d
		),
		created AS (
			SELECT TO_CHAR(date_trunc('month', created_at), 'YYYY-MM') AS month, COUNT(*)::int AS created
			FROM goals
			WHERE user_id = $2
			GROUP BY 1
		),
		completed AS (
			SELECT TO_CHAR(date_trunc('month', completed_at), 'YYYY-MM') AS month, COUNT(*)::int AS completed
			FROM goals
			WHERE user_id = $2 AND completed_at IS NOT NULL
			GROUP BY 1
		)
		SELECT
			m.month,
			COALESCE(c.created, 0)::int AS created,
			COALESCE(d.completed, 0)::int AS completed
		FROM months m
		LEFT JOIN created c ON c.month = m.month
		LEFT JOIN completed d ON d.month = m.month
		ORDER BY m.month ASC
	`, months, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.MonthCount, 0)
	for rows.Next() {
		var item models.MonthCount
		if err := rows.Scan(&item.Month, &item.Created, &item.Completed); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *GoalRepository) ProgressBuckets(ctx context.Context, userID string) ([]models.ProgressBucket, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT bucket, COUNT(*)::int
		FROM (
			SELECT
				CASE
					WHEN progress <= 20 THEN '0-20'
					WHEN progress <= 40 THEN '21-40'
					WHEN progress <= 60 THEN '41-60'
					WHEN progress <= 80 THEN '61-80'
					ELSE '81-100'
				END AS bucket,
				CASE
					WHEN progress <= 20 THEN 0
					WHEN progress <= 40 THEN 1
					WHEN progress <= 60 THEN 2
					WHEN progress <= 80 THEN 3
					ELSE 4
				END AS sort_key
			FROM goals
			WHERE user_id = $1 AND status IN ('draft', 'active', 'completed')
		) t
		GROUP BY bucket, sort_key
		ORDER BY sort_key
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.ProgressBucket, 0)
	for rows.Next() {
		var item models.ProgressBucket
		if err := rows.Scan(&item.Bucket, &item.Count); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *GoalRepository) ListMilestones(ctx context.Context, goalID, userID string) ([]models.GoalMilestone, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT `+milestoneSelect+`
		FROM goal_milestones
		WHERE goal_id = $1 AND user_id = $2
		ORDER BY sort_order ASC, created_at ASC
	`, goalID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]models.GoalMilestone, 0)
	for rows.Next() {
		var m models.GoalMilestone
		if err := scanMilestone(rows, &m); err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

func (r *GoalRepository) CreateMilestone(
	ctx context.Context,
	goalID, userID, title string,
	sortOrder int,
) (*models.GoalMilestone, error) {
	m := &models.GoalMilestone{}
	err := scanMilestone(r.DB.QueryRow(ctx, `
		INSERT INTO goal_milestones (goal_id, user_id, title, sort_order)
		VALUES ($1, $2, $3, $4)
		RETURNING `+milestoneSelect,
		goalID, userID, title, sortOrder,
	), m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (r *GoalRepository) GetMilestone(ctx context.Context, id, goalID, userID string) (*models.GoalMilestone, error) {
	m := &models.GoalMilestone{}
	err := scanMilestone(r.DB.QueryRow(ctx, `
		SELECT `+milestoneSelect+`
		FROM goal_milestones
		WHERE id = $1 AND goal_id = $2 AND user_id = $3
	`, id, goalID, userID), m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (r *GoalRepository) UpdateMilestone(
	ctx context.Context,
	id, goalID, userID string,
	title *string,
	isCompleted *bool,
	sortOrder *int,
) (*models.GoalMilestone, error) {
	sets := make([]string, 0)
	args := make([]any, 0)
	argN := 1

	if title != nil {
		sets = append(sets, fmt.Sprintf("title = $%d", argN))
		args = append(args, *title)
		argN++
	}
	if isCompleted != nil {
		sets = append(sets, fmt.Sprintf("is_completed = $%d", argN))
		args = append(args, *isCompleted)
		argN++
		if *isCompleted {
			sets = append(sets, "completed_at = CURRENT_TIMESTAMP")
		} else {
			sets = append(sets, "completed_at = NULL")
		}
	}
	if sortOrder != nil {
		sets = append(sets, fmt.Sprintf("sort_order = $%d", argN))
		args = append(args, *sortOrder)
		argN++
	}

	if len(sets) == 0 {
		return r.GetMilestone(ctx, id, goalID, userID)
	}

	sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id, goalID, userID)

	q := fmt.Sprintf(`
		UPDATE goal_milestones
		SET %s
		WHERE id = $%d AND goal_id = $%d AND user_id = $%d
		RETURNING %s
	`, strings.Join(sets, ", "), argN, argN+1, argN+2, milestoneSelect)

	m := &models.GoalMilestone{}
	if err := scanMilestone(r.DB.QueryRow(ctx, q, args...), m); err != nil {
		return nil, err
	}
	return m, nil
}

func (r *GoalRepository) DeleteMilestone(ctx context.Context, id, goalID, userID string) error {
	tag, err := r.DB.Exec(ctx, `
		DELETE FROM goal_milestones
		WHERE id = $1 AND goal_id = $2 AND user_id = $3
	`, id, goalID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *GoalRepository) NextMilestoneSortOrder(ctx context.Context, goalID, userID string) (int, error) {
	var next int
	err := r.DB.QueryRow(ctx, `
		SELECT COALESCE(MAX(sort_order), -1) + 1
		FROM goal_milestones
		WHERE goal_id = $1 AND user_id = $2
	`, goalID, userID).Scan(&next)
	return next, err
}

func (r *GoalRepository) MilestoneProgress(ctx context.Context, goalID, userID string) (completed, total int, err error) {
	err = r.DB.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE is_completed)::int,
			COUNT(*)::int
		FROM goal_milestones
		WHERE goal_id = $1 AND user_id = $2
	`, goalID, userID).Scan(&completed, &total)
	return
}
