package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
	"github.com/jatinjkagrawal/user-management-poc/internal/httpx"
	"github.com/jatinjkagrawal/user-management-poc/internal/service"
)

// PlatformHandler exposes super-admin tenant provisioning, protected by the
// platform API key rather than tenant JWTs.
type PlatformHandler struct{ tenants *service.TenantService }

func NewPlatformHandler(t *service.TenantService) *PlatformHandler {
	return &PlatformHandler{tenants: t}
}

type createTenantRequest struct {
	Name  string `json:"name" binding:"required,min=2,max=100"`
	Slug  string `json:"slug" binding:"required,min=2,max=63"`
	Admin struct {
		Email     string `json:"email" binding:"required,email,max=254"`
		FirstName string `json:"first_name" binding:"max=100"`
		LastName  string `json:"last_name" binding:"max=100"`
	} `json:"admin" binding:"required"`
}

func (h *PlatformHandler) CreateTenant(c *gin.Context) {
	var req createTenantRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	res, err := h.tenants.Create(c.Request.Context(), service.CreateTenantInput{
		Name: req.Name, Slug: req.Slug,
		AdminEmail: req.Admin.Email, AdminFirstName: req.Admin.FirstName, AdminLastName: req.Admin.LastName,
	})
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, res)
}

func (h *PlatformHandler) ListTenants(c *gin.Context) {
	p := httpx.Pagination(c)
	tenants, total, err := h.tenants.List(c.Request.Context(), p)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.List(c, tenants, total, p)
}

func (h *PlatformHandler) GetTenant(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "id")
	if !ok {
		return
	}
	t, err := h.tenants.Get(c.Request.Context(), id)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, t)
}

type tenantStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active suspended"`
}

func (h *PlatformHandler) SetTenantStatus(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req tenantStatusRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	t, err := h.tenants.SetStatus(c.Request.Context(), id, domain.TenantStatus(req.Status))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, t)
}
