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

type GoalHandler struct {
	GoalService *service.GoalService
}

func NewGoalHandler(goalService *service.GoalService) *GoalHandler {
	return &GoalHandler{GoalService: goalService}
}

func (h *GoalHandler) writeGoalError(c *gin.Context, err error) {
	if writePlanLimitError(c, err) {
		return
	}
	switch {
	case errors.Is(err, service.ErrGoalNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": i18n.T(c, "goal.not_found"),
			"code":    "goal.not_found",
		})
	case errors.Is(err, service.ErrMilestoneNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": i18n.T(c, "goal.milestone_not_found"),
			"code":    "goal.milestone_not_found",
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

func (h *GoalHandler) List(c *gin.Context) {
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

	result, err := h.GoalService.List(ctx, userID, repository.GoalListFilter{
		Status:   strings.TrimSpace(c.Query("status")),
		Priority: strings.TrimSpace(c.Query("priority")),
		Category: strings.TrimSpace(c.Query("category")),
		Search:   strings.TrimSpace(c.Query("q")),
		Page:     parsePositiveInt(c.Query("page"), defaultPage, 0),
		Limit:    parsePositiveInt(c.Query("limit"), 20, maxLimit),
	})
	if err != nil {
		h.writeGoalError(c, err)
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

func (h *GoalHandler) Stats(c *gin.Context) {
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

	stats, err := h.GoalService.Stats(ctx, userID)
	if err != nil {
		h.writeGoalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   stats,
	})
}

func (h *GoalHandler) Dashboard(c *gin.Context) {
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

	dashboard, err := h.GoalService.Dashboard(ctx, userID)
	if err != nil {
		h.writeGoalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   dashboard,
	})
}

func (h *GoalHandler) ExportDashboardPDF(c *gin.Context) {
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

	pdfBytes, fileName, err := h.GoalService.ExportDashboardPDF(ctx, userID, localeFromContext(c))
	if err != nil {
		h.writeGoalError(c, err)
		return
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", `attachment; filename="`+fileName+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

func (h *GoalHandler) Get(c *gin.Context) {
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

	goal, err := h.GoalService.Get(ctx, userID, c.Param("id"))
	if err != nil {
		h.writeGoalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   goal,
	})
}

func (h *GoalHandler) Create(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.unauthorized"),
		})
		return
	}

	var req service.GoalCreateInput
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

	goal, err := h.GoalService.Create(ctx, userID, req)
	if err != nil {
		h.writeGoalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": i18n.T(c, "goal.created"),
		"data":    goal,
	})
}

func (h *GoalHandler) Update(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.unauthorized"),
		})
		return
	}

	var req service.GoalUpdateInput
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

	goal, err := h.GoalService.Update(ctx, userID, c.Param("id"), req)
	if err != nil {
		h.writeGoalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "goal.updated"),
		"data":    goal,
	})
}

func (h *GoalHandler) Delete(c *gin.Context) {
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

	if err := h.GoalService.Delete(ctx, userID, c.Param("id")); err != nil {
		h.writeGoalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "goal.deleted"),
	})
}

func (h *GoalHandler) AddMilestone(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.unauthorized"),
		})
		return
	}

	var req service.MilestoneCreateInput
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

	goal, err := h.GoalService.AddMilestone(ctx, userID, c.Param("id"), req)
	if err != nil {
		h.writeGoalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": i18n.T(c, "goal.milestone_created"),
		"data":    goal,
	})
}

func (h *GoalHandler) UpdateMilestone(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.unauthorized"),
		})
		return
	}

	var req service.MilestoneUpdateInput
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

	goal, err := h.GoalService.UpdateMilestone(ctx, userID, c.Param("id"), c.Param("mid"), req)
	if err != nil {
		h.writeGoalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "goal.milestone_updated"),
		"data":    goal,
	})
}

func (h *GoalHandler) DeleteMilestone(c *gin.Context) {
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

	goal, err := h.GoalService.DeleteMilestone(ctx, userID, c.Param("id"), c.Param("mid"))
	if err != nil {
		h.writeGoalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "goal.milestone_deleted"),
		"data":    goal,
	})
}
