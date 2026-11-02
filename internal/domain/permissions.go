package domain

// Permission codes seeded by migrations/000002_seed_modules_permissions.up.sql.
// Format: "<module>:<action>".
const (
	PermUserMgmtRead   = "user_management:read"
	PermUserMgmtCreate = "user_management:create"
	PermUserMgmtUpdate = "user_management:update"
	PermUserMgmtDelete = "user_management:delete"

	PermInventoryRead   = "inventory:read"
	PermInventoryCreate = "inventory:create"
	PermInventoryUpdate = "inventory:update"
	PermInventoryDelete = "inventory:delete"

	PermReportsRead   = "reports:read"
	PermReportsExport = "reports:export"

	PermBillingRead   = "billing:read"
	PermBillingManage = "billing:manage"
)
