package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/sangchenglong/kapi/internal/pkg/response"
	"github.com/sangchenglong/kapi/internal/service"
)

type BudgetHandler struct {
	budgetSvc *service.BudgetService
}

func NewBudgetHandler(budgetSvc *service.BudgetService) *BudgetHandler {
	return &BudgetHandler{budgetSvc: budgetSvc}
}

type createBudgetRequest struct {
	Name       string  `json:"name" binding:"max=64"`
	Category   string  `json:"category" binding:"max=32"`
	Categories string  `json:"categories"`
	Amount     float64 `json:"amount" binding:"required,gt=0"`
	Period     string  `json:"period" binding:"required,oneof=monthly weekly"`
	BudgetType string  `json:"budget_type" binding:"omitempty,oneof=optional essential"`
}

type updateBudgetRequest struct {
	Name       string  `json:"name" binding:"max=64"`
	Category   string  `json:"category" binding:"max=32"`
	Categories string  `json:"categories"`
	Amount     float64 `json:"amount" binding:"required,gt=0"`
	Period     string  `json:"period" binding:"required,oneof=monthly weekly"`
	BudgetType string  `json:"budget_type" binding:"omitempty,oneof=optional essential"`
}

func (h *BudgetHandler) Create(c *gin.Context) {
	var req createBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid parameters: "+err.Error())
		return
	}
	userID, ok := getUserID(c)
	if !ok {
		return
	}
	budget, err := h.budgetSvc.Create(c.Request.Context(), userID, req.Name, req.Categories, req.Amount, req.Period, req.BudgetType)
	if err != nil {
		response.InternalError(c, "create budget failed")
		return
	}
	response.Success(c, budget)
}

func (h *BudgetHandler) List(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}
	budgets, err := h.budgetSvc.List(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, "list budgets failed")
		return
	}
	response.Success(c, budgets)
}

func (h *BudgetHandler) Update(c *gin.Context) {
	var req updateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid parameters: "+err.Error())
		return
	}
	userID, ok := getUserID(c)
	if !ok {
		return
	}
	budgetID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid budget id")
		return
	}
	budget, err := h.budgetSvc.Update(c.Request.Context(), userID, budgetID, req.Name, req.Categories, req.Amount, req.Period, req.BudgetType)
	if err != nil {
		if errors.Is(err, service.ErrBudgetNotFound) {
			response.NotFound(c, "budget not found")
			return
		}
		response.InternalError(c, "update budget failed")
		return
	}
	response.Success(c, budget)
}
