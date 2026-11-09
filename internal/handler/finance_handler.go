package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"go-be/internal/i18n"
	"go-be/internal/service"
)

type FinanceHandler struct {
	FinanceService *service.FinanceService
}

func NewFinanceHandler(financeService *service.FinanceService) *FinanceHandler {
	return &FinanceHandler{FinanceService: financeService}
}

func (h *FinanceHandler) writeFinanceError(c *gin.Context, err error) {
	writeLifeOSError(c, err, service.ErrFinanceNotFound, "finance.not_found")
}

func (h *FinanceHandler) List(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := h.FinanceService.List(ctx, userID, service.FinanceListQuery{
		Kind:     strings.TrimSpace(c.Query("kind")),
		Category: strings.TrimSpace(c.Query("category")),
		From:     strings.TrimSpace(c.Query("from")),
		To:       strings.TrimSpace(c.Query("to")),
		Page:     parsePositiveInt(c.Query("page"), defaultPage, 0),
		Limit:    parsePositiveInt(c.Query("limit"), 20, maxLimit),
	})
	if err != nil {
		h.writeFinanceError(c, err)
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

func (h *FinanceHandler) Stats(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := h.FinanceService.Stats(ctx, userID, c.Query("from"), c.Query("to"))
	if err != nil {
		h.writeFinanceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": stats})
}

func (h *FinanceHandler) Get(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := h.FinanceService.Get(ctx, userID, c.Param("id"))
	if err != nil {
		h.writeFinanceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": item})
}

func (h *FinanceHandler) Create(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	var req service.FinanceCreateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		writeInvalidRequest(c)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := h.FinanceService.Create(ctx, userID, req)
	if err != nil {
		h.writeFinanceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": i18n.T(c, "finance.created"),
		"data":    item,
	})
}

func (h *FinanceHandler) Update(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	var req service.FinanceUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		writeInvalidRequest(c)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := h.FinanceService.Update(ctx, userID, c.Param("id"), req)
	if err != nil {
		h.writeFinanceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "finance.updated"),
		"data":    item,
	})
}

func (h *FinanceHandler) Delete(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.FinanceService.Delete(ctx, userID, c.Param("id")); err != nil {
		h.writeFinanceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "finance.deleted"),
	})
}
