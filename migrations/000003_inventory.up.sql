-- Tables backing the dummy "inventory" module (tenant scoped).
CREATE TABLE inventory_items (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    sku         CITEXT      NOT NULL,
    name        TEXT        NOT NULL,
    quantity    INTEGER     NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    created_by  UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_inventory_items_tenant_sku UNIQUE (tenant_id, sku)
);
CREATE INDEX idx_inventory_items_tenant ON inventory_items (tenant_id, created_at DESC);
CREATE TRIGGER trg_inventory_items_updated_at BEFORE UPDATE ON inventory_items
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
