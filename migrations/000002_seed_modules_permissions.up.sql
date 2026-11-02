-- Seed the module / permission catalogue. Modules are global; which tenant
-- users may use them is decided per tenant through roles.
INSERT INTO modules (code, name, description) VALUES
    ('user_management', 'User Management', 'Manage users, roles and permissions of the tenant'),
    ('inventory',       'Inventory',       'Dummy module: manage inventory items'),
    ('reports',         'Reports',         'Dummy module: view and export reports'),
    ('billing',         'Billing',         'Dummy module: view invoices and manage subscription');

INSERT INTO permissions (module_id, action, code, description)
SELECT m.id, p.action, m.code || ':' || p.action, p.description
FROM modules m
JOIN (VALUES
    ('user_management', 'read',   'View users, roles and permissions'),
    ('user_management', 'create', 'Invite users and create roles'),
    ('user_management', 'update', 'Update users and roles, assign roles and permissions'),
    ('user_management', 'delete', 'Delete users and roles'),
    ('inventory',       'read',   'View inventory items'),
    ('inventory',       'create', 'Create inventory items'),
    ('inventory',       'update', 'Update inventory items'),
    ('inventory',       'delete', 'Delete inventory items'),
    ('reports',         'read',   'View reports'),
    ('reports',         'export', 'Export reports'),
    ('billing',         'read',   'View invoices'),
    ('billing',         'manage', 'Manage subscription')
) AS p(module_code, action, description) ON p.module_code = m.code;
