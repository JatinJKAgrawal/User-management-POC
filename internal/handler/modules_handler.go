package handler

import (
	"encoding/csv"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jatinjkagrawal/user-management-poc/internal/httpx"
	"github.com/jatinjkagrawal/user-management-poc/internal/middleware"
	"github.com/jatinjkagrawal/user-management-poc/internal/service"
)

// ---- Inventory ------------------------------------------------------------

type InventoryHandler struct{ svc *service.InventoryService }

func NewInventoryHandler(s *service.InventoryService) *InventoryHandler {
	return &InventoryHandler{svc: s}
}

type inventoryCreateRequest struct {
	SKU      string `json:"sku" binding:"required,min=1,max=64"`
	Name     string `json:"name" binding:"required,min=1,max=200"`
	Quantity int    `json:"quantity" binding:"min=0"`
}

type inventoryUpdateRequest struct {
	Name     string `json:"name" binding:"required,min=1,max=200"`
	Quantity int    `json:"quantity" binding:"min=0"`
}

func (h *InventoryHandler) List(c *gin.Context) {
	p := httpx.Pagination(c)
	items, total, err := h.svc.List(c.Request.Context(), middleware.MustPrincipal(c), p)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.List(c, items, total, p)
}

func (h *InventoryHandler) Get(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.Get(c.Request.Context(), middleware.MustPrincipal(c), id)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, item)
}

func (h *InventoryHandler) Create(c *gin.Context) {
	var req inventoryCreateRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	item, err := h.svc.Create(c.Request.Context(), middleware.MustPrincipal(c), req.SKU, req.Name, req.Quantity)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, item)
}

func (h *InventoryHandler) Update(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req inventoryUpdateRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	item, err := h.svc.Update(c.Request.Context(), middleware.MustPrincipal(c), id, req.Name, req.Quantity)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, item)
}

func (h *InventoryHandler) Delete(c *gin.Context) {
	id, ok := httpx.UUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), middleware.MustPrincipal(c), id); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.NoContent(c)
}

// ---- Reports --------------------------------------------------------------

type ReportHandler struct{ svc *service.ReportService }

func NewReportHandler(s *service.ReportService) *ReportHandler { return &ReportHandler{svc: s} }

func (h *ReportHandler) Summary(c *gin.Context) {
	s, err := h.svc.Summary(c.Request.Context(), middleware.MustPrincipal(c))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, s)
}

func (h *ReportHandler) ExportInventory(c *gin.Context) {
	rows, err := h.svc.ExportInventoryCSV(c.Request.Context(), middleware.MustPrincipal(c))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="inventory.csv"`)
	c.Status(http.StatusOK)
	w := csv.NewWriter(c.Writer)
	_ = w.WriteAll(rows)
}

// ---- Billing --------------------------------------------------------------

type BillingHandler struct{ svc *service.BillingService }

func NewBillingHandler(s *service.BillingService) *BillingHandler { return &BillingHandler{svc: s} }

func (h *BillingHandler) Invoices(c *gin.Context) {
	httpx.OK(c, http.StatusOK, h.svc.Invoices(middleware.MustPrincipal(c)))
}

type changePlanRequest struct {
	Plan  string `json:"plan" binding:"required"`
	Seats int    `json:"seats" binding:"required,min=1,max=10000"`
}

func (h *BillingHandler) ChangePlan(c *gin.Context) {
	var req changePlanRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	sub, err := h.svc.ChangePlan(req.Plan, req.Seats)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, sub)
}
