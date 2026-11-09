package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"go-be/internal/i18n"
	"go-be/internal/repository"
	"go-be/internal/service"
)

type HabitHandler struct {
	HabitService *service.HabitService
}

func NewHabitHandler(habitService *service.HabitService) *HabitHandler {
	return &HabitHandler{HabitService: habitService}
}

func (h *HabitHandler) writeHabitError(c *gin.Context, err error) {
	if writePlanLimitError(c, err) {
		return
	}
	switch {
	case errors.Is(err, service.ErrHabitNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": i18n.T(c, "habit.not_found"),
			"code":    "habit.not_found",
		})
	case errors.Is(err, service.ErrHabitLogNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": i18n.T(c, "habit.log_not_found"),
			"code":    "habit.log_not_found",
		})
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.invalid_input"),
			"code":    "auth.invalid_input",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": i18n.T(c, "common.internal_error"),
			"code":    "common.internal_error",
		})
	}
}

func (h *HabitHandler) List(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.unauthorized"),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := h.HabitService.List(ctx, userID, repository.HabitListFilter{
		Status:   strings.TrimSpace(c.Query("status")),
		Category: strings.TrimSpace(c.Query("category")),
		Search:   strings.TrimSpace(c.Query("q")),
		Page:     parsePositiveInt(c.Query("page"), defaultPage, 0),
		Limit:    parsePositiveInt(c.Query("limit"), 20, maxLimit),
	})
	if err != nil {
		h.writeHabitError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   result.Items,
		"meta": gin.H{
			"page":  result.Page,
			"limit": result.Limit,
			"total": result.Total,
		},
	})
}

func (h *HabitHandler) Stats(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.unauthorized"),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := h.HabitService.Stats(ctx, userID)
	if err != nil {
		h.writeHabitError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   stats,
	})
}

func (h *HabitHandler) Dashboard(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.unauthorized"),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	dashboard, err := h.HabitService.Dashboard(ctx, userID)
	if err != nil {
		h.writeHabitError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   dashboard,
	})
}

func (h *HabitHandler) ExportDashboardPDF(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.unauthorized"),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pdfBytes, fileName, err := h.HabitService.ExportDashboardPDF(ctx, userID, localeFromContext(c))
	if err != nil {
		h.writeHabitError(c, err)
		return
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", `attachment; filename="`+fileName+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

func (h *HabitHandler) Get(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.unauthorized"),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	habit, err := h.HabitService.Get(ctx, userID, c.Param("id"))
	if err != nil {
		h.writeHabitError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   habit,
	})
}

func (h *HabitHandler) Create(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.unauthorized"),
		})
		return
	}

	var req service.HabitCreateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.invalid_request"),
			"code":    "auth.invalid_request",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	habit, err := h.HabitService.Create(ctx, userID, req)
	if err != nil {
		h.writeHabitError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": i18n.T(c, "habit.created"),
		"data":    habit,
	})
}

func (h *HabitHandler) Update(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.unauthorized"),
		})
		return
	}

	var req service.HabitUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.invalid_request"),
			"code":    "auth.invalid_request",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	habit, err := h.HabitService.Update(ctx, userID, c.Param("id"), req)
	if err != nil {
		h.writeHabitError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "habit.updated"),
		"data":    habit,
	})
}

func (h *HabitHandler) Delete(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.unauthorized"),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.HabitService.Delete(ctx, userID, c.Param("id")); err != nil {
		h.writeHabitError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "habit.deleted"),
	})
}

func (h *HabitHandler) CheckIn(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.unauthorized"),
		})
		return
	}

	var req service.HabitCheckInInput
	_ = c.ShouldBindJSON(&req)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	habit, err := h.HabitService.CheckIn(ctx, userID, c.Param("id"), req)
	if err != nil {
		h.writeHabitError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "habit.checked_in"),
		"data":    habit,
	})
}

func (h *HabitHandler) ListLogs(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.unauthorized"),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logs, err := h.HabitService.ListLogs(ctx, userID, c.Param("id"), c.Query("from"), c.Query("to"))
	if err != nil {
		h.writeHabitError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   logs,
	})
}
