package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/sangchenglong/kapi/internal/pkg/response"
	"github.com/sangchenglong/kapi/internal/service"
)

type AssetHandler struct {
	assetSvc *service.AssetService
}

func NewAssetHandler(assetSvc *service.AssetService) *AssetHandler {
	return &AssetHandler{assetSvc: assetSvc}
}

type createAssetRequest struct {
	Name    string  `json:"name" binding:"required,max=64"`
	Type    string  `json:"type" binding:"required,oneof=cash credit investment debt"`
	Balance float64 `json:"balance"`
}

type updateAssetRequest struct {
	Name    string  `json:"name" binding:"required,max=64"`
	Type    string  `json:"type" binding:"required,oneof=cash credit investment debt"`
	Balance float64 `json:"balance"`
}

func (h *AssetHandler) Create(c *gin.Context) {
	var req createAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid parameters: "+err.Error())
		return
	}
	userID, ok := getUserID(c)
	if !ok {
		return
	}
	asset, err := h.assetSvc.Create(c.Request.Context(), userID, req.Name, req.Type, req.Balance)
	if err != nil {
		response.InternalError(c, "create asset failed")
		return
	}
	response.Success(c, asset)
}

func (h *AssetHandler) List(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}
	assets, err := h.assetSvc.List(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, "list assets failed")
		return
	}
	response.Success(c, assets)
}

func (h *AssetHandler) Update(c *gin.Context) {
	var req updateAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid parameters: "+err.Error())
		return
	}
	userID, ok := getUserID(c)
	if !ok {
		return
	}
	assetID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid asset id")
		return
	}
	asset, err := h.assetSvc.Update(c.Request.Context(), userID, assetID, req.Name, req.Type, req.Balance)
	if err != nil {
		if errors.Is(err, service.ErrAssetNotFound) {
			response.NotFound(c, "asset not found")
			return
		}
		response.InternalError(c, "update asset failed")
		return
	}
	response.Success(c, asset)
}
