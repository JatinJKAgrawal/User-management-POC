# User Management POC

A multi-tenant user management backend in Go (Gin + PostgreSQL). It covers:

- Tenant admins onboard users by email.
- Users verify their email from the link, set a password, then log in.
- JWT authentication: access, refresh, and password-setup tokens.
- RBAC: admins create roles, attach module permissions to them and assign them to users.
- Each tenant's data is kept separate in both the application and the database.

## Quick start (Docker)

```bash
cp .env.example .env        # optional: override secrets/ports
docker compose up -d --build
```

| Service    | URL                     | Purpose                                         |
|------------|-------------------------|-------------------------------------------------|
| `api`      | http://localhost:8080   | The API                                         |
| `postgres` | localhost:5432          | PostgreSQL 16 (`app` / `app`, db `usermgmt`)    |
| `mailhog`  | http://localhost:8025   | Catches outgoing email (onboarding links)       |
| `migrate`  | one-shot                | Applies migrations, then exits before `api` starts |

Run the end-to-end smoke test against the running stack:

```bash
PLATFORM_API_KEY=change-me-platform-api-key ./scripts/smoke_test.sh
```

It checks onboarding, login, RBAC, privilege-escalation guards, tenant isolation and session revocation.

### Local development (without Docker for the app)

```bash
docker compose up -d postgres mailhog
cp .env.example .env
make migrate-up
make run
make test        # unit tests
make lint
```

## Project layout

```
cmd/
  api/                  HTTP server entrypoint (config, DI, graceful shutdown)
  migrate/              Migration CLI (up / down N / version / force V), migrations embedded
migrations/             Versioned SQL migrations (golang-migrate format) + embed.go
internal/
  config/               Env-based configuration with validation
  domain/               Entities, statuses, permission codes, Principal
  apperror/             Typed errors -> HTTP status mapping
  auth/                 JWT manager, bcrypt passwords + policy, opaque one-time tokens
  repository/           PostgreSQL repositories (pgx); every tenant query takes tenant_id
  service/              Business logic (onboarding, auth, users, roles, tenants, modules)
  handler/              Gin handlers: request DTOs/validation -> service calls
  middleware/           Request ID, logging, recovery, security headers, JWT auth, RBAC
  httpx/                JSON envelope, error rendering, binding, pagination
  server/               Router (routes + permissions) and HTTP server lifecycle
  platform/
    database/           pgx pool
    logger/             slog setup
    mailer/             SMTP / log mailer + invitation template
scripts/smoke_test.sh   End-to-end test of all flows
Dockerfile              Multi-stage build -> distroless, non-root (api + migrate binaries)
docker-compose.yml      postgres, mailhog, migrate, api
```

Dependencies only point inward: handler → service → repository → database. Services never see HTTP, and handlers never see SQL.

## Flows

### 1. Tenant provisioning (platform super-admin)

Platform endpoints are protected by the `X-Platform-API-Key` header, not by a tenant JWT.

```bash
curl -X POST localhost:8080/api/v1/platform/tenants \
  -H 'X-Platform-API-Key: change-me-platform-api-key' -H 'Content-Type: application/json' \
  -d '{"name":"Acme Inc","slug":"acme","admin":{"email":"admin@acme.test","first_name":"Ada"}}'
```

This does the following in a single transaction:
1. Creates the tenant.
2. Creates the built-in **Tenant Admin** system role, which holds every permission and cannot be modified.
3. Creates the admin user in the `invited` state and gives them that role.
4. Issues a one-time onboarding token. The email is sent after the transaction commits.

### 2. Onboarding: invite → verify → set password

```
invited ──(click email link)──▶ pending_password ──(set password)──▶ active
```

1. **Invite**: `POST /api/v1/users` (needs `user_management:create`). The user gets an email with
   `GET /api/v1/auth/onboarding/verify?token=<opaque>`. Only a SHA-256 hash of the token is stored. It expires after 72h by default, and a resend makes the old link stop working.
2. **Verify**: clicking the link uses up the token, marks the email as verified and creates a short-lived (30m) **password-setup JWT**.
   - If `ONBOARDING_SET_PASSWORD_URL` is set, browsers get a `302` redirect to `<url>?token=<setup-jwt>&email=...`. On failure the redirect is `<url>?error=<code>`.
   - Clients sending `Accept: application/json`, or any client when no URL is set, get `{"data":{"setup_token":...}}`.
3. **Set password**: `POST /api/v1/auth/onboarding/set-password`
   `{"token":"<setup-jwt>","password":"...","confirm_password":"..."}`.
   This only works while the user is `pending_password`, so each setup token can be used once.
   Password policy: 8–72 characters, with upper-case, lower-case, digit and special characters.

### 3. Login & sessions

```bash
curl -X POST localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"tenant":"acme","email":"admin@acme.test","password":"Str0ng!Passw0rd"}'
```

Login returns an access token (15m) and a refresh token (7d). Both are HS256 JWTs.

- **Token types** use separate `typ` claims and audiences (`um:access`, `um:refresh`, `um:password_setup`). This stops one kind of token being accepted in place of another. The parser also checks the algorithm, issuer, expiry and iat.
- **Refresh** (`POST /auth/refresh`) issues a new refresh token and revokes the old one each time. The refresh JWT's `jti` is a row in `refresh_tokens`. If an already-revoked refresh token is presented, it is treated as a stolen token and **every session of that user is revoked**.
- **Logout** (`POST /auth/logout`) revokes one refresh token. `POST /auth/logout-all` revokes all of the caller's sessions.
- Each authenticated request reloads the user's status, the tenant's status and the user's permissions from the database. So disabling a user, suspending a tenant or changing a role takes effect immediately, not when the access token expires.
- Login runs bcrypt even when the user does not exist. This keeps response times the same, so timing does not reveal which emails are registered.

## RBAC

**Modules → permissions → roles → users.** Modules and permissions form a global catalogue that is seeded by migrations. Roles belong to a tenant.

| Module            | Permissions                                   | Endpoints |
|-------------------|-----------------------------------------------|-----------|
| `user_management` | `read`, `create`, `update`, `delete`          | `/users`, `/roles`, `/modules` |
| `inventory` (dummy) | `read`, `create`, `update`, `delete`        | `/inventory/items` (tenant-scoped table) |
| `reports` (dummy) | `read`, `export`                              | `/reports/summary`, `/reports/inventory.csv` |
| `billing` (dummy) | `read`, `manage`                              | `/billing/invoices`, `/billing/subscription` |

Permission codes look like `<module>:<action>`, for example `inventory:read`. Routes declare the permission they need in `internal/server/router.go`:

```go
inv.POST("", perm(domain.PermInventoryCreate), invH.Create)
```

**Guardrails**
- **No privilege escalation.** You can only grant permissions you hold yourself, whether through role permissions or role assignment. You can only edit a role if you hold every permission in it. Only tenant admins can grant or revoke the system admin role.
- **System role is immutable.** It cannot be renamed, edited or deleted, and its name is reserved.
- **Last admin protection.** A tenant always keeps at least one active admin. The check covers disable, delete and role removal, and runs under a tenant row lock to handle concurrent requests.
- You cannot disable or delete yourself. A role that is still assigned to users cannot be deleted.

### Example: create a role and assign it

```bash
TOKEN=...   # admin access token
curl -X POST localhost:8080/api/v1/roles -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Inventory Clerk","description":"Manage stock","permissions":["inventory:read","inventory:create","inventory:update"]}'

curl -X POST localhost:8080/api/v1/users -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"email":"clerk@acme.test","first_name":"Carl","role_ids":["<role-id>"]}'

curl -X PUT localhost:8080/api/v1/users/<user-id>/roles -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"role_ids":["<role-id>"]}'
```

## Tenant isolation

Tenant isolation is enforced at several layers:

1. **The tenant comes only from the verified JWT** (`tid` claim) and is never read from the request body, path or headers. Login is the only place a tenant slug is supplied, and there the credentials must match that tenant.
2. **Every repository method on tenant-owned data takes `tenantID`** and filters on it. A user ID or role ID from another tenant returns `404`, not `403`, so the API does not reveal that the resource exists.
3. **The database enforces it too.** `user_roles`, `user_action_tokens` and `refresh_tokens` use **composite foreign keys** `(user_id, tenant_id)` and `(role_id, tenant_id)`. Assigning one tenant's role to another tenant's user fails even if the application code has a bug.
4. Emails are unique per tenant, not globally. The same person can belong to several tenants with separate accounts.

## API reference

All responses use the shape `{"data": ...}` and add `"meta": {total, limit, offset}` for lists. Errors use
`{"error": {"code", "message", "details?", "request_id"}}`.

| Method | Path | Auth | Permission |
|---|---|---|---|
| GET | `/healthz`, `/readyz` | – | – |
| POST | `/api/v1/platform/tenants` | Platform key | – |
| GET | `/api/v1/platform/tenants`, `/api/v1/platform/tenants/:id` | Platform key | – |
| PATCH | `/api/v1/platform/tenants/:id/status` `{status: active\|suspended}` | Platform key | – |
| GET | `/api/v1/auth/onboarding/verify?token=` | – | – |
| POST | `/api/v1/auth/onboarding/set-password` | setup JWT in body | – |
| POST | `/api/v1/auth/login` | – | – |
| POST | `/api/v1/auth/refresh`, `/api/v1/auth/logout` | refresh JWT in body | – |
| GET | `/api/v1/auth/me` | Bearer | – |
| POST | `/api/v1/auth/logout-all` | Bearer | – |
| GET | `/api/v1/modules` | Bearer | `user_management:read` |
| GET | `/api/v1/users?status=&q=&limit=&offset=` | Bearer | `user_management:read` |
| POST | `/api/v1/users` (invite) | Bearer | `user_management:create` |
| GET | `/api/v1/users/:id` | Bearer | `user_management:read` |
| PATCH | `/api/v1/users/:id` `{first_name,last_name}` | Bearer | `user_management:update` |
| PATCH | `/api/v1/users/:id/status` `{status: enabled\|disabled}` | Bearer | `user_management:update` |
| PUT | `/api/v1/users/:id/roles` `{role_ids}` | Bearer | `user_management:update` |
| POST | `/api/v1/users/:id/resend-invite` | Bearer | `user_management:create` |
| DELETE | `/api/v1/users/:id` | Bearer | `user_management:delete` |
| GET | `/api/v1/roles`, `/api/v1/roles/:id` | Bearer | `user_management:read` |
| POST | `/api/v1/roles` `{name,description,permissions}` | Bearer | `user_management:create` |
| PUT | `/api/v1/roles/:id` `{name,description}` | Bearer | `user_management:update` |
| PUT | `/api/v1/roles/:id/permissions` `{permissions}` | Bearer | `user_management:update` |
| DELETE | `/api/v1/roles/:id` | Bearer | `user_management:delete` |
| GET/POST | `/api/v1/inventory/items` | Bearer | `inventory:read` / `inventory:create` |
| GET/PUT/DELETE | `/api/v1/inventory/items/:id` | Bearer | `inventory:read` / `update` / `delete` |
| GET | `/api/v1/reports/summary` | Bearer | `reports:read` |
| GET | `/api/v1/reports/inventory.csv` | Bearer | `reports:export` |
| GET | `/api/v1/billing/invoices` | Bearer | `billing:read` |
| PUT | `/api/v1/billing/subscription` `{plan,seats}` | Bearer | `billing:manage` |

## Migrations

Migrations are plain SQL files in `migrations/` in [golang-migrate](https://github.com/golang-migrate/migrate) format. They are embedded into the `migrate` binary.

| Version | Contents |
|---|---|
| 000001 | tenants, users, modules, permissions, roles, role_permissions, user_roles, user_action_tokens, refresh_tokens |
| 000002 | seed modules + permissions |
| 000003 | inventory_items (dummy module) |

```bash
make migrate-up | make migrate-down | make migrate-version
make migrate-create name=add_something
docker compose run --rm migrate          # in Docker
```

## Configuration

All settings come from environment variables. See [`.env.example`](.env.example) for the full list. The app refuses to start if `DATABASE_URL` is missing, if `JWT_SECRET` is shorter than 32 characters, or if `PLATFORM_API_KEY` is shorter than 16 characters.

## Production notes / next steps

- Use real secrets (`openssl rand -base64 48`) from a secret manager, and put the API behind TLS. Set `HTTP_TRUSTED_PROXIES`.
- For multiple services, consider RS256/EdDSA with a JWKS endpoint so other services can verify tokens without the shared secret.
- Add rate limiting on `/auth/login`, `/auth/refresh` and the onboarding endpoints, for example at the gateway or with Redis.
- Optional: PostgreSQL Row-Level Security as a further isolation layer (per-request `SET LOCAL app.tenant_id`).
- Cache the principal's permissions, for example in Redis with invalidation on role changes, if the per-request lookup becomes a hotspot.
- Send email through an outbox table and a worker, to guarantee delivery and retries.
- Add password reset ("forgot password"). It can reuse the `user_action_tokens` table with a new purpose.
