package handler

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/sangchenglong/kapi/internal/pkg/response"
	"github.com/sangchenglong/kapi/internal/service"
)

type AuthHandler struct {
	authSvc *service.AuthService
}

func NewAuthHandler(authSvc *service.AuthService) *AuthHandler {
	return &AuthHandler{authSvc: authSvc}
}

type registerRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=128"`
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid parameters: "+err.Error())
		return
	}

	user, err := h.authSvc.Register(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrUserExists) {
			response.Error(c, 409, 42001, "user already exists")
			return
		}
		response.InternalError(c, "registration failed")
		return
	}

	response.Success(c, gin.H{
		"id":       user.ID,
		"username": user.Username,
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid parameters: "+err.Error())
		return
	}

	token, err := h.authSvc.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) || errors.Is(err, service.ErrWrongPassword) {
			response.Unauthorized(c, "invalid username or password")
			return
		}
		response.InternalError(c, "login failed")
		return
	}

	response.Success(c, gin.H{
		"token":    token,
		"username": req.Username,
	})
}
