#!/usr/bin/env bash
# Seed complementar do realm dmpf, só de desenvolvimento: roda depois do seed
# gerado (infra/keycloak/dev/seed.sh) e deixa pronto o operador do marketplace.
# WHY: infra/ é saída do dmpf-plugin (ADR-011), então o que é do marketplace mora
# aqui, na borda que verifica o token. Cada chamada do kcadm.sh sobe uma JVM: o
# script lê cada lista uma vez. Nunca rodar no hmg: o client marketplace-dev
# aceita senha direto no endpoint de token e o login do Swagger UI local sem
# PKCE. O converged.sh confere pelo HTTP o mesmo estado que as pós-condições.
set -euo pipefail

: "${KEYCLOAK_URL:?KEYCLOAK_URL ausente}"
admin_user="${KC_BOOTSTRAP_ADMIN_USERNAME:-admin}"
admin_password="${KC_BOOTSTRAP_ADMIN_PASSWORD:-admin}"
seed_password="${KEYCLOAK_SEED_PASSWORD:-dmpf-dev-password}"
realm=dmpf
org_alias=marketplace
tenant=marketplace
operator=operator
client=marketplace-dev
audience=dmpf-bff
roles=(tenancy:read tenancy:write)
scopes=(basic acr roles profile email organization)
swagger_port="${SWAGGER_PORT:-8082}"
swagger_origins=("http://127.0.0.1:$swagger_port" "http://localhost:$swagger_port")
config="$(mktemp)"
body="$(mktemp)"
trap 'rm -f "$config" "$body"' EXIT

fail() {
  echo "seed-marketplace: $1" >&2
  exit 1
}

case "$seed_password" in
  *[\"\\]*) fail "KEYCLOAK_SEED_PASSWORD não aceita aspas nem barra invertida" ;;
esac

kc() { /opt/keycloak/bin/kcadm.sh "$@" --config "$config"; }
field() { grep ",$2\$" <<<"$1" | cut -d, -f1 || true; }
contains() { grep -qx -- "$2" <<<"$1"; }

logged_in=""
for _ in $(seq 1 60); do
  if kc config credentials --server "$KEYCLOAK_URL" --realm master \
    --user "$admin_user" --password "$admin_password" >/dev/null 2>&1; then
    logged_in=1
    break
  fi
  sleep 2
done
[ -n "$logged_in" ] || fail "login no realm master falhou em $KEYCLOAK_URL"

ensure_org() {
  local orgs id
  orgs="$(kc get organizations -r "$realm" -q max=1000 --fields id,alias --format csv --noquotes)"
  id="$(field "$orgs" "$org_alias")"
  if [ -z "$id" ]; then
    id="$(kc create organizations -r "$realm" -s name=Marketplace -s "alias=$org_alias" -i)"
  fi
  kc update "organizations/$id" -r "$realm" -s "attributes={\"tenant_id\":[\"$tenant\"]}"
  echo "$id"
}

ensure_roles() {
  local current role
  current="$(kc get roles -r "$realm" --fields name --format csv --noquotes)"
  for role in "${roles[@]}"; do
    contains "$current" "$role" ||
      kc create roles -r "$realm" -s "name=$role" -s "description=Permissão $role do marketplace"
  done
}

ensure_operator() {
  local users id
  users="$(kc get users -r "$realm" -q "username=$operator" -q exact=true --fields id,username --format csv --noquotes)"
  id="$(field "$users" "$operator")"
  if [ -z "$id" ]; then
    cat >"$body" <<JSON
{"username": "$operator", "enabled": true, "emailVerified": true, "email": "$operator@marketplace.test",
 "firstName": "Operador", "lastName": "Marketplace",
 "credentials": [{"type": "password", "value": "$seed_password", "temporary": false}]}
JSON
    kc create users -r "$realm" -f "$body" -i
    return
  fi
  kc update "users/$id" -r "$realm" -s enabled=true -s emailVerified=true \
    -s "email=$operator@marketplace.test" -s firstName=Operador -s lastName=Marketplace -s 'requiredActions=[]'
  kc set-password -r "$realm" --userid "$id" --new-password "$seed_password"
  echo "$id"
}

# INVARIANT: o authn resolve o tenant por organization.*.tenant_id e recusa
# mais de uma organização, então o operador é membro só da marketplace.
ensure_single_membership() {
  local org=$1 user=$2 current other
  current="$(kc get "organizations/members/$user/organizations" -r "$realm" --fields id --format csv --noquotes)"
  for other in $current; do
    [ "$other" = "$org" ] || kc delete "organizations/$other/members/$user" -r "$realm"
  done
  contains "$current" "$org" || kc create "organizations/$org/members" -r "$realm" -b "\"$user\""
}

# WHY: o Swagger UI gerado (infra/local/compose/swagger-ui.yml) não liga o PKCE,
# então o authorization code daqui aceita a troca sem code_verifier; o
# webOrigins libera o POST do navegador ao endpoint de token.
write_client() {
  local origin redirects="" origins=""
  for origin in "${swagger_origins[@]}"; do
    redirects+="${redirects:+, }\"$origin/oauth2-redirect.html\""
    origins+="${origins:+, }\"$origin\""
  done
  cat >"$body" <<JSON
{"clientId": "$client", "name": "Marketplace (desenvolvimento)", "enabled": true,
 "protocol": "openid-connect", "publicClient": true, "standardFlowEnabled": true,
 "implicitFlowEnabled": false, "directAccessGrantsEnabled": true, "serviceAccountsEnabled": false,
 "redirectUris": [$redirects], "webOrigins": [$origins],
 "attributes": {"pkce.code.challenge.method": ""},
 "fullScopeAllowed": true, "defaultClientScopes": [], "optionalClientScopes": []}
JSON
}

write_audience_mapper() {
  cat >"$body" <<JSON
{"name": "$audience-audience", "protocol": "openid-connect", "protocolMapper": "oidc-audience-mapper",
 "config": {"included.client.audience": "$audience", "id.token.claim": "false",
  "access.token.claim": "true", "introspection.token.claim": "true"}}
JSON
}

ensure_client() {
  local id
  id="$(kc get clients -r "$realm" -q "clientId=$client" --fields id --format csv --noquotes)"
  write_client
  if [ -z "$id" ]; then
    id="$(kc create clients -r "$realm" -f "$body" -i)"
  else
    kc update "clients/$id" -r "$realm" -f "$body"
  fi
  echo "$id"
}

ensure_client_scopes() {
  local id=$1 all defaults optionals scope sid
  all="$(kc get client-scopes -r "$realm" --fields id,name --format csv --noquotes)"
  defaults="$(kc get "clients/$id/default-client-scopes" -r "$realm" --fields name --format csv --noquotes)"
  optionals="$(kc get "clients/$id/optional-client-scopes" -r "$realm" --fields name --format csv --noquotes)"
  for scope in "${scopes[@]}"; do
    sid="$(field "$all" "$scope")"
    [ -n "$sid" ] || fail "client scope $scope ausente no realm $realm"
    if contains "$optionals" "$scope"; then
      kc delete "clients/$id/optional-client-scopes/$sid" -r "$realm"
    fi
    contains "$defaults" "$scope" || kc update "clients/$id/default-client-scopes/$sid" -r "$realm" -n -b '{}'
  done
}

ensure_audience_mapper() {
  local id=$1 mappers mid
  mappers="$(kc get "clients/$id/protocol-mappers/models" -r "$realm" --fields id,name --format csv --noquotes)"
  mid="$(field "$mappers" "$audience-audience")"
  write_audience_mapper
  if [ -z "$mid" ]; then
    kc create "clients/$id/protocol-mappers/models" -r "$realm" -f "$body"
  else
    kc update "clients/$id/protocol-mappers/models/$mid" -r "$realm" -m -f "$body"
  fi
}

org="$(ensure_org)"
ensure_roles
user="$(ensure_operator)"
ensure_single_membership "$org" "$user"
kc add-roles -r "$realm" --uid "$user" --rolename "${roles[0]}" --rolename "${roles[1]}"
client_id="$(ensure_client)"
ensure_client_scopes "$client_id"
ensure_audience_mapper "$client_id"

org_json="$(kc get "organizations/$org" -r "$realm")"
memberships="$(kc get "organizations/members/$user/organizations" -r "$realm" --fields id --format csv --noquotes)"
user_roles="$(kc get-roles -r "$realm" --uid "$user" --effective --fields name --format csv --noquotes)"
client_scopes="$(kc get "clients/$client_id/default-client-scopes" -r "$realm" --fields name --format csv --noquotes)"
client_mappers="$(kc get "clients/$client_id/protocol-mappers/models" -r "$realm" --fields name --format csv --noquotes)"
client_json="$(kc get "clients/$client_id" -r "$realm" --fields standardFlowEnabled,redirectUris,webOrigins)"

grep -q "\"$tenant\"" <<<"$org_json" || fail "pós-condição falhou: tenant de $org_alias"
[ "$memberships" = "$org" ] || fail "pós-condição falhou: $operator membro só de $org_alias"
for role in "${roles[@]}"; do
  contains "$user_roles" "$role" || fail "pós-condição falhou: $operator com $role"
done
for scope in "${scopes[@]}"; do
  contains "$client_scopes" "$scope" || fail "pós-condição falhou: $client com o scope $scope"
done
contains "$client_mappers" "$audience-audience" || fail "pós-condição falhou: $client com a audiência $audience"
grep -q '"standardFlowEnabled" : true' <<<"$client_json" || fail "pós-condição falhou: $client com authorization code"
for origin in "${swagger_origins[@]}"; do
  grep -qF "\"$origin/oauth2-redirect.html\"" <<<"$client_json" || fail "pós-condição falhou: $client com o redirect de $origin"
  grep -qF "\"$origin\"" <<<"$client_json" || fail "pós-condição falhou: $client com a origem $origin"
done
echo "seed-marketplace: realm $realm convergido"
