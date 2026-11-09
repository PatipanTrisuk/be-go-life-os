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

type PlannerRepository struct {
	DB *pgxpool.Pool
}

func NewPlannerRepository(db *pgxpool.Pool) *PlannerRepository {
	return &PlannerRepository{DB: db}
}

type PlannerCreateParams struct {
	UserID    string
	PlanDate  time.Time
	Title     string
	Notes     string
	Status    string
	Priority  string
	StartTime string
	EndTime   string
	SortOrder int
}

type PlannerUpdateParams struct {
	Title       *string
	Notes       *string
	Status      *string
	Priority    *string
	StartTime   *string
	EndTime     *string
	SortOrder   *int
	CompletedAt **time.Time
}

const plannerSelect = `
	id, user_id, plan_date, title, notes, status, priority,
	start_time, end_time, sort_order, completed_at, created_at, updated_at
`

func scanPlannerTask(scanner interface {
	Scan(dest ...any) error
}, t *models.PlannerTask) error {
	var completedAt sql.NullTime
	if err := scanner.Scan(
		&t.ID,
		&t.UserID,
		&t.PlanDate,
		&t.Title,
		&t.Notes,
		&t.Status,
		&t.Priority,
		&t.StartTime,
		&t.EndTime,
		&t.SortOrder,
		&completedAt,
		&t.CreatedAt,
		&t.UpdatedAt,
	); err != nil {
		return err
	}
	if completedAt.Valid {
		v := completedAt.Time
		t.CompletedAt = &v
	}
	return nil
}

func (r *PlannerRepository) Create(ctx context.Context, p PlannerCreateParams) (*models.PlannerTask, error) {
	row := r.DB.QueryRow(ctx, `
		INSERT INTO planner_tasks (
			user_id, plan_date, title, notes, status, priority, start_time, end_time, sort_order
		) VALUES ($1, $2::date, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+plannerSelect,
		p.UserID, p.PlanDate.Format("2006-01-02"), p.Title, p.Notes, p.Status, p.Priority, p.StartTime, p.EndTime, p.SortOrder,
	)
	var t models.PlannerTask
	if err := scanPlannerTask(row, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *PlannerRepository) GetByID(ctx context.Context, id, userID string) (*models.PlannerTask, error) {
	row := r.DB.QueryRow(ctx, `
		SELECT `+plannerSelect+`
		FROM planner_tasks
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	var t models.PlannerTask
	if err := scanPlannerTask(row, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *PlannerRepository) ListByDate(ctx context.Context, userID string, date time.Time) ([]models.PlannerTask, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT `+plannerSelect+`
		FROM planner_tasks
		WHERE user_id = $1 AND plan_date = $2::date
		ORDER BY
			CASE status WHEN 'todo' THEN 0 WHEN 'done' THEN 1 ELSE 2 END,
			sort_order ASC,
			CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END,
			created_at ASC
	`, userID, date.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.PlannerTask, 0)
	for rows.Next() {
		var t models.PlannerTask
		if err := scanPlannerTask(rows, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *PlannerRepository) NextSortOrder(ctx context.Context, userID string, date time.Time) (int, error) {
	var n int
	err := r.DB.QueryRow(ctx, `
		SELECT COALESCE(MAX(sort_order), -1) + 1
		FROM planner_tasks
		WHERE user_id = $1 AND plan_date = $2::date
	`, userID, date.Format("2006-01-02")).Scan(&n)
	return n, err
}

func (r *PlannerRepository) Update(ctx context.Context, id, userID string, p PlannerUpdateParams) (*models.PlannerTask, error) {
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
	if p.Notes != nil {
		add("notes", *p.Notes)
	}
	if p.Status != nil {
		add("status", *p.Status)
	}
	if p.Priority != nil {
		add("priority", *p.Priority)
	}
	if p.StartTime != nil {
		add("start_time", *p.StartTime)
	}
	if p.EndTime != nil {
		add("end_time", *p.EndTime)
	}
	if p.SortOrder != nil {
		add("sort_order", *p.SortOrder)
	}
	if p.CompletedAt != nil {
		if *p.CompletedAt == nil {
			sets = append(sets, "completed_at = NULL")
		} else {
			add("completed_at", **p.CompletedAt)
		}
	}
	if len(sets) == 0 {
		return r.GetByID(ctx, id, userID)
	}
	sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id, userID)

	row := r.DB.QueryRow(ctx, `
		UPDATE planner_tasks
		SET `+strings.Join(sets, ", ")+`
		WHERE id = $`+fmt.Sprintf("%d", argN)+` AND user_id = $`+fmt.Sprintf("%d", argN+1)+`
		RETURNING `+plannerSelect, args...)
	var t models.PlannerTask
	if err := scanPlannerTask(row, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *PlannerRepository) Delete(ctx context.Context, id, userID string) error {
	tag, err := r.DB.Exec(ctx, `DELETE FROM planner_tasks WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *PlannerRepository) DayStats(ctx context.Context, userID string, date time.Time) (*models.PlannerDayStats, error) {
	s := &models.PlannerDayStats{Date: date.Format("2006-01-02")}
	err := r.DB.QueryRow(ctx, `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE status = 'todo')::int,
			COUNT(*) FILTER (WHERE status = 'done')::int,
			COUNT(*) FILTER (WHERE status = 'skipped')::int,
			COUNT(*) FILTER (WHERE priority = 'high' AND status = 'todo')::int
		FROM planner_tasks
		WHERE user_id = $1 AND plan_date = $2::date
	`, userID, date.Format("2006-01-02")).Scan(&s.Total, &s.Todo, &s.Done, &s.Skipped, &s.High)
	if err != nil {
		return nil, err
	}
	actionable := s.Todo + s.Done
	if actionable > 0 {
		s.Completion = int(float64(s.Done) / float64(actionable) * 100)
	}
	return s, nil
}

func (r *PlannerRepository) DailyDoneTrend(ctx context.Context, userID string, days int) ([]models.DayCount, error) {
	if days < 1 {
		days = 14
	}
	rows, err := r.DB.Query(ctx, `
		WITH days AS (
			SELECT TO_CHAR(d::date, 'YYYY-MM-DD') AS day
			FROM generate_series(
				CURRENT_DATE - (($1::int - 1) || ' days')::interval,
				CURRENT_DATE,
				'1 day'::interval
			) AS d
		),
		done AS (
			SELECT TO_CHAR(plan_date, 'YYYY-MM-DD') AS day, COUNT(*)::int AS count
			FROM planner_tasks
			WHERE user_id = $2 AND status = 'done'
			  AND plan_date >= CURRENT_DATE - (($1::int - 1) || ' days')::interval
			GROUP BY 1
		)
		SELECT d.day, COALESCE(x.count, 0)::int
		FROM days d
		LEFT JOIN done x ON x.day = d.day
		ORDER BY d.day ASC
	`, days, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.DayCount, 0)
	for rows.Next() {
		var item models.DayCount
		if err := rows.Scan(&item.Date, &item.Count); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PlannerRepository) WeeklyActivity(ctx context.Context, userID string, days int) ([]models.LifeDayActivity, error) {
	if days < 1 {
		days = 7
	}
	rows, err := r.DB.Query(ctx, `
		WITH days AS (
			SELECT d::date AS day, TO_CHAR(d::date, 'YYYY-MM-DD') AS day_key
			FROM generate_series(
				CURRENT_DATE - (($1::int - 1) || ' days')::interval,
				CURRENT_DATE,
				'1 day'::interval
			) AS d
		)
		SELECT
			d.day_key,
			COUNT(t.id) FILTER (WHERE t.status = 'done')::int AS done,
			COUNT(t.id)::int AS total
		FROM days d
		LEFT JOIN planner_tasks t
			ON t.user_id = $2 AND t.plan_date = d.day
		GROUP BY d.day_key
		ORDER BY d.day_key ASC
	`, days, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.LifeDayActivity, 0)
	for rows.Next() {
		var item models.LifeDayActivity
		if err := rows.Scan(&item.Date, &item.PlannerDone, &item.PlannerTotal); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
