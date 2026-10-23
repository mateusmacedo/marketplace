#!/usr/bin/env bash
# Testes do bff contra o Keycloak de testes (infra/test/compose.yml, profile
# keycloak, 127.0.0.1:18180) e o Redis de testes (16379), com o seed aplicado.
# Só estes dois sobem, e os dois descem ao fim mesmo quando um teste falha; a
# rede do projeto Compose só desce se nada mais dele estiver de pé.
set -euo pipefail
root="${DMPF_WORKSPACE_ROOT:-$(git rev-parse --show-toplevel)}"
compose=(docker compose -f "$root/infra/test/compose.yml" --profile keycloak --profile init)

teardown() {
  "${compose[@]}" rm --stop --force --volumes keycloak redis >/dev/null 2>&1 || true
  if [[ -z "$("${compose[@]}" ps --quiet)" ]]; then
    "${compose[@]}" down --remove-orphans >/dev/null 2>&1 || true
  fi
}
trap teardown EXIT

"${compose[@]}" up --detach --wait keycloak redis
"${compose[@]}" run --rm keycloak-init

go test -race -count=1 -p 1 -tags=integration,keycloak -run 'Keycloak' ./...
