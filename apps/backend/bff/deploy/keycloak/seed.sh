#!/usr/bin/env bash
# Seed complementar do realm dmpf, só de desenvolvimento: roda depois do seed
# gerado (infra/keycloak/dev/seed.sh) e deixa prontos o operador, o comprador e
# a loja-demo com o dono.
# WHY: infra/ é saída do dmpf-plugin (ADR-011), então o que é do marketplace mora
# aqui, na borda que verifica o token. Cada chamada do kcadm.sh sobe uma JVM: o
# script lê cada lista uma vez. Nunca rodar no hmg: os clients de login
# aceitam senha no endpoint de token e o Swagger UI sem PKCE. O converged.sh
# confere pelo HTTP o essencial das pós-condições e o marcador que o seed grava
# ao terminar.
set -euo pipefail
# WHY: sem isso, a falha de um kcadm dentro de $(...) não interrompe a função,
# que devolve o id e segue como se tivesse convergido.
shopt -s inherit_errexit

: "${KEYCLOAK_URL:?KEYCLOAK_URL ausente}"
admin_user="${KC_BOOTSTRAP_ADMIN_USERNAME:-admin}"
admin_password="${KC_BOOTSTRAP_ADMIN_PASSWORD:-admin}"
seed_password="${KEYCLOAK_SEED_PASSWORD:-dmpf-dev-password}"
iam_admin_secret="${KEYCLOAK_IAM_ADMIN_SECRET:?KEYCLOAK_IAM_ADMIN_SECRET ausente}"
realm=dmpf
org_alias=marketplace
tenant=marketplace
operator=operator
buyer=buyer-demo
store_alias=loja-demo
store_owner=loja-demo-owner
owner_group=owner
client=marketplace-dev
buyer_client=marketplace-buyer
seller_client=marketplace-seller
store_client=marketplace-store
iam_client=iam-admin
session_events_client=dmpf-session-events
# WHY: manage-users e manage-organizations liberam tudo antes do FGAP v2. A
# service account tem exatamente estes papéis de realm-management, e o que ela
# altera sai só das permissões do FGAP; os proibidos se conferem nos efetivos,
# para pegar também os que chegam por composite.
iam_admin_roles=(query-organizations view-organizations view-users view-realm view-clients)
iam_admin_forbidden=(manage-users manage-organizations realm-admin manage-realm manage-clients
  manage-authorization manage-identity-providers impersonation)
operators_group=operators
audience=dmpf-bff
tenant_claim=organization.$org_alias.tenant_id
group_mapper=organization-groups
operator_roles=(tenancy:read tenancy:write iam:read iam:write)
buyer_role=iam:buyer
roles=("${operator_roles[@]}" "$buyer_role")
# INVARIANT: o catálogo fechado da SPEC-2FJ0KYKV; são client roles do
# marketplace-store, que o bff só lê dentro de organization.<alias>, então uma
# permissão de loja nunca vale fora do tenant da loja.
catalog=(iam:read iam:write seller:read seller:write catalog:read catalog:write
  inventory:read inventory:write pricing:read pricing:write ordering:read ordering:write
  fulfillment:read fulfillment:write settlement:read settlement:write)
managed_scopes=(basic acr roles profile email organization)
scopes=(basic acr roles profile email organization)
buyer_scopes=(basic acr roles profile email)
seller_scopes=(basic acr profile email organization)
swagger_port="${SWAGGER_PORT:-8082}"
swagger_origins=("http://127.0.0.1:$swagger_port" "http://localhost:$swagger_port")
config="$(mktemp)"
body="$(mktemp)"
archive="$config.jsa"
trap 'rm -f "$config" "$body" "$archive"' EXIT
# WHY: cada kcadm.sh sobe uma JVM curta; sem JIT C2, com GC serial e o CDS
# criado na primeira chamada, a partida cai de ~920 ms para ~580 ms.
export KC_OPTS="${KC_OPTS:--XX:TieredStopAtLevel=1 -XX:+UseSerialGC -XX:SharedArchiveFile=$archive -XX:+AutoCreateSharedArchive}"

fail() {
  echo "seed-marketplace: $1" >&2
  exit 1
}

case "$seed_password$iam_admin_secret" in
  *[\"\\]*) fail "KEYCLOAK_SEED_PASSWORD e KEYCLOAK_IAM_ADMIN_SECRET não aceitam aspas nem barra invertida" ;;
esac

kc() { /opt/keycloak/bin/kcadm.sh "$@" --config "$config"; }
field() { grep ",$2\$" <<<"$1" | cut -d, -f1 || true; }
contains() { grep -qx -- "$2" <<<"$1"; }
listed() { printf '%s\n' "${@:2}" | grep -qx -- "$1"; }
sorted() { printf '%s\n' "$@" | sort; }

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
clients="$(kc get clients -r "$realm" --fields id,clientId --format csv --noquotes)"

ensure_org() {
  local alias=$1 name=$2 tenant_id=$3 orgs id
  orgs="$(kc get organizations -r "$realm" -q "q=tenant_id:$tenant_id" --fields id,alias --format csv --noquotes)"
  id="$(field "$orgs" "$alias")"
  if [ -n "$id" ]; then
    echo "$id"
    return
  fi
  orgs="$(kc get organizations -r "$realm" -q max=1000 --fields id,alias --format csv --noquotes)"
  id="$(field "$orgs" "$alias")"
  if [ -z "$id" ]; then
    id="$(kc create organizations -r "$realm" -s "name=$name" -s "alias=$alias" -i)"
  fi
  kc update "organizations/$id" -r "$realm" -s "attributes={\"tenant_id\":[\"$tenant_id\"]}"
  echo "$id"
}

ensure_roles() {
  local role created=""
  realm_roles="$(kc get roles -r "$realm" --fields id,name --format csv --noquotes)"
  for role in "${roles[@]}"; do
    [ -n "$(field "$realm_roles" "$role")" ] && continue
    kc create roles -r "$realm" -s "name=$role" -s "description=Permissão $role do marketplace"
    created=1
  done
  [ -z "$created" ] || realm_roles="$(kc get roles -r "$realm" --fields id,name --format csv --noquotes)"
}

ensure_user() {
  local username=$1 first=$2 last=$3 email="$1@marketplace.test" users id
  users="$(kc get users -r "$realm" -q "username=$username" -q exact=true \
    --fields id,username,enabled,emailVerified,email,firstName,lastName,requiredActions --format csv --noquotes)"
  id="$(grep ",$username," <<<"$users" | cut -d, -f1 || true)"
  if [ -z "$id" ]; then
    cat >"$body" <<JSON
{"username": "$username", "enabled": true, "emailVerified": true, "email": "$email",
 "firstName": "$first", "lastName": "$last",
 "credentials": [{"type": "password", "value": "$seed_password", "temporary": false}]}
JSON
    kc create users -r "$realm" -f "$body" -i
    return
  fi
  # WHY: o CSV omite o requiredActions vazio, então a linha só bate com o
  # usuário convergido. A senha não se lê: o set-password roda sempre.
  [ "$users" = "$id,$username,true,true,$email,$first,$last" ] ||
    kc update "users/$id" -r "$realm" -s enabled=true -s emailVerified=true \
      -s "email=$email" -s "firstName=$first" -s "lastName=$last" -s 'requiredActions=[]'
  kc set-password -r "$realm" --userid "$id" --new-password "$seed_password"
  echo "$id"
}

# INVARIANT: o authn resolve o tenant por organization.*.tenant_id e recusa
# mais de uma organização, então o operador é membro só da marketplace e o
# comprador de nenhuma (o tenant dele vem do claim fixo do marketplace-buyer).
ensure_memberships() {
  local user=$1 org=${2:-} current other
  current="$(kc get "organizations/members/$user/organizations" -r "$realm" --fields id --format csv --noquotes)"
  for other in $current; do
    [ "$other" = "$org" ] || kc delete "organizations/$other/members/$user" -r "$realm"
  done
  [ -z "$org" ] || contains "$current" "$org" || kc create "organizations/$org/members" -r "$realm" -b "\"$user\""
}

# WHY: o Swagger UI gerado (infra/local/compose/swagger-ui.yml) não liga o PKCE,
# então o authorization code daqui aceita a troca sem code_verifier; o
# webOrigins libera o POST do navegador ao endpoint de token.
write_client() {
  local client_id=$1 name=$2 origin redirects="" origins=""
  for origin in "${swagger_origins[@]}"; do
    redirects+="${redirects:+, }\"$origin/oauth2-redirect.html\""
    origins+="${origins:+, }\"$origin\""
  done
  cat >"$body" <<JSON
{"clientId": "$client_id", "name": "$name", "enabled": true,
 "protocol": "openid-connect", "publicClient": true, "standardFlowEnabled": true,
 "implicitFlowEnabled": false, "directAccessGrantsEnabled": true, "serviceAccountsEnabled": false,
 "redirectUris": [$redirects], "webOrigins": [$origins],
 "attributes": {"pkce.code.challenge.method": ""},
 "fullScopeAllowed": true, "defaultClientScopes": [], "optionalClientScopes": []}
JSON
}

# WHY: o ponto no nome vira claim aninhado (organization.<alias>.tenant_id), o
# mesmo caminho que o mapper de Organization emite para os membros.
write_tenant_mapper() {
  cat >"$body" <<JSON
{"name": "$org_alias-tenant", "protocol": "openid-connect", "protocolMapper": "oidc-hardcoded-claim-mapper",
 "config": {"claim.name": "$tenant_claim", "claim.value": "$tenant", "jsonType.label": "String",
  "id.token.claim": "false", "access.token.claim": "true", "userinfo.token.claim": "false",
  "introspection.token.claim": "true"}}
JSON
}

write_store_client() {
  cat >"$body" <<JSON
{"clientId": "$1", "name": "$2", "enabled": true, "protocol": "openid-connect",
 "publicClient": false, "standardFlowEnabled": false, "implicitFlowEnabled": false,
 "directAccessGrantsEnabled": false, "serviceAccountsEnabled": false,
 "fullScopeAllowed": false, "defaultClientScopes": [], "optionalClientScopes": []}
JSON
}

write_iam_client() {
  cat >"$body" <<JSON
{"clientId": "$1", "name": "$2", "enabled": true, "protocol": "openid-connect",
 "description": "", "publicClient": false, "clientAuthenticatorType": "client-secret", "secret": "$iam_admin_secret",
 "standardFlowEnabled": false, "implicitFlowEnabled": false, "directAccessGrantsEnabled": false,
 "serviceAccountsEnabled": true, "fullScopeAllowed": true,
 "defaultClientScopes": [], "optionalClientScopes": []}
JSON
}

# WHY: com addGroupRoleMappings, as roles dos grupos de Organization do usuário
# saem em organization.<alias>.resource_access. O mapper mora no client do
# lojista, não no scope organization: no scope, todo client que o pede (o
# dmpf-portal, por exemplo) carregaria as permissões de loja.
write_group_mapper() {
  cat >"$body" <<JSON
{"name": "$group_mapper", "protocol": "openid-connect",
 "protocolMapper": "oidc-organization-group-membership-mapper",
 "config": {"addGroupRoleMappings": "true", "id.token.claim": "false", "access.token.claim": "true",
  "lightweight.claim": "false", "userinfo.token.claim": "false", "introspection.token.claim": "true"}}
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
  local client_id=$1 name=$2 writer=${3:-write_client} id
  id="$(field "$clients" "$client_id")"
  "$writer" "$client_id" "$name"
  if [ -z "$id" ]; then
    id="$(kc create clients -r "$realm" -f "$body" -i)"
  else
    kc update "clients/$id" -r "$realm" -f "$body"
  fi
  echo "$id"
}

# Deixa como default exatamente os scopes pedidos entre os gerenciados pelo seed:
# organization (opcional default do realm) sai do comprador e roles do seller.
ensure_client_scopes() {
  local id=$1 all defaults optionals scope sid
  shift
  all="$(kc get client-scopes -r "$realm" --fields id,name --format csv --noquotes)"
  defaults="$(kc get "clients/$id/default-client-scopes" -r "$realm" --fields name --format csv --noquotes)"
  optionals="$(kc get "clients/$id/optional-client-scopes" -r "$realm" --fields name --format csv --noquotes)"
  for scope in "$@"; do
    sid="$(field "$all" "$scope")"
    [ -n "$sid" ] || fail "client scope $scope ausente no realm $realm"
    if contains "$optionals" "$scope"; then
      kc delete "clients/$id/optional-client-scopes/$sid" -r "$realm"
    fi
    contains "$defaults" "$scope" || kc update "clients/$id/default-client-scopes/$sid" -r "$realm" -n -b '{}'
  done
  for scope in "${managed_scopes[@]}"; do
    printf '%s\n' "$@" | grep -qx -- "$scope" && continue
    sid="$(field "$all" "$scope")"
    if contains "$optionals" "$scope"; then kc delete "clients/$id/optional-client-scopes/$sid" -r "$realm"; fi
    if contains "$defaults" "$scope"; then kc delete "clients/$id/default-client-scopes/$sid" -r "$realm"; fi
  done
}

# INVARIANT: o catálogo é fechado; a role fora dele sai, e com ela os mappings
# nos grupos, para que a service account do iam não a mapeie.
ensure_client_roles() {
  local id=$1 current role
  shift
  current="$(kc get "clients/$id/roles" -r "$realm" --fields name --format csv --noquotes)"
  for role in "$@"; do
    contains "$current" "$role" || kc create "clients/$id/roles" -r "$realm" -s "name=$role"
  done
  for role in $current; do
    listed "$role" "$@" || kc delete "clients/$id/roles/$role" -r "$realm"
  done
}

# WHY: grupo de Organization só aceita gestão pela API da Organization; a de
# grupos do realm recusa ("organization related group").
ensure_org_group() {
  local org=$1 name=$2 groups gid
  groups="$(kc get "organizations/$org/groups" -r "$realm" --fields id,name --format csv --noquotes)"
  gid="$(field "$groups" "$name")"
  [ -n "$gid" ] || gid="$(kc create "organizations/$org/groups" -r "$realm" -s "name=$name" -i)"
  echo "$gid"
}

ensure_group_client_roles() {
  local org=$1 gid=$2 cid=$3 mapped all role rid reps=""
  shift 3
  mapped="$(kc get "organizations/$org/groups/$gid/role-mappings/clients/$cid" -r "$realm" --fields name --format csv --noquotes)"
  all="$(kc get "clients/$cid/roles" -r "$realm" --fields id,name --format csv --noquotes)"
  for role in "$@"; do
    contains "$mapped" "$role" && continue
    rid="$(field "$all" "$role")"
    [ -n "$rid" ] || fail "client role $role ausente"
    reps+="${reps:+, }{\"id\": \"$rid\", \"name\": \"$role\", \"clientRole\": true, \"containerId\": \"$cid\"}"
  done
  [ -z "$reps" ] || kc create "organizations/$org/groups/$gid/role-mappings/clients/$cid" -r "$realm" -b "[$reps]"
}

ensure_group_member() {
  local org=$1 gid=$2 user=$3
  contains "$(kc get "organizations/$org/groups/$gid/members" -r "$realm" --fields id --format csv --noquotes)" "$user" ||
    kc update "organizations/$org/groups/$gid/members/$user" -r "$realm" -n -b '{}'
}

ensure_realm_group_members() {
  local name=$1 groups gid members user
  shift
  groups="$(kc get groups -r "$realm" -q "search=$name" -q exact=true --fields id,name --format csv --noquotes)"
  gid="$(field "$groups" "$name")"
  [ -n "$gid" ] || gid="$(kc create groups -r "$realm" -s "name=$name" -i)"
  members="$(kc get "groups/$gid/members" -r "$realm" -q max=1000 --fields id --format csv --noquotes)"
  for user in "$@"; do
    contains "$members" "$user" || kc update "users/$user/groups/$gid" -r "$realm" -n -b '{}'
  done
  echo "$gid"
}

# Põe o que falta e tira todo papel de realm-management fora da lista, deixado
# por uma versão anterior ou por ajuste manual.
ensure_iam_admin_roles() {
  local sa=$1 current role add=() remove=()
  current="$(kc get-roles -r "$realm" --uid "$sa" --cclientid realm-management --fields name --format csv --noquotes)"
  for role in "${iam_admin_roles[@]}"; do contains "$current" "$role" || add+=(--rolename "$role"); done
  for role in $current; do listed "$role" "${iam_admin_roles[@]}" || remove+=(--rolename "$role"); done
  [ ${#add[@]} -eq 0 ] || kc add-roles -r "$realm" --uid "$sa" --cclientid realm-management "${add[@]}"
  [ ${#remove[@]} -eq 0 ] || kc remove-roles -r "$realm" --uid "$sa" --cclientid realm-management "${remove[@]}"
}

# WHY: a política e a permissão do FGAP só se leem uma a uma; a description
# guarda o digest do que o seed escreveu, e a listagem única basta para pular
# o que já convergiu. Ela não prova o conteúdo, então o que nega (always) se
# reescreve sempre: uma edição manual ali reabre as contas protegidas.
ensure_authz() {
  local kind=$1 name=$2 fragment=$3 always=${4:-} digest row id
  digest="seed:$(sha256sum <<<"$fragment" | cut -c1-16)"
  authz_expected+=("$name,$digest")
  row="$(grep -E "^[^,]+,$name(,|\$)" <<<"$authz" || true)"
  id="${row%%,*}"
  [ -n "$always" ] || [ -z "$id" ] || [ "$row" != "$id,$name,$digest" ] || return 0
  printf '{"name": "%s", "description": "%s", %s}\n' "$name" "$digest" "$fragment" >"$body"
  if [ -z "$id" ]; then
    kc create "$authz_base/$kind" -r "$realm" -f "$body" >/dev/null
  else
    kc update "$authz_base/$kind/$id" -r "$realm" -f "$body"
  fi
}

grant() {
  printf '"resourceType": "%s", "scopes": [%s], "resources": [%s], "policies": ["%s"], "decisionStrategy": "UNANIMOUS"' "$@"
}

ensure_mapper() {
  local id=$1 name=$2 writer=$3 mappers mid
  mappers="$(kc get "clients/$id/protocol-mappers/models" -r "$realm" --fields id,name --format csv --noquotes)"
  mid="$(field "$mappers" "$name")"
  "$writer"
  if [ -z "$mid" ]; then
    kc create "clients/$id/protocol-mappers/models" -r "$realm" -f "$body"
  else
    kc update "clients/$id/protocol-mappers/models/$mid" -r "$realm" -m -f "$body"
  fi
}

check_client() {
  local id=$1 client_id=$2 scopes_csv optional_csv mappers_csv client_json scope origin
  shift 2
  scopes_csv="$(kc get "clients/$id/default-client-scopes" -r "$realm" --fields name --format csv --noquotes)"
  optional_csv="$(kc get "clients/$id/optional-client-scopes" -r "$realm" --fields name --format csv --noquotes)"
  mappers_csv="$(kc get "clients/$id/protocol-mappers/models" -r "$realm" --fields name --format csv --noquotes)"
  client_json="$(kc get "clients/$id" -r "$realm" --fields standardFlowEnabled,redirectUris,webOrigins)"
  for scope in "$@"; do
    contains "$scopes_csv" "$scope" || fail "pós-condição falhou: $client_id com o scope $scope"
  done
  for scope in "${managed_scopes[@]}"; do
    printf '%s\n' "$@" | grep -qx -- "$scope" && continue
    ! contains "$(printf '%s\n%s' "$scopes_csv" "$optional_csv")" "$scope" || fail "pós-condição falhou: $client_id sem o scope $scope"
  done
  contains "$mappers_csv" "$audience-audience" || fail "pós-condição falhou: $client_id com a audiência $audience"
  grep -q '"standardFlowEnabled" : true' <<<"$client_json" || fail "pós-condição falhou: $client_id com authorization code"
  for origin in "${swagger_origins[@]}"; do
    grep -qF "\"$origin/oauth2-redirect.html\"" <<<"$client_json" || fail "pós-condição falhou: $client_id com o redirect de $origin"
    grep -qF "\"$origin\"" <<<"$client_json" || fail "pós-condição falhou: $client_id com a origem $origin"
  done
  echo "$mappers_csv"
}

org="$(ensure_org "$org_alias" Marketplace "$tenant")"
ensure_roles
user="$(ensure_user "$operator" Operador Marketplace)"
ensure_memberships "$user" "$org"
role_args=()
for role in "${operator_roles[@]}"; do role_args+=(--rolename "$role"); done
kc add-roles -r "$realm" --uid "$user" "${role_args[@]}"
buyer_id="$(ensure_user "$buyer" Comprador Demo)"
ensure_memberships "$buyer_id"
kc add-roles -r "$realm" --uid "$buyer_id" --rolename "$buyer_role"
client_id="$(ensure_client "$client" "Marketplace (desenvolvimento)")"
ensure_client_scopes "$client_id" "${scopes[@]}"
ensure_mapper "$client_id" "$audience-audience" write_audience_mapper
buyer_client_id="$(ensure_client "$buyer_client" "Marketplace (comprador)")"
ensure_client_scopes "$buyer_client_id" "${buyer_scopes[@]}"
ensure_mapper "$buyer_client_id" "$audience-audience" write_audience_mapper
ensure_mapper "$buyer_client_id" "$org_alias-tenant" write_tenant_mapper
store_client_id="$(ensure_client "$store_client" "Marketplace (permissões de loja)" write_store_client)"
ensure_client_roles "$store_client_id" "${catalog[@]}"
seller_client_id="$(ensure_client "$seller_client" "Marketplace (lojista)")"
ensure_client_scopes "$seller_client_id" "${seller_scopes[@]}"
ensure_mapper "$seller_client_id" "$audience-audience" write_audience_mapper
ensure_mapper "$seller_client_id" "$group_mapper" write_group_mapper
iam_client_id="$(ensure_client "$iam_client" "Marketplace (service account do iam)" write_iam_client)"
iam_sa_id="$(kc get "clients/$iam_client_id/service-account-user" -r "$realm" --fields id --format csv --noquotes)"
ensure_iam_admin_roles "$iam_sa_id"
session_events_client_id="$(field "$clients" "$session_events_client")"
[ -n "$session_events_client_id" ] || fail "client $session_events_client ausente no realm $realm"
session_events_sa_id="$(kc get "clients/$session_events_client_id/service-account-user" -r "$realm" --fields id --format csv --noquotes)"
store_org="$(ensure_org "$store_alias" "Loja Demo" "$store_alias")"
owner_gid="$(ensure_org_group "$store_org" "$owner_group")"
ensure_group_client_roles "$store_org" "$owner_gid" "$store_client_id" "${catalog[@]}"
owner_id="$(ensure_user "$store_owner" Dono "Loja Demo")"
ensure_memberships "$owner_id" "$store_org"
ensure_group_member "$store_org" "$owner_gid" "$owner_id"
# WHY: o FGAP v2 não tem condição por alvo; o grupo operators reúne as contas
# que a service account do iam não pode gerir: o operador e as service accounts.
protected_users=("$user" "$iam_sa_id" "$session_events_sa_id")
operators_gid="$(ensure_realm_group_members "$operators_group" "${protected_users[@]}")"
protected_resources="$(printf '"%s", ' "${protected_users[@]}")"
protected_resources="${protected_resources%, }"

[ "$(kc get "realms/$realm" --fields adminPermissionsEnabled --format csv --noquotes)" = true ] ||
  kc update "realms/$realm" -s adminPermissionsEnabled=true
admin_permissions_id="$(kc get clients -r "$realm" -q clientId=admin-permissions --fields id --format csv --noquotes)"
[ -n "$admin_permissions_id" ] || fail "client admin-permissions ausente com o FGAP ligado"
authz_base="clients/$admin_permissions_id/authz/resource-server"
authz="$(kc get "$authz_base/policy" -r "$realm" -q max=-1 --fields id,name,description --format csv --noquotes)"
authz_expected=()
buyer_role_id="$(field "$realm_roles" "$buyer_role")"
# WHY: uma user policy sem usuários nunca concede, então nega quem a recebe.
# O deny do grupo operators cobre gerir, trocar senha e mover as contas
# protegidas; o map-roles não tem alias de grupo e leva deny por usuário. Os
# denies nascem antes das concessões, para que um seed interrompido não deixe
# a service account com as concessões e sem os denies.
ensure_authz policy/user iam-admin-sa "\"logic\": \"POSITIVE\", \"users\": [\"$iam_sa_id\"]"
ensure_authz policy/user deny-all '"logic": "POSITIVE", "users": []' always
ensure_authz permission/scope deny-operator-organization "$(grant Organizations '"manage"' "\"$org\"" deny-all)" always
ensure_authz permission/scope deny-operators-membership \
  "$(grant Groups '"manage-members", "manage-membership-of-members"' "\"$operators_gid\"" deny-all)" always
ensure_authz permission/scope deny-operator-map-roles \
  "$(grant Users '"map-roles"' "$protected_resources" deny-all)" always
ensure_authz permission/scope iam-map-buyer "$(grant Roles '"map-role"' "\"$buyer_role_id\"" iam-admin-sa)"
ensure_authz permission/scope iam-map-store-roles "$(grant Clients '"map-roles"' "\"$store_client_id\"" iam-admin-sa)"
ensure_authz permission/scope iam-users \
  "$(grant Users '"view", "manage", "map-roles", "manage-group-membership"' '' iam-admin-sa)"
ensure_authz permission/scope iam-organizations "$(grant Organizations '"view", "manage"' '' iam-admin-sa)"

org_json="$(kc get "organizations/$org" -r "$realm")"
store_org_json="$(kc get "organizations/$store_org" -r "$realm")"
memberships="$(kc get "organizations/members/$user/organizations" -r "$realm" --fields id --format csv --noquotes)"
buyer_memberships="$(kc get "organizations/members/$buyer_id/organizations" -r "$realm" --fields id --format csv --noquotes)"
owner_memberships="$(kc get "organizations/members/$owner_id/organizations" -r "$realm" --fields id --format csv --noquotes)"
user_roles="$(kc get-roles -r "$realm" --uid "$user" --effective --fields name --format csv --noquotes)"
buyer_roles="$(kc get-roles -r "$realm" --uid "$buyer_id" --effective --fields name --format csv --noquotes)"
store_roles="$(kc get "clients/$store_client_id/roles" -r "$realm" --fields name --format csv --noquotes)"
owner_group_roles="$(kc get "organizations/$store_org/groups/$owner_gid/role-mappings/clients/$store_client_id" -r "$realm" --fields name --format csv --noquotes)"
owner_group_members="$(kc get "organizations/$store_org/groups/$owner_gid/members" -r "$realm" --fields id --format csv --noquotes)"
iam_sa_roles="$(kc get-roles -r "$realm" --uid "$iam_sa_id" --cclientid realm-management --effective --fields name --format csv --noquotes)"
iam_sa_direct_roles="$(kc get-roles -r "$realm" --uid "$iam_sa_id" --cclientid realm-management --fields name --format csv --noquotes)"
iam_secret="$(kc get "clients/$iam_client_id/client-secret" -r "$realm" --fields value --format csv --noquotes)"
group_mapper_json="$(kc get "clients/$seller_client_id/protocol-mappers/models" -r "$realm" --fields "name,config(addGroupRoleMappings)")"
operators_members="$(kc get "groups/$operators_gid/members" -r "$realm" -q max=1000 --fields id --format csv --noquotes)"
authz="$(kc get "$authz_base/policy" -r "$realm" -q max=-1 --fields id,name,description --format csv --noquotes)"
fgap_enabled="$(kc get "realms/$realm" --fields adminPermissionsEnabled --format csv --noquotes)"

grep -qF "\"tenant_id\" : [ \"$tenant\" ]" <<<"$org_json" || fail "pós-condição falhou: tenant de $org_alias"
grep -qF "\"tenant_id\" : [ \"$store_alias\" ]" <<<"$store_org_json" || fail "pós-condição falhou: tenant de $store_alias"
[ "$memberships" = "$org" ] || fail "pós-condição falhou: $operator membro só de $org_alias"
[ -z "$buyer_memberships" ] || fail "pós-condição falhou: $buyer sem Organization"
[ "$owner_memberships" = "$store_org" ] || fail "pós-condição falhou: $store_owner membro só de $store_alias"
for role in "${operator_roles[@]}"; do
  contains "$user_roles" "$role" || fail "pós-condição falhou: $operator com $role"
done
contains "$buyer_roles" "$buyer_role" || fail "pós-condição falhou: $buyer com $buyer_role"
for role in "${catalog[@]}"; do
  contains "$store_roles" "$role" || fail "pós-condição falhou: $store_client com a role $role"
  contains "$owner_group_roles" "$role" || fail "pós-condição falhou: grupo $owner_group de $store_alias com $role"
done
[ "$(sort <<<"$store_roles")" = "$(sorted "${catalog[@]}")" ] ||
  fail "pós-condição falhou: $store_client só com as roles do catálogo"
[ "$(sort <<<"$owner_group_roles")" = "$(sorted "${catalog[@]}")" ] ||
  fail "pós-condição falhou: grupo $owner_group de $store_alias só com o catálogo"
contains "$owner_group_members" "$owner_id" || fail "pós-condição falhou: $store_owner no grupo $owner_group"
grep -q '"addGroupRoleMappings" : "true"' <<<"$group_mapper_json" ||
  fail "pós-condição falhou: $seller_client com o mapper $group_mapper e addGroupRoleMappings"
[ "$(sort <<<"$iam_sa_direct_roles")" = "$(sorted "${iam_admin_roles[@]}")" ] ||
  fail "pós-condição falhou: $iam_client só com ${iam_admin_roles[*]}"
for role in "${iam_admin_forbidden[@]}"; do
  ! contains "$iam_sa_roles" "$role" || fail "pós-condição falhou: $iam_client sem $role"
done
[ "$fgap_enabled" = true ] || fail "pós-condição falhou: realm $realm com o FGAP v2 ligado"
for protected in "${protected_users[@]}"; do
  contains "$operators_members" "$protected" || fail "pós-condição falhou: $protected no grupo $operators_group"
done
[ "$iam_secret" = "$iam_admin_secret" ] || fail "pós-condição falhou: $iam_client com o segredo de KEYCLOAK_IAM_ADMIN_SECRET"
for expected in "${authz_expected[@]}"; do
  grep -q ",$expected\$" <<<"$authz" || fail "pós-condição falhou: FGAP com ${expected%%,*}"
done
check_client "$client_id" "$client" "${scopes[@]}" >/dev/null
buyer_mappers="$(check_client "$buyer_client_id" "$buyer_client" "${buyer_scopes[@]}")"
contains "$buyer_mappers" "$org_alias-tenant" || fail "pós-condição falhou: $buyer_client com o claim fixo $tenant_claim"
check_client "$seller_client_id" "$seller_client" "${seller_scopes[@]}" >/dev/null
# WHY: o converged.sh compara este marcador com o seed.sh local e roda o seed
# quando o script mudou. O client o perde no começo de cada execução e só o
# recebe depois das pós-condições, então um seed interrompido não o deixa.
kc update "clients/$iam_client_id" -r "$realm" -s "description=seed:$(sha256sum "$0" | cut -c1-16)"
echo "seed-marketplace: realm $realm convergido"
