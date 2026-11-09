package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"go-be/internal/i18n"
	"go-be/internal/service"
)

type PlannerHandler struct {
	PlannerService *service.PlannerService
}

func NewPlannerHandler(plannerService *service.PlannerService) *PlannerHandler {
	return &PlannerHandler{PlannerService: plannerService}
}

func (h *PlannerHandler) writePlannerError(c *gin.Context, err error) {
	if writePlanLimitError(c, err) {
		return
	}
	switch {
	case errors.Is(err, service.ErrPlannerNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": i18n.T(c, "planner.not_found"),
			"code":    "planner.not_found",
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

func (h *PlannerHandler) List(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": i18n.T(c, "auth.unauthorized")})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	items, err := h.PlannerService.List(ctx, userID, c.Query("date"))
	if err != nil {
		h.writePlannerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": items})
}

func (h *PlannerHandler) Stats(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": i18n.T(c, "auth.unauthorized")})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := h.PlannerService.Stats(ctx, userID, c.Query("date"))
	if err != nil {
		h.writePlannerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": stats})
}

func (h *PlannerHandler) Get(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": i18n.T(c, "auth.unauthorized")})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := h.PlannerService.Get(ctx, userID, c.Param("id"))
	if err != nil {
		h.writePlannerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": item})
}

func (h *PlannerHandler) Create(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": i18n.T(c, "auth.unauthorized")})
		return
	}
	var req service.PlannerCreateInput
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

	item, err := h.PlannerService.Create(ctx, userID, req)
	if err != nil {
		h.writePlannerError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": i18n.T(c, "planner.created"),
		"data":    item,
	})
}

func (h *PlannerHandler) Update(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": i18n.T(c, "auth.unauthorized")})
		return
	}
	var req service.PlannerUpdateInput
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

	item, err := h.PlannerService.Update(ctx, userID, c.Param("id"), req)
	if err != nil {
		h.writePlannerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "planner.updated"),
		"data":    item,
	})
}

func (h *PlannerHandler) Toggle(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": i18n.T(c, "auth.unauthorized")})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := h.PlannerService.Toggle(ctx, userID, c.Param("id"))
	if err != nil {
		h.writePlannerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "planner.updated"),
		"data":    item,
	})
}

func (h *PlannerHandler) Delete(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": i18n.T(c, "auth.unauthorized")})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.PlannerService.Delete(ctx, userID, c.Param("id")); err != nil {
		h.writePlannerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "planner.deleted"),
	})
}
