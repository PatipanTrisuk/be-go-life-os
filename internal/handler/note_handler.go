package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"go-be/internal/i18n"
	"go-be/internal/repository"
	"go-be/internal/service"
)

type NoteHandler struct {
	NoteService *service.NoteService
}

func NewNoteHandler(noteService *service.NoteService) *NoteHandler {
	return &NoteHandler{NoteService: noteService}
}

func (h *NoteHandler) writeNoteError(c *gin.Context, err error) {
	writeLifeOSError(c, err, service.ErrNoteNotFound, "note.not_found")
}

func (h *NoteHandler) List(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := h.NoteService.List(ctx, userID, repository.NoteListFilter{
		Kind:   strings.TrimSpace(c.Query("kind")),
		Search: strings.TrimSpace(c.Query("q")),
		Page:   parsePositiveInt(c.Query("page"), defaultPage, 0),
		Limit:  parsePositiveInt(c.Query("limit"), 20, maxLimit),
	})
	if err != nil {
		h.writeNoteError(c, err)
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

func (h *NoteHandler) Stats(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := h.NoteService.Stats(ctx, userID)
	if err != nil {
		h.writeNoteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": stats})
}

func (h *NoteHandler) Get(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := h.NoteService.Get(ctx, userID, c.Param("id"))
	if err != nil {
		h.writeNoteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": item})
}

func (h *NoteHandler) Create(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	var req service.NoteCreateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		writeInvalidRequest(c)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := h.NoteService.Create(ctx, userID, req)
	if err != nil {
		h.writeNoteError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": i18n.T(c, "note.created"),
		"data":    item,
	})
}

func (h *NoteHandler) Update(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	var req service.NoteUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		writeInvalidRequest(c)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := h.NoteService.Update(ctx, userID, c.Param("id"), req)
	if err != nil {
		h.writeNoteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "note.updated"),
		"data":    item,
	})
}

func (h *NoteHandler) Delete(c *gin.Context) {
	userID, ok := requireActor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.NoteService.Delete(ctx, userID, c.Param("id")); err != nil {
		h.writeNoteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": i18n.T(c, "note.deleted"),
	})
}
