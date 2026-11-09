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

// requireActor returns the authenticated user id or writes a 401 and returns false.
func requireActor(c *gin.Context) (string, bool) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": i18n.T(c, "auth.unauthorized")})
		return "", false
	}
	return userID, true
}

func writeInvalidRequest(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{
		"status":  "error",
		"message": i18n.T(c, "auth.invalid_request"),
		"code":    "auth.invalid_request",
	})
}

// writePlanLimitError writes a 403 billing.limit_reached response when err is a PlanLimitError.
func writePlanLimitError(c *gin.Context, err error) bool {
	var lim *service.PlanLimitError
	if !errors.As(err, &lim) {
		return false
	}
	c.JSON(http.StatusForbidden, gin.H{
		"status":  "error",
		"message": i18n.T(c, "billing.limit_reached"),
		"code":    "billing.limit_reached",
		"meta":    gin.H{"feature": lim.Feature, "limit": lim.Limit, "usage": lim.Usage},
	})
	return true
}

// writeLifeOSError maps a not-found sentinel / invalid input / internal errors to responses.
func writeLifeOSError(c *gin.Context, err error, notFound error, notFoundKey string) {
	if writePlanLimitError(c, err) {
		return
	}
	switch {
	case notFound != nil && errors.Is(err, notFound):
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": i18n.T(c, notFoundKey),
			"code":    notFoundKey,
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

type CalendarHandler struct {
	CalendarService *service.CalendarService
}

func NewCalendarHandler(calendarService *service.CalendarService) *CalendarHandler {
	return &CalendarHandler{CalendarService: calendarService}
}

func (h *CalendarHandler) writeCalendarError(c *gin.Context, err error) {
	writeLifeOSError(c, err, service.ErrCalendarNotFound, "calendar.not_found")
}

func (h *CalendarHandler) List(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	items, err := h.CalendarService.List(ctx, userID, c.Query("from"), c.Query("to"))
	if err != nil {
		h.writeCalendarError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": items})
}

func (h *CalendarHandler) Stats(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := h.CalendarService.Stats(ctx, userID, c.Query("from"), c.Query("to"))
	if err != nil {
		h.writeCalendarError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": stats})
}

func (h *CalendarHandler) Get(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := h.CalendarService.Get(ctx, userID, c.Param("id"))
	if err != nil {
		h.writeCalendarError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": item})
}

func (h *CalendarHandler) Create(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	var req service.CalendarCreateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		writeInvalidRequest(c)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := h.CalendarService.Create(ctx, userID, req)
	if err != nil {
		h.writeCalendarError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": i18n.T(c, "calendar.created"),
		"data":    item,
	})
}

func (h *CalendarHandler) Update(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	var req service.CalendarUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		writeInvalidRequest(c)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := h.CalendarService.Update(ctx, userID, c.Param("id"), req)
	if err != nil {
		h.writeCalendarError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "calendar.updated"),
		"data":    item,
	})
}

func (h *CalendarHandler) Delete(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.CalendarService.Delete(ctx, userID, c.Param("id")); err != nil {
		h.writeCalendarError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "calendar.deleted"),
	})
}
