package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/sangchenglong/kapi/internal/pkg/response"
	"github.com/sangchenglong/kapi/internal/service"
)

// SuggestionHandler 处理智能建议相关的 HTTP 请求
type SuggestionHandler struct {
	svc *service.SuggestionService
}

// NewSuggestionHandler 创建 SuggestionHandler 实例
func NewSuggestionHandler(svc *service.SuggestionService) *SuggestionHandler {
	return &SuggestionHandler{svc: svc}
}

// GetSuggestions 获取当前用户的智能建议列表
func (h *SuggestionHandler) GetSuggestions(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	suggestions, err := h.svc.GetSuggestions(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, "get suggestions failed")
		return
	}

	response.Success(c, suggestions)
}
