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

type HabitRepository struct {
	DB *pgxpool.Pool
}

func NewHabitRepository(db *pgxpool.Pool) *HabitRepository {
	return &HabitRepository{DB: db}
}

type HabitListFilter struct {
	Status   string
	Category string
	Search   string
	Page     int
	Limit    int
}

type HabitCreateParams struct {
	UserID      string
	Title       string
	Description string
	Status      string
	Frequency   string
	TargetCount int
	Category    string
	Color       string
}

type HabitUpdateParams struct {
	Title       *string
	Description *string
	Status      *string
	Frequency   *string
	TargetCount *int
	Category    *string
	Color       *string
	ArchivedAt  **time.Time
}

const habitSelect = `
	id, user_id, title, description, status, frequency, target_count,
	category, color, current_streak, best_streak, archived_at,
	created_at, updated_at
`

const habitLogSelect = `
	id, habit_id, user_id, log_date, count, note, created_at, updated_at
`

func scanHabit(scanner interface {
	Scan(dest ...any) error
}, h *models.Habit) error {
	var archivedAt sql.NullTime
	if err := scanner.Scan(
		&h.ID,
		&h.UserID,
		&h.Title,
		&h.Description,
		&h.Status,
		&h.Frequency,
		&h.TargetCount,
		&h.Category,
		&h.Color,
		&h.CurrentStreak,
		&h.BestStreak,
		&archivedAt,
		&h.CreatedAt,
		&h.UpdatedAt,
	); err != nil {
		return err
	}
	if archivedAt.Valid {
		t := archivedAt.Time
		h.ArchivedAt = &t
	}
	return nil
}

func scanHabitLog(scanner interface {
	Scan(dest ...any) error
}, log *models.HabitLog) error {
	return scanner.Scan(
		&log.ID,
		&log.HabitID,
		&log.UserID,
		&log.LogDate,
		&log.Count,
		&log.Note,
		&log.CreatedAt,
		&log.UpdatedAt,
	)
}

func (r *HabitRepository) Create(ctx context.Context, p HabitCreateParams) (*models.Habit, error) {
	row := r.DB.QueryRow(ctx, `
		INSERT INTO habits (
			user_id, title, description, status, frequency, target_count, category, color
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING `+habitSelect, p.UserID, p.Title, p.Description, p.Status, p.Frequency, p.TargetCount, p.Category, p.Color)
	var h models.Habit
	if err := scanHabit(row, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

func (r *HabitRepository) GetByID(ctx context.Context, id, userID string) (*models.Habit, error) {
	row := r.DB.QueryRow(ctx, `
		SELECT `+habitSelect+`
		FROM habits
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	var h models.Habit
	if err := scanHabit(row, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

func (r *HabitRepository) List(ctx context.Context, userID string, f HabitListFilter) ([]models.Habit, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 {
		f.Limit = 20
	}
	where := []string{"user_id = $1"}
	args := []any{userID}
	argN := 2

	if s := strings.TrimSpace(f.Status); s != "" {
		where = append(where, fmt.Sprintf("status = $%d", argN))
		args = append(args, s)
		argN++
	}
	if c := strings.TrimSpace(f.Category); c != "" {
		where = append(where, fmt.Sprintf("category ILIKE $%d", argN))
		args = append(args, c)
		argN++
	}
	if q := strings.TrimSpace(f.Search); q != "" {
		where = append(where, fmt.Sprintf("(title ILIKE $%d OR description ILIKE $%d OR category ILIKE $%d)", argN, argN, argN))
		args = append(args, "%"+q+"%")
		argN++
	}

	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := r.DB.QueryRow(ctx, `SELECT COUNT(*)::int FROM habits WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (f.Page - 1) * f.Limit
	args = append(args, f.Limit, offset)
	rows, err := r.DB.Query(ctx, `
		SELECT `+habitSelect+`
		FROM habits
		WHERE `+whereSQL+`
		ORDER BY
			CASE status WHEN 'active' THEN 0 WHEN 'paused' THEN 1 ELSE 2 END,
			created_at DESC
		LIMIT $`+fmt.Sprintf("%d", argN)+` OFFSET $`+fmt.Sprintf("%d", argN+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]models.Habit, 0)
	for rows.Next() {
		var h models.Habit
		if err := scanHabit(rows, &h); err != nil {
			return nil, 0, err
		}
		out = append(out, h)
	}
	return out, total, rows.Err()
}

func (r *HabitRepository) Update(ctx context.Context, id, userID string, p HabitUpdateParams) (*models.Habit, error) {
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
	if p.Description != nil {
		add("description", *p.Description)
	}
	if p.Status != nil {
		add("status", *p.Status)
	}
	if p.Frequency != nil {
		add("frequency", *p.Frequency)
	}
	if p.TargetCount != nil {
		add("target_count", *p.TargetCount)
	}
	if p.Category != nil {
		add("category", *p.Category)
	}
	if p.Color != nil {
		add("color", *p.Color)
	}
	if p.ArchivedAt != nil {
		if *p.ArchivedAt == nil {
			sets = append(sets, "archived_at = NULL")
		} else {
			add("archived_at", **p.ArchivedAt)
		}
	}

	if len(sets) == 0 {
		return r.GetByID(ctx, id, userID)
	}
	sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id, userID)

	row := r.DB.QueryRow(ctx, `
		UPDATE habits
		SET `+strings.Join(sets, ", ")+`
		WHERE id = $`+fmt.Sprintf("%d", argN)+` AND user_id = $`+fmt.Sprintf("%d", argN+1)+`
		RETURNING `+habitSelect, args...)
	var h models.Habit
	if err := scanHabit(row, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

func (r *HabitRepository) Delete(ctx context.Context, id, userID string) error {
	tag, err := r.DB.Exec(ctx, `DELETE FROM habits WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *HabitRepository) Stats(ctx context.Context, userID string) (*models.HabitStats, error) {
	s := &models.HabitStats{}
	err := r.DB.QueryRow(ctx, `
		SELECT
			COUNT(*)::int AS total,
			COUNT(*) FILTER (WHERE status = 'active')::int AS active,
			COUNT(*) FILTER (WHERE status = 'paused')::int AS paused,
			COUNT(*) FILTER (WHERE status = 'archived')::int AS archived,
			COUNT(*) FILTER (
				WHERE status = 'active'
				  AND EXISTS (
					SELECT 1 FROM habit_logs l
					WHERE l.habit_id = habits.id
					  AND l.log_date = CURRENT_DATE
					  AND l.count >= habits.target_count
				  )
			)::int AS completed_today,
			COUNT(*) FILTER (
				WHERE status = 'active'
				  AND NOT EXISTS (
					SELECT 1 FROM habit_logs l
					WHERE l.habit_id = habits.id
					  AND l.log_date = CURRENT_DATE
					  AND l.count >= habits.target_count
				  )
			)::int AS pending_today,
			COALESCE(ROUND(AVG(current_streak) FILTER (WHERE status = 'active')), 0)::int AS avg_streak,
			COALESCE(MAX(best_streak), 0)::int AS best_streak
		FROM habits
		WHERE user_id = $1
	`, userID).Scan(
		&s.Total, &s.Active, &s.Paused, &s.Archived,
		&s.CompletedToday, &s.PendingToday, &s.AvgStreak, &s.BestStreak,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *HabitRepository) UpsertLog(ctx context.Context, habitID, userID string, logDate time.Time, count int, note string) (*models.HabitLog, error) {
	row := r.DB.QueryRow(ctx, `
		INSERT INTO habit_logs (habit_id, user_id, log_date, count, note)
		VALUES ($1, $2, $3::date, $4, $5)
		ON CONFLICT (habit_id, log_date)
		DO UPDATE SET
			count = EXCLUDED.count,
			note = EXCLUDED.note,
			updated_at = CURRENT_TIMESTAMP
		RETURNING `+habitLogSelect, habitID, userID, logDate.Format("2006-01-02"), count, note)
	var log models.HabitLog
	if err := scanHabitLog(row, &log); err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *HabitRepository) GetLogByDate(ctx context.Context, habitID, userID string, logDate time.Time) (*models.HabitLog, error) {
	row := r.DB.QueryRow(ctx, `
		SELECT `+habitLogSelect+`
		FROM habit_logs
		WHERE habit_id = $1 AND user_id = $2 AND log_date = $3::date
	`, habitID, userID, logDate.Format("2006-01-02"))
	var log models.HabitLog
	if err := scanHabitLog(row, &log); err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *HabitRepository) DeleteLog(ctx context.Context, habitID, userID string, logDate time.Time) error {
	tag, err := r.DB.Exec(ctx, `
		DELETE FROM habit_logs
		WHERE habit_id = $1 AND user_id = $2 AND log_date = $3::date
	`, habitID, userID, logDate.Format("2006-01-02"))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *HabitRepository) ListLogs(ctx context.Context, habitID, userID string, from, to time.Time) ([]models.HabitLog, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT `+habitLogSelect+`
		FROM habit_logs
		WHERE habit_id = $1 AND user_id = $2
		  AND log_date >= $3::date AND log_date <= $4::date
		ORDER BY log_date DESC
	`, habitID, userID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.HabitLog, 0)
	for rows.Next() {
		var log models.HabitLog
		if err := scanHabitLog(rows, &log); err != nil {
			return nil, err
		}
		out = append(out, log)
	}
	return out, rows.Err()
}

func (r *HabitRepository) TodayCounts(ctx context.Context, userID string, habitIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(habitIDs))
	if len(habitIDs) == 0 {
		return out, nil
	}
	rows, err := r.DB.Query(ctx, `
		SELECT habit_id::text, count
		FROM habit_logs
		WHERE user_id = $1
		  AND log_date = CURRENT_DATE
		  AND habit_id = ANY($2::uuid[])
	`, userID, habitIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		out[id] = count
	}
	return out, rows.Err()
}

func (r *HabitRepository) CompletedDates(ctx context.Context, habitID, userID string, targetCount int, limitDays int) ([]time.Time, error) {
	if limitDays < 1 {
		limitDays = 400
	}
	rows, err := r.DB.Query(ctx, `
		SELECT log_date
		FROM habit_logs
		WHERE habit_id = $1 AND user_id = $2 AND count >= $3
		ORDER BY log_date DESC
		LIMIT $4
	`, habitID, userID, targetCount, limitDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]time.Time, 0)
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *HabitRepository) UpdateStreaks(ctx context.Context, id, userID string, current, best int) (*models.Habit, error) {
	row := r.DB.QueryRow(ctx, `
		UPDATE habits
		SET current_streak = $3, best_streak = $4, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND user_id = $2
		RETURNING `+habitSelect, id, userID, current, best)
	var h models.Habit
	if err := scanHabit(row, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

func (r *HabitRepository) CountsByStatus(ctx context.Context, userID string) ([]models.NamedCount, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT status, COUNT(*)::int
		FROM habits
		WHERE user_id = $1
		GROUP BY status
		ORDER BY
			CASE status
				WHEN 'active' THEN 0
				WHEN 'paused' THEN 1
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

func (r *HabitRepository) CountsByFrequency(ctx context.Context, userID string) ([]models.NamedCount, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT frequency, COUNT(*)::int
		FROM habits
		WHERE user_id = $1
		GROUP BY frequency
		ORDER BY
			CASE frequency
				WHEN 'daily' THEN 0
				ELSE 1
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

func (r *HabitRepository) CountsByCategory(ctx context.Context, userID string, limit int) ([]models.NamedCount, error) {
	if limit < 1 {
		limit = 6
	}
	rows, err := r.DB.Query(ctx, `
		SELECT
			CASE WHEN TRIM(category) = '' THEN 'uncategorized' ELSE category END AS category,
			COUNT(*)::int
		FROM habits
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

func (r *HabitRepository) CheckinTrend(ctx context.Context, userID string, days int) ([]models.DayCount, error) {
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
		completed AS (
			SELECT TO_CHAR(l.log_date, 'YYYY-MM-DD') AS day, COUNT(*)::int AS count
			FROM habit_logs l
			INNER JOIN habits h ON h.id = l.habit_id AND h.user_id = l.user_id
			WHERE l.user_id = $2
			  AND l.count >= h.target_count
			  AND l.log_date >= CURRENT_DATE - (($1::int - 1) || ' days')::interval
			GROUP BY 1
		)
		SELECT d.day, COALESCE(c.count, 0)::int
		FROM days d
		LEFT JOIN completed c ON c.day = d.day
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

func (r *HabitRepository) StreakBuckets(ctx context.Context, userID string) ([]models.ProgressBucket, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT bucket, COUNT(*)::int
		FROM (
			SELECT
				CASE
					WHEN current_streak = 0 THEN '0'
					WHEN current_streak <= 3 THEN '1-3'
					WHEN current_streak <= 7 THEN '4-7'
					WHEN current_streak <= 14 THEN '8-14'
					ELSE '15+'
				END AS bucket,
				CASE
					WHEN current_streak = 0 THEN 0
					WHEN current_streak <= 3 THEN 1
					WHEN current_streak <= 7 THEN 2
					WHEN current_streak <= 14 THEN 3
					ELSE 4
				END AS sort_key
			FROM habits
			WHERE user_id = $1 AND status IN ('active', 'paused')
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
