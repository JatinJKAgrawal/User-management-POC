-- Core schema: tenants, users, RBAC, auth tokens.
CREATE EXTENSION IF NOT EXISTS citext;

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- Tenants
-- ---------------------------------------------------------------------------
CREATE TABLE tenants (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    slug        CITEXT      NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    status      TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_tenants_updated_at BEFORE UPDATE ON tenants
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Users (always scoped to exactly one tenant)
-- ---------------------------------------------------------------------------
CREATE TABLE users (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    email              CITEXT      NOT NULL,
    first_name         TEXT        NOT NULL DEFAULT '',
    last_name          TEXT        NOT NULL DEFAULT '',
    password_hash      TEXT,
    status             TEXT        NOT NULL DEFAULT 'invited'
                                   CHECK (status IN ('invited', 'pending_password', 'active', 'disabled')),
    email_verified_at  TIMESTAMPTZ,
    password_set_at    TIMESTAMPTZ,
    last_login_at      TIMESTAMPTZ,
    invited_by         UUID,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- The same email may exist in several tenants, but only once per tenant.
    CONSTRAINT uq_users_tenant_email UNIQUE (tenant_id, email),
    -- Target for composite FKs that enforce tenant isolation at DB level.
    CONSTRAINT uq_users_id_tenant UNIQUE (id, tenant_id),
    CONSTRAINT fk_users_invited_by FOREIGN KEY (invited_by, tenant_id)
        REFERENCES users (id, tenant_id) ON DELETE SET NULL (invited_by)
);
CREATE INDEX idx_users_tenant_status ON users (tenant_id, status);
CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Modules & permissions (global catalogue, seeded by migrations)
-- ---------------------------------------------------------------------------
CREATE TABLE modules (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    code        TEXT        NOT NULL UNIQUE,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE permissions (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    module_id   UUID        NOT NULL REFERENCES modules (id) ON DELETE CASCADE,
    action      TEXT        NOT NULL,
    code        TEXT        NOT NULL UNIQUE, -- "<module>:<action>"
    description TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_permissions_module_action UNIQUE (module_id, action)
);

-- ---------------------------------------------------------------------------
-- Roles (tenant scoped)
-- ---------------------------------------------------------------------------
CREATE TABLE roles (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name        CITEXT      NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    is_system   BOOLEAN     NOT NULL DEFAULT FALSE, -- system roles are immutable
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_roles_tenant_name UNIQUE (tenant_id, name),
    CONSTRAINT uq_roles_id_tenant UNIQUE (id, tenant_id)
);
CREATE TRIGGER trg_roles_updated_at BEFORE UPDATE ON roles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE role_permissions (
    role_id       UUID        NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    permission_id UUID        NOT NULL REFERENCES permissions (id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (role_id, permission_id)
);
CREATE INDEX idx_role_permissions_permission ON role_permissions (permission_id);

-- Composite FKs guarantee a user can only ever hold roles of their own tenant.
CREATE TABLE user_roles (
    tenant_id   UUID        NOT NULL,
    user_id     UUID        NOT NULL,
    role_id     UUID        NOT NULL,
    assigned_by UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id),
    CONSTRAINT fk_user_roles_user FOREIGN KEY (user_id, tenant_id)
        REFERENCES users (id, tenant_id) ON DELETE CASCADE,
    CONSTRAINT fk_user_roles_role FOREIGN KEY (role_id, tenant_id)
        REFERENCES roles (id, tenant_id) ON DELETE CASCADE
);
CREATE INDEX idx_user_roles_role ON user_roles (role_id);
CREATE INDEX idx_user_roles_tenant ON user_roles (tenant_id);

-- ---------------------------------------------------------------------------
-- One-time action tokens (onboarding links). Only a SHA-256 hash is stored.
-- ---------------------------------------------------------------------------
CREATE TABLE user_action_tokens (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL,
    user_id     UUID        NOT NULL,
    purpose     TEXT        NOT NULL CHECK (purpose IN ('onboarding')),
    token_hash  TEXT        NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_user_action_tokens_user FOREIGN KEY (user_id, tenant_id)
        REFERENCES users (id, tenant_id) ON DELETE CASCADE
);
CREATE INDEX idx_user_action_tokens_user ON user_action_tokens (user_id, purpose);

-- ---------------------------------------------------------------------------
-- Refresh token sessions. The refresh JWT's "jti" is the row id, which lets us
-- revoke / rotate refresh tokens and detect reuse.
-- ---------------------------------------------------------------------------
CREATE TABLE refresh_tokens (
    id          UUID        PRIMARY KEY,
    tenant_id   UUID        NOT NULL,
    user_id     UUID        NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    replaced_by UUID,
    user_agent  TEXT        NOT NULL DEFAULT '',
    ip_address  TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_refresh_tokens_user FOREIGN KEY (user_id, tenant_id)
        REFERENCES users (id, tenant_id) ON DELETE CASCADE
);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens (user_id) WHERE revoked_at IS NULL;
