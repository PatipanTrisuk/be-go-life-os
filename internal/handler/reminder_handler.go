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

type ReminderHandler struct {
	ReminderService *service.ReminderService
}

func NewReminderHandler(reminderService *service.ReminderService) *ReminderHandler {
	return &ReminderHandler{ReminderService: reminderService}
}

func (h *ReminderHandler) writeReminderError(c *gin.Context, err error) {
	writeLifeOSError(c, err, service.ErrReminderNotFound, "reminder.not_found")
}

func (h *ReminderHandler) List(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	items, err := h.ReminderService.List(ctx, userID, strings.TrimSpace(c.Query("status")))
	if err != nil {
		h.writeReminderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": items})
}

func (h *ReminderHandler) Get(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := h.ReminderService.Get(ctx, userID, c.Param("id"))
	if err != nil {
		h.writeReminderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": item})
}

func (h *ReminderHandler) Create(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	var req service.ReminderCreateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		writeInvalidRequest(c)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := h.ReminderService.Create(ctx, userID, req)
	if err != nil {
		h.writeReminderError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": i18n.T(c, "reminder.created"),
		"data":    item,
	})
}

func (h *ReminderHandler) Update(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	var req service.ReminderUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		writeInvalidRequest(c)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := h.ReminderService.Update(ctx, userID, c.Param("id"), req)
	if err != nil {
		h.writeReminderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "reminder.updated"),
		"data":    item,
	})
}

func (h *ReminderHandler) MarkDone(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := h.ReminderService.MarkDone(ctx, userID, c.Param("id"))
	if err != nil {
		h.writeReminderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "reminder.done"),
		"data":    item,
	})
}

func (h *ReminderHandler) Check(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := h.ReminderService.Check(ctx, userID)
	if err != nil {
		h.writeReminderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "reminder.checked"),
		"data":    result,
	})
}

func (h *ReminderHandler) Delete(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.ReminderService.Delete(ctx, userID, c.Param("id")); err != nil {
		h.writeReminderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "reminder.deleted"),
	})
}
