package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/sangchenglong/kapi/internal/pkg/response"
	"github.com/sangchenglong/kapi/internal/service"
)

type BillHandler struct {
	billSvc *service.BillService
}

func NewBillHandler(billSvc *service.BillService) *BillHandler {
	return &BillHandler{billSvc: billSvc}
}

type createBillRequest struct {
	BillType    string  `json:"bill_type" binding:"omitempty,oneof=expense income"`
	Amount      float64 `json:"amount" binding:"required,gt=0"`
	Category    string  `json:"category" binding:"required,max=32"`
	SubCategory string  `json:"sub_category" binding:"max=32"`
	Merchant    string  `json:"merchant" binding:"max=128"`
	Date        string  `json:"date" binding:"required,datetime=2006-01-02"`
	Note        string  `json:"note" binding:"max=255"`
	AssetID     uint64  `json:"asset_id"`
}

type updateBillRequest struct {
	Amount   float64 `json:"amount" binding:"required,gt=0"`
	Category string  `json:"category" binding:"required,max=32"`
	Merchant string  `json:"merchant" binding:"max=128"`
	Date     string  `json:"date" binding:"required,datetime=2006-01-02"`
	Note     string  `json:"note" binding:"max=255"`
}

func getUserID(c *gin.Context) (uint64, bool) {
	userID := c.GetUint64("user_id")
	if userID == 0 {
		response.Unauthorized(c, "invalid user context")
		return 0, false
	}
	return userID, true
}

func (h *BillHandler) Create(c *gin.Context) {
	var req createBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid parameters: "+err.Error())
		return
	}
	userID, ok := getUserID(c)
	if !ok {
		return
	}
	bill, err := h.billSvc.Create(c.Request.Context(), userID, req.BillType, req.Amount, req.Category, req.SubCategory, req.Merchant, req.Date, req.Note, req.AssetID)
	if err != nil {
		response.InternalError(c, "create bill failed")
		return
	}
	response.Success(c, bill)
}

func (h *BillHandler) List(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}
	bills, err := h.billSvc.List(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, "list bills failed")
		return
	}
	response.Success(c, bills)
}

func (h *BillHandler) Update(c *gin.Context) {
	var req updateBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid parameters: "+err.Error())
		return
	}
	userID, ok := getUserID(c)
	if !ok {
		return
	}
	billID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid bill id")
		return
	}
	bill, err := h.billSvc.Update(c.Request.Context(), userID, billID, req.Amount, req.Category, req.Merchant, req.Date, req.Note)
	if err != nil {
		if errors.Is(err, service.ErrBillNotFound) {
			response.NotFound(c, "bill not found")
			return
		}
		response.InternalError(c, "update bill failed")
		return
	}
	response.Success(c, bill)
}

func (h *BillHandler) Delete(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}
	billID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid bill id")
		return
	}
	if err := h.billSvc.Delete(c.Request.Context(), userID, billID); err != nil {
		if errors.Is(err, service.ErrBillNotFound) {
			response.NotFound(c, "bill not found")
			return
		}
		response.InternalError(c, "delete bill failed")
		return
	}
	response.Success(c, nil)
}
