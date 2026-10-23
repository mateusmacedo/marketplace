#!/usr/bin/env bash
# Imprime só o access token do operador do marketplace, emitido pelo Keycloak do
# compose local para o client marketplace-dev (deploy/keycloak/seed.sh). Serve
# para o bearerAuth do Swagger UI e para chamadas manuais ao bff:
#   TOKEN="$(bash apps/backend/bff/scripts/operator-token.sh)"
# Só de desenvolvimento: o client aceita a senha direto no endpoint de token.
set -euo pipefail

keycloak_url="${KEYCLOAK_URL:-http://keycloak.localhost:${KEYCLOAK_PORT:-8180}}"
realm="${KEYCLOAK_REALM:-dmpf}"
client_id="${KEYCLOAK_CLIENT_ID:-marketplace-dev}"
username="${KEYCLOAK_USERNAME:-operator}"
password="${KEYCLOAK_SEED_PASSWORD:-dmpf-dev-password}"

for tool in curl jq; do
  command -v "$tool" >/dev/null || { echo "operator-token: $tool não encontrado" >&2; exit 1; }
done

response="$(curl --silent --show-error --max-time 10 \
  --data-urlencode grant_type=password \
  --data-urlencode "client_id=$client_id" \
  --data-urlencode "username=$username" \
  --data-urlencode "password=$password" \
  "$keycloak_url/realms/$realm/protocol/openid-connect/token")" || {
  echo "operator-token: Keycloak inacessível em $keycloak_url (suba o compose com --profile keycloak)" >&2
  exit 1
}

token="$(jq -r '.access_token // empty' <<<"$response" 2>/dev/null || true)"
if [ -z "$token" ]; then
  reason="$(jq -r '.error_description // .error // empty' <<<"$response" 2>/dev/null || true)"
  echo "operator-token: token recusado: ${reason:-$response}" >&2
  exit 1
fi
printf '%s\n' "$token"
