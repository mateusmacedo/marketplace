# ADR-012: Borda e infraestrutura alinhadas ao DMPF

## Status

Aceito — 2026-10-23. Implementa SPEC-DXAWGZ43 (DEVS-143).

## Contexto

Os documentos de `docs/product/` descrevem a borda e a infraestrutura do
marketplace com Logto como IdP, validação de JWT no Traefik com headers
internos para os serviços, RabbitMQ como broker e uma API GraphQL ao lado da
REST. A ADR-011 já tinha trocado o Java Quarkus desses documentos pelos
serviços Go do `@mateusmacedo/dmpf-plugin`.

A `1.0.0-rc.7` do plugin traz uma topologia de referência que cobre a mesma
necessidade com outras peças:

- Keycloak 26.8.1 com o realm `dmpf`, Organizations como tenant, os clients
  `dmpf-portal`, `dmpf-bff` e `dmpf-session-events` e um seed de
  desenvolvimento (ADR-064 do platform).
- Verificação do token no próprio processo pelo módulo `authn` do kernel, com
  deny-list de revogação de sessão no Redis (ADR-065 do platform).
- Adaptadores de broker `kafka` e `sqs` no kernel; o compose gerado sobe
  Redpanda.
- Borda REST/JSON com contrato OpenAPI por contexto e contextos atrás dela em
  gRPC (ADR-044 do platform), sem suporte a GraphQL.

O plugin só tem os generators `init` e `bounded-context`, sem generator de
borda. O platform tem um `bff` de referência no commit
`ddd3d0c4202879f168c7f6bfafaefadd94a3a621` (o `workflowRef` da rc.7).

Na rc.7, dois gates do DMPF não verificam nada num workspace sem bounded
context. O `dmpf-gate-check.sh` reprova com "nenhum provider de contexto
encontrado", e o reutilizável do `dmpf-ci.yml` o ativa sempre que um PR toca
`.golangci.yml` ou um `*dmpf-units.json`. O `dmpf-context-check.sh` encerra
antes da fase de bordas quando não há contexto, então não confere o `bff`.

## Decisão

**A borda e a infraestrutura do marketplace seguem o DMPF.**

- **IdP**: Keycloak de referência gerado pelo `init`, sem extensão. Os papéis de
  negócio ficam com o RBAC da spec de `iam`; a borda só autentica e exige a
  permissão declarada em cada rota.
- **Verificação do token no `bff`**: o `authn` valida assinatura, `iss`, `aud`,
  `exp` e `sub` e resolve o tenant por `organization.*.tenant_id`. Os headers
  `X-Subject-ID` e `X-Tenant-ID` e a query `tenant_id` são asserções do cliente:
  divergência com o token é recusada com 403 `identity-mismatch`.
- **Revogação com falha fechada**: cada token aceito é consultado na deny-list
  do Redis; Redis fora ou lento além de `OIDC_REVOCATION_TIMEOUT_MS` recusa o
  token com 401.
- **Broker Redpanda (Kafka)**, pelo adaptador `kafka` do kernel.
- **Borda REST** no `bff` (`apps/backend/bff`), portado do `bff` de referência e
  podado dos contextos do platform. O pacote `app/rpc` nasce só com o `doc.go`,
  porque o `context-check` o exige em toda borda; os clientes gRPC chegam com
  cada contexto. Sem generator, o `go.mod`, o `project.json` e o
  `dmpf-units.json` do `bff` são escritos à mão, seguindo o template
  `bounded-context` da rc.7.
- **Traefik e GraphQL ficam fora do MVP.** No compose local, o `bff` publica em
  `127.0.0.1`; TLS, roteamento por host e rate limit voltam à pauta com um
  ambiente remoto.
- **`gate-check` vermelho até o primeiro contexto**: o `dmpf-ci.yml` reprova no
  `gate-check` enquanto não houver bounded context, o mesmo estado já herdado do
  template (ADR-011). A spec de `tenancy` (`SPEC-XB4NCW4J`) é a primeira em que
  o `gate-check` e a fase de bordas do `context-check` passam a valer.

## Alternativas descartadas

- **Logto**: perde o realm, o seed e a revogação de referência e exige uma
  integração própria fora do kernel.
- **JWT validado no Traefik com headers internos**: contraria o `authn`, que
  trata esses headers como asserção do cliente, e não existe no DMPF.
- **RabbitMQ**: o kernel não tem adaptador.
- **GraphQL**: o kernel não tem suporte, e a topologia de referência é REST.
- **Mover o `bff` para a spec de `tenancy`**: deixaria os gates verdes no mesmo
  PR do `bff`, mas adiaria a prova de autenticação e de revogação para uma spec
  que já carrega o primeiro contexto inteiro.
- **Remover o `app/rpc`**: quebraria o `context-check` assim que o primeiro
  contexto existir.

## Consequências

**Positivas:**

- A borda nasce com autenticação, tenant e revogação do kernel, provadas contra
  o Keycloak de referência (`pnpm nx run bff:test-keycloak`).
- Cada contexto do roadmap só acrescenta as próprias rotas e o cliente gRPC no
  `bff`; o `edge: bff` do `dmpf.json` já o libera nos contextos.
- Toda a infraestrutura continua vindo do plugin e muda só por `nx migrate`.

**Negativas:**

- `docs/product/` diverge da implementação em IdP, broker, borda e API; vale
  esta ADR.
- O `dmpf-ci.yml` segue vermelho no `gate-check` até `tenancy`, e nenhum gate do
  DMPF verifica a estrutura do `bff` antes disso.
- O teste com Keycloak real roda só localmente: o `dmpf-ci.yml` gerado não sobe
  o Keycloak, e o CI cobre o mesmo comportamento com um issuer falso.
- O `bff` fica fora da release enquanto a release Docker cobrir só apps fora de
  Go (`!tag:stack:go`).

## Referências

- SPEC-DXAWGZ43 — `docs/specs/SPEC-DXAWGZ43-platform.md`
- [ADR-011](./011-template-consome-dmpf-por-versao.md)
- ADRs 044, 064 e 065 do platform no commit `ddd3d0c` —
  `github.com/mateusmacedo/dmpf/tree/ddd3d0c4202879f168c7f6bfafaefadd94a3a621/docs/adr`
- `bff` de referência —
  `github.com/mateusmacedo/dmpf/tree/ddd3d0c4202879f168c7f6bfafaefadd94a3a621/apps/backend/bff`
