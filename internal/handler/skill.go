package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
	"github.com/sangchenglong/kapi/internal/pkg/response"
)

type SkillHandler struct {
	skillDAO *dao.SkillDAO
}

func NewSkillHandler(skillDAO *dao.SkillDAO) *SkillHandler {
	return &SkillHandler{skillDAO: skillDAO}
}

func (h *SkillHandler) List(c *gin.Context) {
	skills, err := h.skillDAO.List(c.Request.Context())
	if err != nil {
		response.InternalError(c, "list skills failed")
		return
	}
	response.Success(c, skills)
}

func (h *SkillHandler) Get(c *gin.Context) {
	name := c.Param("name")
	s, err := h.skillDAO.GetByName(c.Request.Context(), name)
	if err != nil {
		response.BadRequest(c, "skill not found")
		return
	}
	response.Success(c, s)
}

type createSkillRequest struct {
	Name           string `json:"name" binding:"required,max=64"`
	Page           string `json:"page" binding:"required"`
	Type           string `json:"type" binding:"required,oneof=read write"`
	TriggerDesc    string `json:"trigger_desc" binding:"required"`
	ParamsSchema   string `json:"params_schema"`
	PromptTemplate string `json:"prompt_template"`
	Version        string `json:"version" binding:"required"`
}

func (h *SkillHandler) Create(c *gin.Context) {
	var req createSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid parameters: "+err.Error())
		return
	}

	if req.ParamsSchema == "" {
		req.ParamsSchema = "[]"
	}

	s := &model.Skill{
		Name:           req.Name,
		Page:           req.Page,
		Type:           req.Type,
		TriggerDesc:    req.TriggerDesc,
		ParamsSchema:   req.ParamsSchema,
		PromptTemplate: req.PromptTemplate,
		Status:         "enabled",
		Version:        req.Version,
	}
	if err := h.skillDAO.Create(c.Request.Context(), s); err != nil {
		response.InternalError(c, "create skill failed")
		return
	}
	response.Success(c, s)
}

type updateSkillRequest struct {
	Page           string `json:"page"`
	Type           string `json:"type" binding:"omitempty,oneof=read write"`
	TriggerDesc    string `json:"trigger_desc"`
	ParamsSchema   string `json:"params_schema"`
	PromptTemplate string `json:"prompt_template"`
	Status         string `json:"status" binding:"omitempty,oneof=enabled disabled"`
	Version        string `json:"version"`
}

func (h *SkillHandler) Update(c *gin.Context) {
	name := c.Param("name")
	s, err := h.skillDAO.GetByName(c.Request.Context(), name)
	if err != nil {
		response.BadRequest(c, "skill not found")
		return
	}

	var req updateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid parameters: "+err.Error())
		return
	}

	if req.Page != "" {
		s.Page = req.Page
	}
	if req.Type != "" {
		s.Type = req.Type
	}
	if req.TriggerDesc != "" {
		s.TriggerDesc = req.TriggerDesc
	}
	if req.ParamsSchema != "" {
		s.ParamsSchema = req.ParamsSchema
	}
	if req.PromptTemplate != "" {
		s.PromptTemplate = req.PromptTemplate
	}
	if req.Status != "" {
		s.Status = req.Status
	}
	if req.Version != "" {
		s.Version = req.Version
	}

	if err := h.skillDAO.Update(c.Request.Context(), s); err != nil {
		response.InternalError(c, "update skill failed")
		return
	}
	response.Success(c, s)
}

func (h *SkillHandler) Delete(c *gin.Context) {
	name := c.Param("name")
	if err := h.skillDAO.Delete(c.Request.Context(), name); err != nil {
		response.InternalError(c, "delete skill failed")
		return
	}
	response.Success(c, gin.H{"deleted": name})
}
