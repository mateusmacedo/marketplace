#!/usr/bin/env bash
# Sai 0 só quando o realm dmpf já está como o seed.sh o deixa, conferido pelo
# HTTP do Keycloak, sem subir JVM: o token do operador pelo client
# marketplace-dev carrega audiência, tenant, roles e scope que o bff exige, o
# endpoint de token responde CORS às origens do Swagger UI e o authorize aceita
# os redirects dele; o comprador sai com o tenant do operador, o dono da
# loja-demo com o catálogo da loja e a service account do iam com o segredo,
# só os papéis de leitura, as contas protegidas no grupo operators e as
# permissões do FGAP v2; o client iam-admin traz o marcador do seed.sh atual.
# Qualquer divergência sai 1 e o keycloak-up roda o seed.
set -euo pipefail

keycloak_url="${KEYCLOAK_URL:-http://keycloak.localhost:${KEYCLOAK_PORT:-8180}}"
password="${KEYCLOAK_SEED_PASSWORD:-dmpf-dev-password}"
iam_admin_secret="${KEYCLOAK_IAM_ADMIN_SECRET:-dmpf-dev-iam-admin}"
swagger_port="${SWAGGER_PORT:-8082}"
realm_url="$keycloak_url/realms/dmpf/protocol/openid-connect"
client=marketplace-dev

diverges() {
  echo "keycloak-converged: $1; o seed vai rodar" >&2
  exit 1
}

for tool in curl jq base64 sha256sum; do
  command -v "$tool" >/dev/null || diverges "$tool não encontrado"
done

payload() {
  local segment
  segment="$(cut -d. -f2 <<<"$1" | tr '_-' '/+')"
  while [ $((${#segment} % 4)) -ne 0 ]; do segment+="="; done
  base64 -d <<<"$segment"
}

claims=""
for origin in "http://127.0.0.1:$swagger_port" "http://localhost:$swagger_port"; do
  # WHY: a última linha do write-out separa o header de CORS do corpo JSON.
  answer="$(curl --silent --max-time 5 -H "Origin: $origin" \
    --write-out '\n%header{access-control-allow-origin}' \
    --data-urlencode grant_type=password --data-urlencode "client_id=$client" \
    --data-urlencode username=operator --data-urlencode "password=$password" \
    "$realm_url/token")" || diverges "Keycloak inacessível em $keycloak_url"
  allowed="$(tail -n 1 <<<"$answer")"
  token="$(sed '$d' <<<"$answer" | jq -r '.access_token // empty' 2>/dev/null || true)"
  [ -n "$token" ] || diverges "sem token do operador pelo $client com a origem $origin"
  [ "$allowed" = "$origin" ] || diverges "endpoint de token sem CORS para $origin"
  [ -n "$claims" ] || claims="$(payload "$token")"

  status="$(curl --silent --max-time 5 --output /dev/null --write-out '%{http_code}' --get \
    --data-urlencode "client_id=$client" --data-urlencode response_type=code --data-urlencode scope=openid \
    --data-urlencode "redirect_uri=$origin/oauth2-redirect.html" "$realm_url/auth")" || status=000
  [ "$status" = 200 ] || diverges "authorize recusa o redirect de $origin ($status)"
done

audience='((if (.aud | type) == "array" then .aud else [.aud] end) | index("dmpf-bff")) != null'

jq -e "$audience"'
  and ([.organization[]?.tenant_id[]?] == ["marketplace"])
  and ((.realm_access.roles // []) | (index("tenancy:read") != null and index("tenancy:write") != null
    and index("iam:read") != null and index("iam:write") != null))
  and (((.scope // "") | split(" ")) | index("profile")) != null
' <<<"$claims" >/dev/null || diverges "claims do token do operador fora do que o bff exige"

token_of() {
  curl --silent --max-time 5 "$@" "$realm_url/token" | jq -r '.access_token // empty' 2>/dev/null || true
}

buyer_token="$(token_of --data-urlencode grant_type=password --data-urlencode client_id=marketplace-buyer \
  --data-urlencode username=buyer-demo --data-urlencode "password=$password")"
[ -n "$buyer_token" ] || diverges "sem token do buyer-demo pelo marketplace-buyer"
jq -e "$audience"'
  and (.organization | keys) == ["marketplace"] and .organization.marketplace.tenant_id == "marketplace"
  and ((.realm_access.roles // []) | index("iam:buyer")) != null
' <<<"$(payload "$buyer_token")" >/dev/null || diverges "claims do token do comprador fora do que o bff exige"

owner_token="$(token_of --data-urlencode grant_type=password --data-urlencode client_id=marketplace-seller \
  --data-urlencode username=loja-demo-owner --data-urlencode "password=$password" \
  --data-urlencode "scope=openid organization:loja-demo")"
[ -n "$owner_token" ] || diverges "sem token do loja-demo-owner pelo marketplace-seller"
jq -e "$audience"'
  and ([.organization[]?.tenant_id[]?] == ["loja-demo"])
  and ((.organization["loja-demo"].resource_access["marketplace-store"].roles // []) | length) == 16
' <<<"$(payload "$owner_token")" >/dev/null || diverges "claims do token do dono da loja-demo fora do que o bff exige"

iam_token="$(token_of --data-urlencode grant_type=client_credentials --data-urlencode client_id=iam-admin \
  --data-urlencode "client_secret=$iam_admin_secret")"
[ -n "$iam_token" ] || diverges "sem token da service account iam-admin"

admin_get() {
  curl --silent --fail --max-time 5 -H "Authorization: Bearer $iam_token" "$keycloak_url/admin/realms/dmpf$1" || true
}

id_of() { jq -r "${1:-.[0].id} // empty" 2>/dev/null || true; }

# WHY: o seed grava no client o digest do script só depois das pós-condições;
# um seed.sh alterado ou uma execução interrompida não batem com ele.
marker="seed:$(sha256sum "$(dirname "${BASH_SOURCE[0]}")/seed.sh" | cut -c1-16)"
iam_client="$(admin_get "/clients?clientId=iam-admin")"
[ "$(id_of '.[0].description' <<<"$iam_client")" = "$marker" ] ||
  diverges "client iam-admin sem o marcador do seed.sh atual"

[ "$(admin_get "" | jq -r '.adminPermissionsEnabled // false' 2>/dev/null)" = true ] ||
  diverges "realm dmpf sem o FGAP v2 que limita a service account iam-admin"
admin_permissions="$(admin_get "/clients?clientId=admin-permissions" | id_of)"
[ -n "$admin_permissions" ] || diverges "service account iam-admin sem leitura do client admin-permissions"
jq -e '(["iam-admin-sa", "deny-all", "iam-map-buyer", "iam-map-store-roles", "iam-users", "iam-organizations",
  "deny-operator-organization", "deny-operators-membership", "deny-operator-map-roles"] - [.[].name]) == []' \
  <<<"$(admin_get "/clients/$admin_permissions/authz/resource-server/policy?max=-1")" >/dev/null 2>&1 ||
  diverges "FGAP do realm dmpf sem as permissões da service account iam-admin"

sa_of() { admin_get "/clients/$1/service-account-user" | id_of .id; }
iam_sa="$(sa_of "$(id_of <<<"$iam_client")")"
realm_management="$(admin_get "/clients?clientId=realm-management" | id_of)"
[ -n "$iam_sa" ] && [ -n "$realm_management" ] || diverges "service account iam-admin sem leitura de si mesma"
jq -e '[.[].name] | sort == ["query-organizations", "view-clients", "view-organizations", "view-realm", "view-users"]' \
  <<<"$(admin_get "/users/$iam_sa/role-mappings/clients/$realm_management")" >/dev/null 2>&1 ||
  diverges "service account iam-admin com papéis de realm-management fora dos de leitura"
jq -e '[.[].name] - ["manage-users", "manage-organizations", "realm-admin", "manage-realm", "manage-clients",
  "manage-authorization", "manage-identity-providers", "impersonation"] == [.[].name]' \
  <<<"$(admin_get "/users/$iam_sa/role-mappings/clients/$realm_management/composite")" >/dev/null 2>&1 ||
  diverges "service account iam-admin com papel proibido por composite"

operators="$(admin_get "/groups?search=operators&exact=true" | id_of '.[] | select(.name == "operators") | .id')"
[ -n "$operators" ] || diverges "realm dmpf sem o grupo operators"
members="$(admin_get "/groups/$operators/members?max=1000" | jq -r '.[].id' 2>/dev/null || true)"
for protected in "$(admin_get "/users?username=operator&exact=true" | id_of)" "$iam_sa" \
  "$(sa_of "$(admin_get "/clients?clientId=dmpf-session-events" | id_of)")"; do
  [ -n "$protected" ] && grep -qx -- "$protected" <<<"$members" ||
    diverges "conta protegida fora do grupo operators"
done
echo "keycloak-converged: realm dmpf já convergido"
