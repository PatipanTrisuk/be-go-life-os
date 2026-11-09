package service

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"go-be/internal/models"
	"go-be/internal/repository"
)

var (
	ErrExportJobNotFound = errors.New("export job not found")
	ErrExportNotReady    = errors.New("export job not ready")
	ErrExportForbidden   = errors.New("export job forbidden")
)

type ExportFilters struct {
	Search        string `json:"search"`
	Role          string `json:"role"`
	AuthProvider  string `json:"auth_provider"`
	DeletedStatus string `json:"deleted_status"`
	CreatedFrom   string `json:"created_from"`
	CreatedTo     string `json:"created_to"`
	UpdatedFrom   string `json:"updated_from"`
	UpdatedTo     string `json:"updated_to"`
	SortBy        string `json:"sort_by"`
	SortDir       string `json:"sort_dir"`
}

type ExportService struct {
	ExportJobRepo   *repository.ExportJobRepository
	UserRepo        *repository.UserRepository
	NotificationSvc *NotificationService
	queue           chan string
	exportDir       string
}

func NewExportService(
	exportJobRepo *repository.ExportJobRepository,
	userRepo *repository.UserRepository,
	notificationSvc *NotificationService,
	exportDir string,
) *ExportService {
	if strings.TrimSpace(exportDir) == "" {
		exportDir = filepath.Join("storage", "exports")
	}
	s := &ExportService{
		ExportJobRepo:   exportJobRepo,
		UserRepo:        userRepo,
		NotificationSvc: notificationSvc,
		queue:           make(chan string, 64),
		exportDir:       exportDir,
	}
	return s
}

func (s *ExportService) Start(ctx context.Context, workers int) {
	if workers < 1 {
		workers = 1
	}
	if err := os.MkdirAll(s.exportDir, 0o755); err != nil {
		log.Printf("export dir create failed: %v", err)
	}
	for i := 0; i < workers; i++ {
		go s.worker(ctx, i+1)
	}
}

func (s *ExportService) EnqueueUsersCSV(ctx context.Context, userID string, filters ExportFilters) (*models.ExportJob, error) {
	raw, err := json.Marshal(filters)
	if err != nil {
		return nil, err
	}

	job, err := s.ExportJobRepo.Create(ctx, userID, models.ExportTypeUsersCSV, string(raw))
	if err != nil {
		return nil, err
	}

	s.pushNotification(ctx, userID, models.NotificationTypeExportQueued, job.ID, map[string]any{
		"export_type": job.Type,
	})

	select {
	case s.queue <- job.ID.String():
	default:
		// Queue full — still process eventually via direct goroutine.
		go s.process(context.Background(), job.ID.String())
	}

	return job, nil
}

func (s *ExportService) GetJobForUser(ctx context.Context, jobID, userID string) (*models.ExportJob, error) {
	job, err := s.ExportJobRepo.FindByID(ctx, jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrExportJobNotFound
	}
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(job.UserID.String(), userID) {
		return nil, ErrExportForbidden
	}
	return job, nil
}

func (s *ExportService) OpenCompletedFile(ctx context.Context, jobID, userID string) (*models.ExportJob, *os.File, error) {
	job, err := s.GetJobForUser(ctx, jobID, userID)
	if err != nil {
		return nil, nil, err
	}
	if job.Status != models.ExportJobCompleted || job.FilePath == nil || strings.TrimSpace(*job.FilePath) == "" {
		return nil, nil, ErrExportNotReady
	}
	f, err := os.Open(*job.FilePath)
	if err != nil {
		return nil, nil, err
	}
	return job, f, nil
}

func (s *ExportService) worker(ctx context.Context, id int) {
	for {
		select {
		case <-ctx.Done():
			return
		case jobID := <-s.queue:
			s.process(ctx, jobID)
			_ = id
		}
	}
}

func (s *ExportService) process(ctx context.Context, jobID string) {
	jobCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := s.ExportJobRepo.MarkProcessing(jobCtx, jobID); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("export mark processing failed id=%s: %v", jobID, err)
		}
		return
	}

	job, err := s.ExportJobRepo.FindByID(jobCtx, jobID)
	if err != nil {
		log.Printf("export load job failed id=%s: %v", jobID, err)
		return
	}

	var filters ExportFilters
	if err := json.Unmarshal([]byte(job.FiltersJSON), &filters); err != nil {
		s.failJob(jobCtx, job, "invalid filters")
		return
	}

	params := repository.ListParams{
		Query:         filters.Search,
		Role:          filters.Role,
		AuthProvider:  filters.AuthProvider,
		DeletedStatus: filters.DeletedStatus,
		CreatedFrom:   parseExportDate(filters.CreatedFrom, false),
		CreatedTo:     parseExportDate(filters.CreatedTo, true),
		UpdatedFrom:   parseExportDate(filters.UpdatedFrom, false),
		UpdatedTo:     parseExportDate(filters.UpdatedTo, true),
		SortBy:        filters.SortBy,
		SortDir:       filters.SortDir,
	}

	users, err := s.UserRepo.ListAll(jobCtx, params)
	if err != nil {
		log.Printf("export list users failed id=%s: %v", jobID, err)
		s.failJob(jobCtx, job, "failed to query users")
		return
	}

	fileName := fmt.Sprintf("users-%s.csv", time.Now().Format("20060102-150405"))
	filePath := filepath.Join(s.exportDir, jobID+"-"+fileName)

	f, err := os.Create(filePath)
	if err != nil {
		log.Printf("export create file failed id=%s: %v", jobID, err)
		s.failJob(jobCtx, job, "failed to create file")
		return
	}

	if _, err := f.Write([]byte("\xEF\xBB\xBF")); err != nil {
		_ = f.Close()
		_ = os.Remove(filePath)
		s.failJob(jobCtx, job, "failed to write file")
		return
	}

	writer := csv.NewWriter(f)
	_ = writer.Write([]string{
		"id",
		"username",
		"email",
		"role",
		"auth_provider",
		"discord_id",
		"email_verified_at",
		"deleted_at",
		"created_at",
		"updated_at",
	})

	for _, user := range users {
		discordID := ""
		if user.DiscordID != nil {
			discordID = *user.DiscordID
		}
		emailVerified := ""
		if user.EmailVerifiedAt != nil {
			emailVerified = user.EmailVerifiedAt.UTC().Format(time.RFC3339)
		}
		deletedAt := ""
		if user.DeletedAt != nil {
			deletedAt = user.DeletedAt.UTC().Format(time.RFC3339)
		}
		_ = writer.Write([]string{
			user.ID.String(),
			user.Username,
			user.Email,
			user.Role,
			user.AuthProvider,
			discordID,
			emailVerified,
			deletedAt,
			user.CreatedAt.UTC().Format(time.RFC3339),
			user.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		_ = f.Close()
		_ = os.Remove(filePath)
		s.failJob(jobCtx, job, "failed to write csv")
		return
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(filePath)
		s.failJob(jobCtx, job, "failed to close file")
		return
	}

	if err := s.ExportJobRepo.MarkCompleted(jobCtx, jobID, fileName, filePath); err != nil {
		log.Printf("export mark completed failed id=%s: %v", jobID, err)
		return
	}

	s.pushNotification(jobCtx, job.UserID.String(), models.NotificationTypeExportReady, job.ID, map[string]any{
		"export_type": job.Type,
		"file_name":   fileName,
	})

	select {
	case <-ctx.Done():
	default:
	}
}

func (s *ExportService) failJob(ctx context.Context, job *models.ExportJob, message string) {
	if err := s.ExportJobRepo.MarkFailed(ctx, job.ID.String(), message); err != nil {
		log.Printf("export mark failed id=%s: %v", job.ID.String(), err)
	}
	s.pushNotification(ctx, job.UserID.String(), models.NotificationTypeExportFailed, job.ID, map[string]any{
		"export_type":   job.Type,
		"error_message": message,
	})
}

func (s *ExportService) pushNotification(
	ctx context.Context,
	userID, notifType string,
	jobID uuid.UUID,
	meta map[string]any,
) {
	if s.NotificationSvc == nil {
		return
	}
	refType := models.NotificationRefExportJob
	id := jobID
	if err := s.NotificationSvc.Create(ctx, userID, notifType, &refType, &id, meta); err != nil {
		log.Printf("export notification create failed type=%s job=%s: %v", notifType, jobID.String(), err)
	}
}

func parseExportDate(raw string, endOfDay bool) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		return nil
	}
	if endOfDay {
		t = t.Add(24*time.Hour - time.Nanosecond)
	}
	return &t
}
