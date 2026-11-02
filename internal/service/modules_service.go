package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jatinjkagrawal/user-management-poc/internal/apperror"
	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
	"github.com/jatinjkagrawal/user-management-poc/internal/repository"
)

// ---------------------------------------------------------------------------
// Inventory (dummy module backed by a tenant-scoped table)
// ---------------------------------------------------------------------------

type InventoryService struct{ d Deps }

func (s *InventoryService) List(ctx context.Context, actor *domain.Principal, p domain.Page) ([]domain.InventoryItem, int, error) {
	return s.d.Store.Inventory.List(ctx, actor.TenantID, p)
}

func (s *InventoryService) Get(ctx context.Context, actor *domain.Principal, id uuid.UUID) (*domain.InventoryItem, error) {
	i, err := s.d.Store.Inventory.Get(ctx, actor.TenantID, id)
	return i, mapNotFound(err, "item_not_found", "inventory item not found")
}

func (s *InventoryService) Create(ctx context.Context, actor *domain.Principal, sku, name string, qty int) (*domain.InventoryItem, error) {
	i, err := s.d.Store.Inventory.Create(ctx, actor.TenantID, strings.TrimSpace(sku), strings.TrimSpace(name), qty, actor.UserID)
	if repository.IsUniqueViolation(err, "uq_inventory_items_tenant_sku") {
		return nil, apperror.Conflict("sku_exists", "an item with this SKU already exists")
	}
	return i, err
}

func (s *InventoryService) Update(ctx context.Context, actor *domain.Principal, id uuid.UUID, name string, qty int) (*domain.InventoryItem, error) {
	i, err := s.d.Store.Inventory.Update(ctx, actor.TenantID, id, strings.TrimSpace(name), qty)
	return i, mapNotFound(err, "item_not_found", "inventory item not found")
}

func (s *InventoryService) Delete(ctx context.Context, actor *domain.Principal, id uuid.UUID) error {
	return mapNotFound(s.d.Store.Inventory.Delete(ctx, actor.TenantID, id), "item_not_found", "inventory item not found")
}

// ---------------------------------------------------------------------------
// Reports (dummy module computing aggregates over tenant data)
// ---------------------------------------------------------------------------

type ReportService struct{ d Deps }

type Summary struct {
	TenantID    uuid.UUID                 `json:"tenant_id"`
	GeneratedAt time.Time                 `json:"generated_at"`
	Users       map[domain.UserStatus]int `json:"users_by_status"`
	Roles       int                       `json:"roles"`
	Inventory   repository.InventoryStats `json:"inventory"`
}

func (s *ReportService) Summary(ctx context.Context, actor *domain.Principal) (*Summary, error) {
	users, err := s.d.Store.Users.Count(ctx, actor.TenantID)
	if err != nil {
		return nil, err
	}
	roles, err := s.d.Store.Roles.Count(ctx, actor.TenantID)
	if err != nil {
		return nil, err
	}
	inv, err := s.d.Store.Inventory.Stats(ctx, actor.TenantID)
	if err != nil {
		return nil, err
	}
	return &Summary{TenantID: actor.TenantID, GeneratedAt: time.Now().UTC(), Users: users, Roles: roles, Inventory: inv}, nil
}

// ExportInventoryCSV renders the tenant's inventory as CSV rows.
func (s *ReportService) ExportInventoryCSV(ctx context.Context, actor *domain.Principal) ([][]string, error) {
	items, _, err := s.d.Store.Inventory.List(ctx, actor.TenantID, domain.Page{Limit: 10000})
	if err != nil {
		return nil, err
	}
	rows := [][]string{{"id", "sku", "name", "quantity", "created_at"}}
	for _, i := range items {
		rows = append(rows, []string{i.ID.String(), i.SKU, i.Name, fmt.Sprint(i.Quantity), i.CreatedAt.UTC().Format(time.RFC3339)})
	}
	return rows, nil
}

// ---------------------------------------------------------------------------
// Billing (dummy module returning static data)
// ---------------------------------------------------------------------------

type BillingService struct{}

type Invoice struct {
	ID       string    `json:"id"`
	Amount   float64   `json:"amount"`
	Currency string    `json:"currency"`
	Status   string    `json:"status"`
	IssuedAt time.Time `json:"issued_at"`
}

func (s *BillingService) Invoices(actor *domain.Principal) []Invoice {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	prefix := strings.ToUpper(actor.TenantID.String()[:8])
	out := make([]Invoice, 0, 3)
	for i := 0; i < 3; i++ {
		out = append(out, Invoice{
			ID: fmt.Sprintf("INV-%s-%03d", prefix, i+1), Amount: 49.00, Currency: "USD",
			Status: "paid", IssuedAt: base.AddDate(0, i, 0),
		})
	}
	return out
}

type Subscription struct {
	Plan      string    `json:"plan"`
	Seats     int       `json:"seats"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *BillingService) ChangePlan(plan string, seats int) (*Subscription, error) {
	switch plan {
	case "free", "pro", "enterprise":
	default:
		return nil, apperror.Validation("invalid_plan", "plan must be one of free, pro, enterprise")
	}
	return &Subscription{Plan: plan, Seats: seats, UpdatedAt: time.Now().UTC()}, nil
}
