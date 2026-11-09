package models

type LifeAnalyticsOverview struct {
	GoalsTotal        int `json:"goals_total"`
	GoalsActive       int `json:"goals_active"`
	GoalsCompleted    int `json:"goals_completed"`
	GoalsAvgProgress  int `json:"goals_avg_progress"`
	HabitsActive      int `json:"habits_active"`
	HabitsDoneToday   int `json:"habits_done_today"`
	HabitsPending     int `json:"habits_pending_today"`
	HabitsBestStreak  int `json:"habits_best_streak"`
	PlannerTotal      int `json:"planner_total_today"`
	PlannerDone       int `json:"planner_done_today"`
	PlannerCompletion int `json:"planner_completion_today"`
	FocusScore        int `json:"focus_score"` // 0-100 composite
}

type LifeDayActivity struct {
	Date            string `json:"date"`
	HabitsCompleted int    `json:"habits_completed"`
	PlannerDone     int    `json:"planner_done"`
	PlannerTotal    int    `json:"planner_total"`
}

type LifeAnalytics struct {
	Overview       LifeAnalyticsOverview `json:"overview"`
	WeeklyActivity []LifeDayActivity     `json:"weekly_activity"`
	GoalsByStatus  []NamedCount          `json:"goals_by_status"`
	HabitsByStatus []NamedCount          `json:"habits_by_status"`
	PlannerTrend   []DayCount            `json:"planner_trend"`
}
