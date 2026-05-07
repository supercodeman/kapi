package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/sangchenglong/kapi/internal/engine"
	"github.com/sangchenglong/kapi/internal/pkg/response"
)

type ChatHandler struct {
	engine *engine.Engine
}

func NewChatHandler(eng *engine.Engine) *ChatHandler {
	return &ChatHandler{engine: eng}
}

type chatRequest struct {
	Message     string `json:"message" binding:"required"`
	PageContext string `json:"page_context" binding:"required"`
	SessionID   uint64 `json:"session_id"`
}

func (h *ChatHandler) Chat(c *gin.Context) {
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid parameters: "+err.Error())
		return
	}

	userID, ok := getUserID(c)
	if !ok {
		return
	}

	chatReq := &engine.ChatRequest{
		UserID:      userID,
		SessionID:   req.SessionID,
		Message:     req.Message,
		PageContext:  req.PageContext,
	}

	resp, err := h.engine.Chat(c.Request.Context(), chatReq)
	if err != nil {
		response.InternalError(c, "chat failed")
		return
	}

	response.Success(c, resp)
}

// ChatStream SSE 流式对话端点
func (h *ChatHandler) ChatStream(c *gin.Context) {
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid parameters"})
		return
	}

	userID, ok := getUserID(c)
	if !ok {
		return
	}

	chatReq := &engine.ChatRequest{
		UserID:      userID,
		SessionID:   req.SessionID,
		Message:     req.Message,
		PageContext:  req.PageContext,
	}

	ch, err := h.engine.ChatStream(c.Request.Context(), chatReq)
	if err != nil {
		c.JSON(500, gin.H{"error": "stream init failed"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	c.Stream(func(w io.Writer) bool {
		event, ok := <-ch
		if !ok {
			return false
		}
		data, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", data)
		if f, ok := w.(interface{ Flush() }); ok {
			f.Flush()
		}
		return true
	})
}

// GetHistory 获取对话历史记录
func (h *ChatHandler) GetHistory(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	sessionIDStr := c.DefaultQuery("session_id", "1")
	sessionID, err := strconv.ParseUint(sessionIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid session_id")
		return
	}

	limitStr := c.DefaultQuery("limit", "20")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		limit = 20
	}

	messages, err := h.engine.GetConversationHistory(c.Request.Context(), userID, sessionID, limit)
	if err != nil {
		response.InternalError(c, "get history failed")
		return
	}

	response.Success(c, messages)
}
