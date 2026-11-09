package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"go-be/internal/i18n"
	"go-be/internal/service"
)

type BillingHandler struct {
	EntitlementService *service.EntitlementService
}

func NewBillingHandler(entitlementService *service.EntitlementService) *BillingHandler {
	return &BillingHandler{EntitlementService: entitlementService}
}

type adminUpdateSubscriptionRequest struct {
	PlanCode string `json:"plan_code" binding:"required"`
}

func (h *BillingHandler) writeBillingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidPlan):
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": i18n.T(c, "billing.invalid_plan"),
			"code":    "billing.invalid_plan",
		})
	case errors.Is(err, service.ErrUserNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": i18n.T(c, "billing.subscription_not_found"),
			"code":    "billing.subscription_not_found",
		})
	default:
		log.Printf("billing action failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": i18n.T(c, "common.internal_error"),
			"code":    "common.internal_error",
		})
	}
}

// ListPlans is public (no auth).
func (h *BillingHandler) ListPlans(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	plans, err := h.EntitlementService.GetPlans(ctx)
	if err != nil {
		h.writeBillingError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": plans})
}

func (h *BillingHandler) GetSubscription(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	snap, err := h.EntitlementService.GetSnapshot(ctx, userID)
	if err != nil {
		h.writeBillingError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": snap})
}

func (h *BillingHandler) Upgrade(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	snap, err := h.EntitlementService.Upgrade(ctx, userID)
	if err != nil {
		h.writeBillingError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "billing.upgraded"),
		"data":    snap,
	})
}

func (h *BillingHandler) Downgrade(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	snap, err := h.EntitlementService.Downgrade(ctx, userID)
	if err != nil {
		h.writeBillingError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "billing.downgraded"),
		"data":    snap,
	})
}

// AdminUpdateSubscription sets a user's plan. Body: { "plan_code": "pro" }.
func (h *BillingHandler) AdminUpdateSubscription(c *gin.Context) {
	var req adminUpdateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeInvalidRequest(c)
		return
	}
	if strings.TrimSpace(req.PlanCode) == "" {
		writeInvalidRequest(c)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	snap, err := h.EntitlementService.AdminSetPlan(ctx, c.Param("id"), req.PlanCode)
	if err != nil {
		h.writeBillingError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "billing.plan_updated"),
		"data":    snap,
	})
}
