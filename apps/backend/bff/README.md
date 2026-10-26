# bff

Borda REST/JSON pública do marketplace sobre o kernel DMPF (ADR-012). Sem banco,
outbox nem consumer, o binário tem dois papéis: a API autentica o sujeito pelo
`authn` do kernel, recusa o token de uma sessão revogada pela deny-list do Redis,
resolve o tenant e autoriza por permissão; o papel `session-events` alimenta essa
deny-list com os eventos de sessão do Keycloak.

Portado do `bff` de referência do platform (commit `ddd3d0c`) e podado dos
contextos de lá; a borda gRPC genérica voltou com o primeiro contexto, `tenancy`.
Especificações: `docs/specs/SPEC-DXAWGZ43-platform.md` e
`docs/specs/SPEC-XB4NCW4J-tenancy.md`.

| Rota | Contrato | Exige | Permissão |
| --- | --- | --- | --- |
| `GET /v1/me` | `contract/openapi/v1/openapi.yaml` | sujeito e tenant | `profile` |
| `POST /tenancy/store` | `apps/backend/tenancy/contract/openapi/v1/openapi.yaml` | sujeito, tenant e `Idempotency-Key` | `tenancy:write` |
| `GET /tenancy/store?status=` | idem | sujeito e tenant | `tenancy:read` |
| `GET /tenancy/store/{ref}` | idem | sujeito e tenant | `tenancy:read` |
| `POST /tenancy/store/{ref}/suspend` | idem | sujeito, tenant e `Idempotency-Key` | `tenancy:write` |
| `POST /tenancy/store/{ref}/reactivate` | idem | sujeito, tenant e `Idempotency-Key` | `tenancy:write` |
| `POST /auth/token` (só com `AUTH_TOKEN_CLIENT_ID`) | `contract/openapi/v1/openapi.yaml` | `Idempotency-Key`, sem credencial | — |

As rotas de `tenancy` chamam o `TenancyService` por gRPC (`TENANCY_GRPC_TARGET`,
obrigatória) com transporte declarado: `GRPC_INSECURE=true` só em desenvolvimento,
ou mTLS por `GRPC_CA_FILE`, `GRPC_CLIENT_CERT_FILE` e `GRPC_CLIENT_KEY_FILE`. O
erro gRPC vira HTTP por `rpc.Classify` (404, 400, 403, 409, 429, 503, 504), a
rejeição de domínio vira `422` com o código do contexto, e o replay de
idempotência responde com `Idempotent-Replayed`. O contrato do contexto é servido
em `GET /openapi/tenancy/v1/openapi.yaml` quando `OPENAPI_TENANCY_PATH` está
definido.

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

`POST /auth/token` existe só quando `AUTH_TOKEN_CLIENT_ID` está definida, o que
acontece só em desenvolvimento (compose, `deploy/.env.example` e overlay `dev`);
sem ela, a rota responde `404`. Ela recebe `{"username", "password"}` (sem campo
extra, cada um não vazio, até 255 e 1024 caracteres), faz o password grant no
endpoint de token do `OIDC_ISSUER` pelo client público da variável, com timeout
de 1,5 s e um cliente HTTP reutilizado, e responde `200` com
`{"access_token", "token_type": "Bearer", "expires_in"}` e `Cache-Control:
no-store`. Recusas: corpo fora do contrato → `400`; credencial recusada pelo
Keycloak → `401 invalid-credentials`; Keycloak fora ou sem token → `503
unavailable`; sem resposta a tempo → `504 deadline-exceeded`. A resposta do
Keycloak nunca chega ao chamador, e a senha e o token não entram no access log nem
no span. A rota declara `RequireNeither` (o kernel exige junto o alcance
`ReachAllTenants`, que só vale em rota com sujeito, então ela não lê dado de
tenant) e, como todo `POST` da borda (RST-02), exige `Idempotency-Key`; repetir a
chave só emite outro token.

## Saúde

Na porta de administração (`ADMIN_ADDR`), fora do contrato e sem autenticação:
`GET /livez` responde `204` enquanto o processo serve; `GET /readyz` responde
`204` enquanto serve e `503` depois do `SIGTERM`, durante o dreno, e também sonda
o health gRPC de cada contexto (hoje, `tenancy`). O subcomando `bff healthcheck` consulta o
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
| `OIDC_PERMISSION_CLAIMS` | api | claims das permissões: `scope`, as realm roles e as client roles de `marketplace-store` dentro de `organization.*` (só no tenant da loja) |
| `AUTH_TOKEN_CLIENT_ID` | api | client público do password grant de `POST /auth/token`, só em desenvolvimento; exige `OIDC_ISSUER` |
| `AUTH_DEV_MOCK` | api | identidade lida do próprio Bearer, sem verificação nem consulta à deny-list, só nas suítes de teste; exclusivo com `OIDC_ISSUER` |
| `REDIS_URL` | api, session-events | deny-list de revogação (usuário ACL restrito a `~dmpf:revocation:*`) |
| `OIDC_REVOCATION_TIMEOUT_MS` | api | prazo da consulta à deny-list; estouro recusa o token |
| `OIDC_REVOCATION_TTL_SECONDS` | session-events | validade de cada entrada da deny-list |
| `OIDC_SESSION_EVENTS_CLIENT_ID`, `OIDC_SESSION_EVENTS_CLIENT_SECRET` | session-events | client dos admin events |
| `OIDC_BACKCHANNEL_CLIENT_ID` | session-events | client que envia o back-channel logout |
| `CORS_ORIGINS` | api | origens do Swagger UI local |
| `OTEL_*` | api, session-events | exportação OTLP para o collector do compose |

Configuração inválida faz a partida sair com código 2; descoberta OIDC sem
resposta, com código 1. Os valores de desenvolvimento estão em
`deploy/.env.example` (API em OIDC contra o Keycloak do compose) e
`deploy/.env.session-events.example`; os `.env` copiados deles ficam fora do git.
O `deploy/.env.example` é também o manifesto de telemetria que o
`telemetry_manifest_test.go` confere; o `degradation_test.go` lê o próprio
`app/testdata/edge.env`, a mesma configuração com o `AUTH_DEV_MOCK`.

## Rodar localmente

```bash
pnpm nx run bff:deploy-env         # copia deploy/.env.example para deploy/.env
pnpm nx run bff:redis-up           # Redis do compose local
pnpm nx run bff:keycloak-up        # Keycloak do compose; roda o seed só se o realm divergir
pnpm nx run bff:serve              # sobe Redis, Keycloak e seed (keycloak-up) e o tenancy; API em 127.0.0.1:8080, admin em 127.0.0.1:8090
pnpm nx run bff:serve-session-events
```

No compose, o `deploy/compose.yml` entra pela semente `infra/local/docker-compose.yml`
e o ambiente sobe com os dois profiles:

```bash
docker compose -f infra/local/docker-compose.yml --profile dmpf --profile keycloak up -d
```

`dmpf-bff` está no profile `dmpf`, mas verifica o token pelo Keycloak e espera o
seed `bff-keycloak-seed` terminar; sem o profile `keycloak`, ele não passa da
descoberta OIDC e reinicia. `dmpf-bff-session-events` e `bff-keycloak-seed` sobem
só no profile `keycloak`. Só a porta pública da API é publicada, em
`127.0.0.1:${HTTP_PORT:-8080}`; as portas de administração e o
`/backchannel-logout` ficam na rede interna do compose. No compose e no overlay
`dev`, como no `serve` no host, a API roda em OIDC e a revogação vale; o
`AUTH_DEV_MOCK` fica só nas suítes de teste.

## Operador e token de desenvolvimento

O seed complementar `deploy/keycloak/seed.sh` roda depois do seed gerado
(`infra/keycloak/dev/seed.sh`, saída do dmpf-plugin, que não muda à mão) e
converge o realm `dmpf` para:

- a Organization `marketplace`, com o atributo `tenant_id=["marketplace"]`, o
  tenant do operador (`TENANCY_OPERATOR_TENANT` do `tenancy`);
- as realm roles `tenancy:read`, `tenancy:write`, `iam:read`, `iam:write` e
  `iam:buyer`;
- o usuário `operator`, senha `${KEYCLOAK_SEED_PASSWORD:-dmpf-dev-password}`,
  membro só da `marketplace`, com as quatro roles do operador e no grupo de realm
  `operators`;
- o client público `marketplace-dev`, com password grant e authorization code
  (redirect `http://127.0.0.1:${SWAGGER_PORT:-8082}/oauth2-redirect.html` e o de
  `localhost`, as duas origens no `webOrigins`, sem PKCE obrigatório), os scopes
  `profile` e `organization` por padrão e a audiência `dmpf-bff`;
- o comprador `buyer-demo`, sem Organization e com `iam:buyer`, e o client público
  `marketplace-buyer`, cujo claim fixo `organization.marketplace.tenant_id` dá ao
  comprador o tenant do operador;
- o client confidencial `marketplace-store`, sem fluxo de login, dono das 16 client
  roles do catálogo de permissões de loja da SPEC-2FJ0KYKV e de nenhuma outra (o
  seed exclui a role fora do catálogo);
- a Organization `loja-demo` (`tenant_id=["loja-demo"]`), o grupo `owner` dela com
  as 16 roles e o usuário `loja-demo-owner`, membro só dela e do grupo;
- o client público `marketplace-seller`, sem o scope `roles` e com o mapper
  `organization-groups` (`addGroupRoleMappings`), que leva as roles do grupo para
  `organization.<alias>.resource_access.marketplace-store.roles` só no token do
  lojista; o login pede `scope=openid organization:<alias>`;
- o client confidencial `iam-admin`, só com service account e o segredo de
  `KEYCLOAK_IAM_ADMIN_SECRET`, que o adaptador do `iam` usa na Admin API;
- o grupo de realm `operators`, com as contas que a service account do `iam`
  não pode gerir: o `operator` e as service accounts do `iam-admin` e do
  `dmpf-session-events`.

A service account do `iam` tem exatamente estes papéis de `realm-management`, todos
de leitura (`query-organizations`, `view-organizations`, `view-users`, `view-realm`,
`view-clients`); o seed tira qualquer outro. O que ela altera vem do FGAP v2, que
o seed liga no realm. Ela cria usuários, gere membros e grupos das lojas, mapeia as
roles de `marketplace-store` e concede `iam:buyer`; recebe `403` ao conceder outra
realm role, mexer na Organization `marketplace`, gerir clients e, pelos denies do
grupo `operators` e das contas dele, ao trocar a senha, desabilitar, mover ou
mapear roles do operador e das service accounts, ela mesma incluída. `manage-users`
e `manage-organizations` nunca voltam a ela: os dois liberam tudo antes do FGAP. O
seed reescreve os denies a cada execução e cria os denies antes das concessões. O
que o FGAP não fecha está em "Resultado dos spikes" na SPEC-2FJ0KYKV.

Ele mora no `bff` porque é a borda que verifica o token: o client, a audiência e
a claim de tenant são configuração dela. No compose roda como serviço
`bff-keycloak-seed`; no overlay `dev`, como Job `bff-keycloak-seed` do Component
`deploy/keycloak/`, contra o Keycloak do Component `infra/keycloak/dev`. O Job é
imutável: um seed novo exige apagar o Job antes de aplicar. O segredo do
`iam-admin` vem do Secret `keycloak-iam-admin` (chave `secret`), sem default, que
precisa existir antes do Job:

```bash
kubectl create secret generic keycloak-iam-admin --from-literal=secret="$(openssl rand -hex 24)"
kubectl delete job bff-keycloak-seed --ignore-not-found
kubectl apply -k apps/backend/bff/deploy/k8s/overlays/dev
```

No compose, o `bff-keycloak-seed` usa `${KEYCLOAK_IAM_ADMIN_SECRET:-dmpf-dev-iam-admin}`.

O password grant dispensa navegador, mas entrega a senha à borda, e o
authorization code do Swagger UI gerado não usa PKCE; por isso o
`marketplace-dev` existe só nesse seed, que nunca roda no `hmg`. O `dmpf-portal`
(PKCE obrigatório) segue sendo o client de aplicações com navegador.

O `keycloak-up` (e o `serve`, que depende dele) roda antes o
`deploy/keycloak/converged.sh`: em menos de um segundo, só por HTTP, ele confere o
token do operador pelo `marketplace-dev` (audiência, tenant, roles e scope), o
CORS do endpoint de token, o redirect do Swagger nas duas origens, o token do
comprador (tenant `marketplace` e `iam:buyer`), o do dono da `loja-demo` (tenant
da loja e as 16 roles), o client credentials do `iam-admin` e, com o token dele, o
marcador do `seed.sh` atual, o FGAP ligado com as permissões do seed, os papéis
da service account e as contas do grupo `operators`. O marcador é o digest do
`seed.sh`, gravado no client `iam-admin` só depois das pós-condições: um script
alterado ou uma execução interrompida fazem o seed rodar de novo. Convergido, o
seed não roda; qualquer divergência dispara o seed, que é idempotente: lê o que
existe, cria o que falta e reescreve clients, mappers, senhas e os denies do FGAP
(cerca de 86 chamadas do `kcadm.sh`, de 50 a 70 s conforme a máquina).

### Como obter o token

Pela rota de desenvolvimento do `bff` (o token vale 300 s, o
`accessTokenLifespan` do realm):

```bash
TOKEN="$(curl -s -X POST http://127.0.0.1:8080/auth/token \
  -H 'Content-Type: application/json' -H "Idempotency-Key: token-$(date +%s)" \
  -d '{"username":"operator","password":"dmpf-dev-password"}' | jq -r .access_token)"
curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/v1/me
```

Pelo Swagger UI (`http://127.0.0.1:8082`): **Authorize** → `keycloak (OAuth2,
authorizationCode)`, `client_id` `marketplace-dev`, `client_secret` vazio, marque
o scope `openid` e entre como `operator` na tela do Keycloak. O token volta ao
Swagger e vale para as rotas do `tenancy`. A outra opção é colar em `bearerAuth`,
sem o prefixo `Bearer`, o token obtido pela rota.

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
  parado virando `401`. Sobre o realm gerado, aplica também o
  `deploy/keycloak/seed.sh` (segredo `${KEYCLOAK_IAM_ADMIN_SECRET:-dmpf-test-iam-admin}`),
  e o `app/keycloak_iam_test.go` cobre o comprador com o tenant do operador, as
  roles da loja restritas ao tenant e ao client do lojista e a service account do
  `iam` gerindo a loja sem alcançar o operador nem as service accounts. Roda só localmente: o
  `dmpf-ci.yml` não sobe o Keycloak.

## Kubernetes

`deploy/k8s/base/` traz Deployment, Service, ServiceAccount, ConfigMap,
NetworkPolicy e PDB da API e o par do `session-events`. `overlays/dev` é o
ambiente de desenvolvimento; `overlays/hmg` é só a `kustomization.yaml` sobre a
base, porque o agregador `infra/k8s/overlays/hmg/apps` gerado pelo `infrasync`
referencia o overlay de mesmo nome de toda app. `pnpm nx run bff:k8s-render`
renderiza os dois.

## Notas

- O kernel `grpc` é importado pelo middleware (identificador de correlação e
  validação de locale) e pelo `app/rpc`, que disca os contextos.
- A PDB usa `maxUnavailable: 1`: com uma réplica, `minAvailable: 1` impediria
  qualquer eviction e travaria o `kubectl drain`.
- `app/rpc` traz a borda gRPC genérica do molde (discagem, metadata, replay,
  readiness, `Classify`) e um cliente por contexto; cada contexto novo acrescenta
  o próprio cliente, as rotas e o alvo `<CTX>_GRPC_TARGET`.
- O `gate-check` do DMPF só roda no job `self-tests` do `dmpf-ci.yml`, desligado
  por padrão; rode-o localmente pelo script do plugin (ADR-012).
