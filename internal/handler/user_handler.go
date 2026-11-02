package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
	"github.com/jatinjkagrawal/user-management-poc/internal/httpx"
	"github.com/jatinjkagrawal/user-management-poc/internal/middleware"
	"github.com/jatinjkagrawal/user-management-poc/internal/repository"
	"github.com/jatinjkagrawal/user-management-poc/internal/service"
)

type UserHandler struct{ users *service.UserService }

func NewUserHandler(u *service.UserService) *UserHandler { return &UserHandler{users: u} }

type inviteUserRequest struct {
	Email     string      `json:"email" binding:"required,email,max=254"`
	FirstName string      `json:"first_name" binding:"max=100"`
	LastName  string      `json:"last_name" binding:"max=100"`
	RoleIDs   []uuid.UUID `json:"role_ids"`
}

// Invite onboards a new user via email.  POST /api/v1/users
func (h *UserHandler) Invite(c *gin.Context) {
	var req inviteUserRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	res, err := h.users.Invite(c.Request.Context(), middleware.MustPrincipal(c), service.InviteUserInput{
		Email: req.Email, FirstName: req.FirstName, LastName: req.LastName, RoleIDs: req.RoleIDs,
	})
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, res)
}

// ResendInvite  POST /api/v1/users/:id/resend-invite
func (h *UserHandler) ResendInvite(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "id")
	if !ok {
		return
	}
	res, err := h.users.ResendInvite(c.Request.Context(), middleware.MustPrincipal(c), id)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, res)
}

// List  GET /api/v1/users?status=&q=&limit=&offset=
func (h *UserHandler) List(c *gin.Context) {
	p := httpx.Pagination(c)
	users, total, err := h.users.List(c.Request.Context(), middleware.MustPrincipal(c), repository.ListUsersFilter{
		Status: domain.UserStatus(c.Query("status")), Search: c.Query("q"), Page: p,
	})
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.List(c, users, total, p)
}

// Get  GET /api/v1/users/:id
func (h *UserHandler) Get(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "id")
	if !ok {
		return
	}
	u, err := h.users.Get(c.Request.Context(), middleware.MustPrincipal(c), id)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, u)
}

type updateUserRequest struct {
	FirstName string `json:"first_name" binding:"max=100"`
	LastName  string `json:"last_name" binding:"max=100"`
}

// Update  PATCH /api/v1/users/:id
func (h *UserHandler) Update(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req updateUserRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	u, err := h.users.UpdateProfile(c.Request.Context(), middleware.MustPrincipal(c), id, req.FirstName, req.LastName)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, u)
}

type userStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=enabled disabled"`
}

// SetStatus  PATCH /api/v1/users/:id/status
func (h *UserHandler) SetStatus(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req userStatusRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	u, err := h.users.SetStatus(c.Request.Context(), middleware.MustPrincipal(c), id, req.Status == "enabled")
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, u)
}

type assignRolesRequest struct {
	RoleIDs []uuid.UUID `json:"role_ids" binding:"required"`
}

// AssignRoles replaces a user's roles.  PUT /api/v1/users/:id/roles
func (h *UserHandler) AssignRoles(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req assignRolesRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	u, err := h.users.AssignRoles(c.Request.Context(), middleware.MustPrincipal(c), id, req.RoleIDs)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, u)
}

// Delete  DELETE /api/v1/users/:id
func (h *UserHandler) Delete(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.users.Delete(c.Request.Context(), middleware.MustPrincipal(c), id); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.NoContent(c)
}
