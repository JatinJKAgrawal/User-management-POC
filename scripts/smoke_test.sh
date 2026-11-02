#!/usr/bin/env bash
# End-to-end smoke test of the onboarding, auth, RBAC and tenant-isolation flows.
#
# Requirements: curl, jq. The API must be running.
#   API_URL           default http://localhost:8080
#   PLATFORM_API_KEY  must match the server's key
#   MAILHOG_URL       default http://localhost:8025 (onboarding links are read from MailHog)
#   API_LOG_FILE      alternative to MailHog: read links from the API log (MAIL_DRIVER=log)
set -euo pipefail

API_URL=${API_URL:-http://localhost:8080}
PLATFORM_API_KEY=${PLATFORM_API_KEY:?PLATFORM_API_KEY is required}
MAILHOG_URL=${MAILHOG_URL:-http://localhost:8025}
RUN=$(date +%s)
PASSWORD='Str0ng!Passw0rd'
fail=0
BODY_FILE=$(mktemp); trap 'rm -f "$BODY_FILE"' EXIT

pass() { printf '  \033[32mPASS\033[0m %s\n' "$1"; }
bad()  { printf '  \033[31mFAIL\033[0m %s (got %s)\n' "$1" "$2"; fail=1; }
expect() { [[ "$2" == "$3" ]] && pass "$1" || bad "$1 (want $3)" "$2"; }

# req METHOD PATH [TOKEN] [BODY] -> prints "<status> <body>"
req() {
  local args=(-s -o "$BODY_FILE" -w '%{http_code}' -X "$1" "$API_URL$2" -H 'Content-Type: application/json' -H 'Accept: application/json')
  [[ -n "${3:-}" ]] && args+=(-H "Authorization: Bearer $3")
  [[ -n "${4:-}" ]] && args+=(-d "$4")
  local code; code=$(curl "${args[@]}")
  echo "$code $(cat "$BODY_FILE")"
}
status() { cut -d' ' -f1 <<<"$1"; }
body()   { cut -d' ' -f2- <<<"$1"; }

onboarding_token() { # email -> plain onboarding token from the latest invite
  local link=""
  for _ in $(seq 1 20); do
    if [[ -n "${API_LOG_FILE:-}" ]]; then
      link=$(grep -F "$1" "$API_LOG_FILE" | grep -o 'token=[A-Za-z0-9_-]*' | tail -1 || true)
    else
      link=$(curl -s "$MAILHOG_URL/api/v2/search?kind=to&query=$1" |
        jq -r '.items[0].Content.Body // ""' | grep -o 'token=[A-Za-z0-9_-]*' | head -1 || true)
    fi
    [[ -n "$link" ]] && { echo "${link#token=}"; return; }
    sleep 0.5
  done
  echo "could not find onboarding email for $1" >&2; exit 1
}

onboard() { # email -> sets password via onboarding link
  local tok setup
  tok=$(onboarding_token "$1")
  setup=$(curl -s -H 'Accept: application/json' "$API_URL/api/v1/auth/onboarding/verify?token=$tok" | jq -r .data.setup_token)
  req POST /api/v1/auth/onboarding/set-password "" \
    "{\"token\":\"$setup\",\"password\":\"$PASSWORD\",\"confirm_password\":\"$PASSWORD\"}" >/dev/null
  echo "$tok"
}

login() { # tenant email -> access token
  body "$(req POST /api/v1/auth/login "" "{\"tenant\":\"$1\",\"email\":\"$2\",\"password\":\"$PASSWORD\"}")" | jq -r .data.access_token
}

create_tenant() { # slug admin-email
  curl -s -X POST "$API_URL/api/v1/platform/tenants" -H "X-Platform-API-Key: $PLATFORM_API_KEY" \
    -H 'Content-Type: application/json' \
    -d "{\"name\":\"$1 Inc\",\"slug\":\"$1\",\"admin\":{\"email\":\"$2\",\"first_name\":\"Admin\"}}"
}

echo "== Tenant provisioning"
A=acme-$RUN; B=globex-$RUN
ADMIN_A=admin@$A.test; ADMIN_B=admin@$B.test
expect "create tenant A" "$(create_tenant "$A" "$ADMIN_A" | jq -r .data.tenant.slug)" "$A"
expect "create tenant B" "$(create_tenant "$B" "$ADMIN_B" | jq -r .data.tenant.slug)" "$B"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API_URL/api/v1/platform/tenants" -H 'X-Platform-API-Key: wrong')
expect "platform API rejects bad key" "$r" 401

echo "== Onboarding"
r=$(req POST /api/v1/auth/login "" "{\"tenant\":\"$A\",\"email\":\"$ADMIN_A\",\"password\":\"$PASSWORD\"}")
expect "invited user cannot log in before onboarding" "$(status "$r")" 401
TOK=$(onboard "$ADMIN_A")
r=$(curl -s -o /dev/null -w '%{http_code}' -H 'Accept: application/json' "$API_URL/api/v1/auth/onboarding/verify?token=$TOK")
expect "onboarding link is single use" "$r" 410
onboard "$ADMIN_B" >/dev/null
TA=$(login "$A" "$ADMIN_A"); TB=$(login "$B" "$ADMIN_B")
[[ "$TA" != null && "$TB" != null ]] && pass "tenant admins log in" || bad "tenant admins log in" "$TA"
r=$(req POST /api/v1/auth/login "" "{\"tenant\":\"$B\",\"email\":\"$ADMIN_A\",\"password\":\"$PASSWORD\"}")
expect "credentials are tenant scoped" "$(status "$r")" 401

echo "== RBAC"
r=$(req POST /api/v1/roles "$TA" '{"name":"Inventory Viewer","permissions":["inventory:read","reports:read"]}')
expect "admin creates role" "$(status "$r")" 201
ROLE=$(body "$r" | jq -r .data.id)
r=$(req POST /api/v1/users "$TA" "{\"email\":\"viewer@$A.test\",\"first_name\":\"Vera\",\"role_ids\":[\"$ROLE\"]}")
expect "admin invites user with role" "$(status "$r")" 201
VIEWER=$(body "$r" | jq -r .data.user.id)
onboard "viewer@$A.test" >/dev/null
TV=$(login "$A" "viewer@$A.test")
expect "viewer can read inventory"       "$(status "$(req GET /api/v1/inventory/items "$TV")")" 200
expect "viewer cannot create inventory"  "$(status "$(req POST /api/v1/inventory/items "$TV" '{"sku":"X","name":"x"}')")" 403
expect "viewer cannot list users"        "$(status "$(req GET /api/v1/users "$TV")")" 403
expect "viewer cannot read billing"      "$(status "$(req GET /api/v1/billing/invoices "$TV")")" 403
r=$(req PUT "/api/v1/roles/$ROLE/permissions" "$TA" '{"permissions":["inventory:read","inventory:create"]}')
expect "admin updates role permissions" "$(status "$r")" 200
expect "permission change applies immediately" \
  "$(status "$(req POST /api/v1/inventory/items "$TV" '{"sku":"SKU-1","name":"Widget","quantity":5}')")" 201
ADMIN_ROLE=$(body "$(req GET /api/v1/roles "$TA")" | jq -r '.data[] | select(.is_system) | .id')
r=$(req PUT "/api/v1/roles/$ADMIN_ROLE/permissions" "$TA" '{"permissions":[]}')
expect "system role is immutable" "$(status "$r")" 403

echo "== Privilege escalation guards"
r=$(req POST /api/v1/roles "$TA" '{"name":"User Manager","permissions":["user_management:read","user_management:create","user_management:update"]}')
UM_ROLE=$(body "$r" | jq -r .data.id)
req POST /api/v1/users "$TA" "{\"email\":\"um@$A.test\",\"role_ids\":[\"$UM_ROLE\"]}" >/dev/null
onboard "um@$A.test" >/dev/null
TU=$(login "$A" "um@$A.test")
expect "user manager can invite users" \
  "$(status "$(req POST /api/v1/users "$TU" "{\"email\":\"plain@$A.test\"}")")" 201
expect "cannot create role with permissions it lacks" \
  "$(status "$(req POST /api/v1/roles "$TU" '{"name":"Biller","permissions":["billing:manage"]}')")" 403
expect "cannot grant itself the Tenant Admin role" \
  "$(status "$(req PUT "/api/v1/users/$(body "$(req GET /api/v1/auth/me "$TU")" | jq -r .data.user.id)/roles" "$TU" "{\"role_ids\":[\"$UM_ROLE\",\"$ADMIN_ROLE\"]}")")" 403

echo "== Tenant isolation"
ITEM=$(body "$(req GET /api/v1/inventory/items "$TA")" | jq -r '.data[0].id')
expect "tenant B cannot read tenant A item"  "$(status "$(req GET "/api/v1/inventory/items/$ITEM" "$TB")")" 404
expect "tenant B cannot see tenant A user"   "$(status "$(req GET "/api/v1/users/$VIEWER" "$TB")")" 404
expect "tenant B cannot edit tenant A role"  "$(status "$(req PUT "/api/v1/roles/$ROLE/permissions" "$TB" '{"permissions":[]}')")" 404
r=$(req PUT "/api/v1/users/$(body "$(req GET /api/v1/auth/me "$TB")" | jq -r .data.user.id)/roles" "$TB" "{\"role_ids\":[\"$ROLE\"]}")
expect "tenant B cannot assign tenant A role" "$(status "$r")" 422
expect "tenant B sees only its own users" "$(body "$(req GET /api/v1/users "$TB")" | jq -r .meta.total)" 1

echo "== Session lifecycle"
r=$(req PATCH "/api/v1/users/$VIEWER/status" "$TA" '{"status":"disabled"}')
expect "admin disables user" "$(status "$r")" 200
expect "disabled user's token is rejected" "$(status "$(req GET /api/v1/inventory/items "$TV")")" 401
r=$(req POST /api/v1/auth/login "" "{\"tenant\":\"$A\",\"email\":\"$ADMIN_A\",\"password\":\"$PASSWORD\"}")
RT=$(body "$r" | jq -r .data.refresh_token)
r=$(req POST /api/v1/auth/refresh "" "{\"refresh_token\":\"$RT\"}")
expect "refresh rotates tokens" "$(status "$r")" 200
expect "old refresh token is rejected (reuse)" "$(status "$(req POST /api/v1/auth/refresh "" "{\"refresh_token\":\"$RT\"}")")" 401
expect "admin cannot disable last admin (self)" \
  "$(status "$(req PATCH "/api/v1/users/$(body "$(req GET /api/v1/auth/me "$TA")" | jq -r .data.user.id)/status" "$TA" '{"status":"disabled"}')")" 403

echo
[[ $fail == 0 ]] && echo "All checks passed" || { echo "Some checks failed"; exit 1; }
