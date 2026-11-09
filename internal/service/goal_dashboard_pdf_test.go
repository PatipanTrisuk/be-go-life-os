package service_test

import (
	"go-be/internal/models"
	"go-be/internal/service"
	"testing"
)

func TestBuildDashboardPDF(t *testing.T) {
	s := &service.GoalService{}
	dash := &models.GoalDashboard{
		Stats:           models.GoalStats{Total: 1, Active: 1, AvgProgress: 10},
		ByStatus:        []models.NamedCount{{Key: "active", Count: 1}},
		ByPriority:      []models.NamedCount{{Key: "medium", Count: 1}},
		ByCategory:      []models.NamedCount{{Key: "การเงิน", Count: 1}},
		CreatedTrend:    []models.MonthCount{{Month: "2026-11", Created: 1}},
		ProgressBuckets: []models.ProgressBucket{{Bucket: "0-20", Count: 1}},
	}
	b, name, err := s.BuildDashboardPDF(dash, "th")
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 100 {
		t.Fatalf("pdf too small: %d", len(b))
	}
	if name == "" {
		t.Fatal("empty name")
	}
	t.Logf("ok bytes=%d name=%s", len(b), name)
}
