package service

import (
	"context"
	"time"

	"go-be/internal/models"
	"go-be/internal/repository"
)

type AnalyticsService struct {
	Goals   *repository.GoalRepository
	Habits  *repository.HabitRepository
	Planner *repository.PlannerRepository
}

func NewAnalyticsService(
	goals *repository.GoalRepository,
	habits *repository.HabitRepository,
	planner *repository.PlannerRepository,
) *AnalyticsService {
	return &AnalyticsService{Goals: goals, Habits: habits, Planner: planner}
}

func computeFocusScore(goal *models.GoalStats, habit *models.HabitStats, planner *models.PlannerDayStats) int {
	score := 0
	parts := 0

	if goal.Total > 0 {
		score += clampProgress(goal.AvgProgress)
		parts++
	}

	activeHabits := habit.CompletedToday + habit.PendingToday
	if activeHabits > 0 {
		score += clampProgress(int(float64(habit.CompletedToday) / float64(activeHabits) * 100))
		parts++
	}

	if planner.Total > 0 {
		score += clampProgress(planner.Completion)
		parts++
	}

	if parts == 0 {
		return 0
	}
	return score / parts
}

func (s *AnalyticsService) Get(ctx context.Context, userID string) (*models.LifeAnalytics, error) {
	goalStats, err := s.Goals.Stats(ctx, userID)
	if err != nil {
		return nil, err
	}
	habitStats, err := s.Habits.Stats(ctx, userID)
	if err != nil {
		return nil, err
	}
	plannerStats, err := s.Planner.DayStats(ctx, userID, truncateDate(time.Now().UTC()))
	if err != nil {
		return nil, err
	}

	goalsByStatus, err := s.Goals.CountsByStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	habitsByStatus, err := s.Habits.CountsByStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	habitTrend, err := s.Habits.CheckinTrend(ctx, userID, 7)
	if err != nil {
		return nil, err
	}
	plannerWeekly, err := s.Planner.WeeklyActivity(ctx, userID, 7)
	if err != nil {
		return nil, err
	}
	plannerTrend, err := s.Planner.DailyDoneTrend(ctx, userID, 14)
	if err != nil {
		return nil, err
	}

	habitByDate := make(map[string]int, len(habitTrend))
	for _, item := range habitTrend {
		habitByDate[item.Date] = item.Count
	}
	weekly := make([]models.LifeDayActivity, 0, len(plannerWeekly))
	for _, day := range plannerWeekly {
		weekly = append(weekly, models.LifeDayActivity{
			Date:            day.Date,
			HabitsCompleted: habitByDate[day.Date],
			PlannerDone:     day.PlannerDone,
			PlannerTotal:    day.PlannerTotal,
		})
	}

	return &models.LifeAnalytics{
		Overview: models.LifeAnalyticsOverview{
			GoalsTotal:        goalStats.Total,
			GoalsActive:       goalStats.Active,
			GoalsCompleted:    goalStats.Completed,
			GoalsAvgProgress:  goalStats.AvgProgress,
			HabitsActive:      habitStats.Active,
			HabitsDoneToday:   habitStats.CompletedToday,
			HabitsPending:     habitStats.PendingToday,
			HabitsBestStreak:  habitStats.BestStreak,
			PlannerTotal:      plannerStats.Total,
			PlannerDone:       plannerStats.Done,
			PlannerCompletion: plannerStats.Completion,
			FocusScore:        computeFocusScore(goalStats, habitStats, plannerStats),
		},
		WeeklyActivity: weekly,
		GoalsByStatus:  goalsByStatus,
		HabitsByStatus: habitsByStatus,
		PlannerTrend:   plannerTrend,
	}, nil
}
