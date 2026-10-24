#!/usr/bin/env bash
# Sai 0 só quando o realm dmpf já está como o seed.sh o deixa, conferido pelo
# HTTP do Keycloak, sem subir JVM: o token do operador pelo client
# marketplace-dev carrega audiência, tenant, roles e scope que o bff exige, o
# endpoint de token responde CORS às origens do Swagger UI e o authorize aceita
# os redirects dele. Qualquer divergência sai 1 e o keycloak-up roda o seed.
set -euo pipefail

keycloak_url="${KEYCLOAK_URL:-http://keycloak.localhost:${KEYCLOAK_PORT:-8180}}"
password="${KEYCLOAK_SEED_PASSWORD:-dmpf-dev-password}"
swagger_port="${SWAGGER_PORT:-8082}"
realm_url="$keycloak_url/realms/dmpf/protocol/openid-connect"
client=marketplace-dev

diverges() {
  echo "keycloak-converged: $1; o seed vai rodar" >&2
  exit 1
}

for tool in curl jq base64; do
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

jq -e '
  ((if (.aud | type) == "array" then .aud else [.aud] end) | index("dmpf-bff")) != null
  and ([.organization[]?.tenant_id[]?] == ["marketplace"])
  and ((.realm_access.roles // []) | (index("tenancy:read") != null and index("tenancy:write") != null))
  and (((.scope // "") | split(" ")) | index("profile")) != null
' <<<"$claims" >/dev/null || diverges "claims do token fora do que o bff exige"
echo "keycloak-converged: realm dmpf já convergido"
