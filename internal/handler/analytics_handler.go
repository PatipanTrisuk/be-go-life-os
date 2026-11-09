package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"go-be/internal/i18n"
	"go-be/internal/service"
)

type AnalyticsHandler struct {
	AnalyticsService *service.AnalyticsService
}

func NewAnalyticsHandler(analyticsService *service.AnalyticsService) *AnalyticsHandler {
	return &AnalyticsHandler{AnalyticsService: analyticsService}
}

func (h *AnalyticsHandler) Get(c *gin.Context) {
	userID := actorID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": i18n.T(c, "auth.unauthorized"),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data, err := h.AnalyticsService.Get(ctx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": i18n.T(c, "common.internal_error"),
			"code":    "common.internal_error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   data,
	})
}
