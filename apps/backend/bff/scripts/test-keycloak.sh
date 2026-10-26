#!/usr/bin/env bash
# Testes do bff contra o Keycloak de testes (infra/test/compose.yml, profile
# keycloak, 127.0.0.1:18180) e o Redis de testes (16379), com o seed aplicado.
# Só estes dois sobem, e os dois descem ao fim mesmo quando um teste falha; a
# rede do projeto Compose só desce se nada mais dele estiver de pé.
set -euo pipefail
root="${DMPF_WORKSPACE_ROOT:-$(git rev-parse --show-toplevel)}"
compose=(docker compose -f "$root/infra/test/compose.yml" --profile keycloak --profile init)
marketplace_seed="$root/apps/backend/bff/deploy/keycloak/seed.sh"
export KEYCLOAK_SEED_PASSWORD="${KEYCLOAK_SEED_PASSWORD:-dmpf-dev-password}"
export KEYCLOAK_IAM_ADMIN_SECRET="${KEYCLOAK_IAM_ADMIN_SECRET:-dmpf-test-iam-admin}"

teardown() {
  "${compose[@]}" rm --stop --force --volumes keycloak redis >/dev/null 2>&1 || true
  if [[ -z "$("${compose[@]}" ps --quiet)" ]]; then
    "${compose[@]}" down --remove-orphans >/dev/null 2>&1 || true
  fi
}
trap teardown EXIT

"${compose[@]}" up --detach --wait keycloak redis
"${compose[@]}" run --rm keycloak-init
# WHY: os testes do iam verificam o realm que o marketplace usa, então o seed da
# borda roda sobre o gerado, na mesma imagem e rede do keycloak-init.
"${compose[@]}" run --rm -e KEYCLOAK_SEED_PASSWORD -e KEYCLOAK_IAM_ADMIN_SECRET \
  -v "$marketplace_seed:/seed/marketplace.sh:ro" --entrypoint bash keycloak-init /seed/marketplace.sh

go test -race -count=1 -p 1 -tags=integration,keycloak -run 'Keycloak' ./...
