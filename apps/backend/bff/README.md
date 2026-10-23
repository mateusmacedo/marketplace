# bff

Borda REST/JSON pública do marketplace sobre o kernel DMPF (ADR-012). Sem banco,
outbox nem consumer, o binário tem dois papéis: a API autentica o sujeito pelo
`authn` do kernel, recusa o token de uma sessão revogada pela deny-list do Redis,
resolve o tenant e autoriza por permissão; o papel `session-events` alimenta essa
deny-list com os eventos de sessão do Keycloak.

Portado do `bff` de referência do platform (commit `ddd3d0c`) e podado dos
contextos de lá. Especificação: `docs/specs/SPEC-DXAWGZ43-platform.md`.

| Rota | Contrato | Exige | Permissão |
| --- | --- | --- | --- |
| `GET /v1/me` | `contract/openapi/v1/openapi.yaml` | sujeito e tenant | `profile` |

`GET /v1/me` devolve `200` com `{"subject": "<sub>", "tenant": "<tenant_id>"}`
lidos do contexto de execução que o `authn` montou. Recusas, com o corpo
`{"code", "message"}` do próprio `bff` (`app/api/handlers.go`):

- token ausente, inválido, expirado ou de sessão revogada → `401`;
- token sem organização, ou com mais de uma → `403 tenant-unresolved`;
- token sem a permissão `profile` (claims `scope` e `realm_access.roles`) → `403 permission-denied`;
- `X-Tenant-ID`, `X-Subject-ID` ou query `tenant_id` divergente do token → `403 identity-mismatch`;
- header `Idempotency-Key` presente e fora do padrão do kernel → `400 invalid-idempotency-key`.

Toda rota passa por `Route.ValidateEdge()` na partida: uma rota que exige sujeito
sem declarar permissão faz o processo sair com código 2.

## Saúde

Na porta de administração (`ADMIN_ADDR`), fora do contrato e sem autenticação:
`GET /livez` responde `204` enquanto o processo serve; `GET /readyz` responde
`204` enquanto serve e `503` depois do `SIGTERM`, durante o dreno. Sem contexto
ainda, o `/readyz` não sonda gRPC. O subcomando `bff healthcheck` consulta o
`/readyz` e é o `HEALTHCHECK` da imagem.

## Papel `session-events`

`bff session-events` roda como processo à parte, com uma réplica. Ele recebe o
back-channel logout do client `dmpf-portal` em `POST /backchannel-logout`
(`SESSION_EVENTS_ADDR`) e lê os admin events do Keycloak com o client
`dmpf-session-events` a cada `OIDC_SESSION_EVENTS_INTERVAL_SECONDS`. Os dois
caminhos gravam na deny-list que a API consulta. Tem a própria porta de
administração, e o `/readyz` só cai para `503` no `SIGTERM`.

## Configuração

| Variável | Papel | Uso |
| --- | --- | --- |
| `HTTP_ADDR` | api | porta pública |
| `ADMIN_ADDR` | api, session-events | `/livez` e `/readyz` |
| `SESSION_EVENTS_ADDR` | session-events | `POST /backchannel-logout` |
| `OIDC_ISSUER` | api, session-events | issuer do Keycloak |
| `OIDC_AUDIENCE` | api | audiência exigida no token |
| `OIDC_TENANT_CLAIM` | api | claim do tenant (`organization.*.tenant_id`) |
| `AUTH_DEV_MOCK` | api | identidade lida do próprio Bearer, sem verificação nem consulta à deny-list, só em desenvolvimento; exclusivo com `OIDC_ISSUER` |
| `REDIS_URL` | api, session-events | deny-list de revogação (usuário ACL restrito a `~dmpf:revocation:*`) |
| `OIDC_REVOCATION_TIMEOUT_MS` | api | prazo da consulta à deny-list; estouro recusa o token |
| `OIDC_REVOCATION_TTL_SECONDS` | session-events | validade de cada entrada da deny-list |
| `OIDC_SESSION_EVENTS_CLIENT_ID`, `OIDC_SESSION_EVENTS_CLIENT_SECRET` | session-events | client dos admin events |
| `OIDC_BACKCHANNEL_CLIENT_ID` | session-events | client que envia o back-channel logout |
| `CORS_ORIGINS` | api | origens do Swagger UI local |
| `OTEL_*` | api, session-events | exportação OTLP para o collector do compose |

Configuração inválida faz a partida sair com código 2; descoberta OIDC sem
resposta, com código 1. Os valores de desenvolvimento estão em
`deploy/.env.example` (API com `AUTH_DEV_MOCK`) e
`deploy/.env.session-events.example`; os `.env` copiados deles ficam fora do git.

## Rodar localmente

```bash
pnpm nx run bff:deploy-env         # copia deploy/.env.example para deploy/.env
pnpm nx run bff:redis-up           # Redis do compose local
pnpm nx run bff:serve              # API em 127.0.0.1:8080, admin em 127.0.0.1:8090

pnpm nx run bff:keycloak-up        # Keycloak do compose local (profile keycloak)
pnpm nx run bff:serve-session-events
```

No compose, o `deploy/compose.yml` entra pela semente `infra/local/docker-compose.yml`:
`dmpf-bff` sobe nos profiles do DMPF e `dmpf-bff-session-events` só no profile
`keycloak`, porque depende dele. Só a porta pública da API é publicada, em
`127.0.0.1:${HTTP_PORT:-8080}`; as portas de administração e o
`/backchannel-logout` ficam na rede interna do compose. No compose e no overlay
`dev`, a API roda com `AUTH_DEV_MOCK`; a revogação só vale com OIDC, como no
`serve` com o `OIDC_ISSUER` configurado e no `test-keycloak`.

## Targets Nx

`tidy`, `fmt-check`, `vet`, `build`, `test-race`, `test-infra-up`,
`test-infra-down`, `test-keycloak`, `govulncheck`, `serve`,
`serve-session-events`, `deploy-env`, `redis-up`, `keycloak-up`,
`docker:build`, `docker:run` e `k8s-render`. `lint`, `test` e `generate` vêm do
`@nx-go/nx-go`. O `bff` não entra na release: ela cobre só libs e apps fora de Go.

## Testes

- `pnpm nx run bff:test-race`: unitários e integração (`-tags=integration`) contra
  o Redis de testes (`127.0.0.1:16379`), com issuer falso. Cobre as recusas de
  `GET /v1/me`, a revogação e a falha fechada com o Redis fora.
- `pnpm nx run bff:test-keycloak`: testes com tags `integration && keycloak`
  contra o Keycloak de testes (`127.0.0.1:18180`). O script
  `scripts/test-keycloak.sh` sobe o Keycloak e o Redis, aplica o seed e os derruba
  ao final, mesmo com falha. Cobre a `alice` do seed recebendo `200`, o logout
  virando `401 session_revoked` dentro do intervalo do `session-events` e o Redis
  parado virando `401`. Roda só localmente: o `dmpf-ci.yml` não sobe o Keycloak.

## Kubernetes

`deploy/k8s/base/` traz Deployment, Service, ServiceAccount, ConfigMap,
NetworkPolicy e PDB da API e o par do `session-events`. `overlays/dev` é o
ambiente de desenvolvimento; `overlays/hmg` é só a `kustomization.yaml` sobre a
base, porque o agregador `infra/k8s/overlays/hmg/apps` gerado pelo `infrasync`
referencia o overlay de mesmo nome de toda app. `pnpm nx run bff:k8s-render`
renderiza os dois.

## Notas

- O kernel `grpc` é importado só pelo middleware (identificador de correlação e
  validação de locale), não para chamar contextos.
- A PDB usa `maxUnavailable: 1`: com uma réplica, `minAvailable: 1` impediria
  qualquer eviction e travaria o `kubectl drain`.
- `app/rpc` contém só o `doc.go`: o `context-check` do DMPF exige o pacote em toda
  borda, e os clientes gRPC chegam com cada contexto, a partir de `tenancy`.
- Enquanto não houver bounded context, o `gate-check` do `dmpf-ci.yml` reprova por
  falta de provider de contexto, e o `context-check` não chega à fase de bordas
  (ADR-012).
