package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jatinjkagrawal/user-management-poc/internal/httpx"
	"github.com/jatinjkagrawal/user-management-poc/internal/middleware"
	"github.com/jatinjkagrawal/user-management-poc/internal/service"
)

type RoleHandler struct{ roles *service.RoleService }

func NewRoleHandler(r *service.RoleService) *RoleHandler { return &RoleHandler{roles: r} }

// ListModules returns every module with its permissions.  GET /api/v1/modules
func (h *RoleHandler) ListModules(c *gin.Context) {
	mods, err := h.roles.ListModules(c.Request.Context())
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, mods)
}

// List  GET /api/v1/roles
func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.roles.List(c.Request.Context(), middleware.MustPrincipal(c))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, roles)
}

// Get  GET /api/v1/roles/:id
func (h *RoleHandler) Get(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "id")
	if !ok {
		return
	}
	r, err := h.roles.Get(c.Request.Context(), middleware.MustPrincipal(c), id)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, r)
}

type createRoleRequest struct {
	Name        string   `json:"name" binding:"required,min=2,max=64"`
	Description string   `json:"description" binding:"max=255"`
	Permissions []string `json:"permissions"`
}

// Create  POST /api/v1/roles
func (h *RoleHandler) Create(c *gin.Context) {
	var req createRoleRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	r, err := h.roles.Create(c.Request.Context(), middleware.MustPrincipal(c), service.RoleInput{
		Name: req.Name, Description: req.Description, Permissions: req.Permissions,
	})
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, r)
}

type updateRoleRequest struct {
	Name        string `json:"name" binding:"required,min=2,max=64"`
	Description string `json:"description" binding:"max=255"`
}

// Update  PUT /api/v1/roles/:id
func (h *RoleHandler) Update(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req updateRoleRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	r, err := h.roles.Update(c.Request.Context(), middleware.MustPrincipal(c), id, req.Name, req.Description)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, r)
}

type setPermissionsRequest struct {
	Permissions []string `json:"permissions" binding:"required"`
}

// SetPermissions replaces a role's permissions.  PUT /api/v1/roles/:id/permissions
func (h *RoleHandler) SetPermissions(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req setPermissionsRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	r, err := h.roles.SetPermissions(c.Request.Context(), middleware.MustPrincipal(c), id, req.Permissions)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, r)
}

// Delete  DELETE /api/v1/roles/:id
func (h *RoleHandler) Delete(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.roles.Delete(c.Request.Context(), middleware.MustPrincipal(c), id); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.NoContent(c)
}
