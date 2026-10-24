---
id: SPEC-XB4NCW4J
slug: tenancy
title: Bounded context — tenancy
stage: done
priority: P0
depends_on: [SPEC-DXAWGZ43]
ticket_url: https://linear.app/mmda/issue/DEVS-144/spec-xb4ncw4j-bounded-context-tenancy
subtask_urls: []
created: 2026-10-21
---

# SPEC-XB4NCW4J: Bounded context — tenancy

Registro das lojas de sellers, que são os tenants da plataforma, e o ciclo de
vida de cada uma (ativa/suspensa). Fase alvo: **MVP**. Primeiro bounded
context do marketplace: com ele, a fase de bordas do `dmpf-context-check.sh`
passa a conferir o `bff` (ADR-012).

## Contexto

- **Fase**: MVP — ver `docs/roadmap.md`.
- **Depende de**: SPEC-DXAWGZ43 (`platform`).
- **Fontes**:
  - `docs/product/Bounded Context/Bounded Context - Configuração do Tenant (Tenant Configuration).md`
  - `docs/product/Bounded Context/Bounded Context - Identidade e Acesso (IAM).md` (agregado Tenant)
  - `docs/product/01_identity_organization.md` (vocabulário de tenant)
  - `docs/product/Fundamentos - Conceitos White Label, Multi-Tenant e Visão Arquitetural Inicial.md`
  - `docs/adr/012-borda-e-infra-alinhadas-ao-dmpf.md`
  - ADRs 051, 053, 056, 064, 066 e 068 do platform no commit
    `ddd3d0c4202879f168c7f6bfafaefadd94a3a621`

### Decisões do detalhamento

O modelo de tenant abaixo é decisão de arquitetura para todos os contextos e
está registrado na ADR-013 (`docs/adr/013-tenant-e-a-loja-do-seller.md`).

- **Tenant é a loja do seller**, como no PRD 01. O Fundamentos e o BC IAM
  usam "tenant" para a organização white label; aqui esse papel é do
  **operador**, que não é uma loja registrada (ver *Linguagem ubíqua*).
- **Tenant do operador**: uma organização própria do marketplace guarda os
  dados de quem não pertence a uma loja — compradores, carrinho e pedido — e o
  próprio registro de lojas deste contexto. O sub-pedido e a reserva de estoque
  chegam à loja por projeção declarada (`ports.Projector`, ADR-068 do
  platform), definida nas specs de `ordering` e `inventory`.
- **Isolamento por discriminador**: `tenant_id` em toda tabela de todo
  contexto, imposto pelo choke point do kernel (ADR-051 do platform), como já
  fixado nas decisões de base do roadmap. Schema e banco por tenant ficam fora.
- **Resolução do tenant pelo Keycloak**: a Organization de cada loja tem o
  atributo `tenant_id` igual à `ref` da loja; o `authn` do `bff` o lê por
  `organization.*.tenant_id` (ADR-012; ADR-064 do platform). As menções a
  Logto e `org_id` de `docs/product/` não valem.
- **O registro vive no escopo do operador**: a rc.7 não tem escrita de
  plataforma com alcance total (ADR-068 do platform, item 14), então o
  operador grava o registro no próprio tenant, onde cada linha é uma loja.
  Listar as lojas é consulta comum, sem `PlatformReader`.
- **O agregado se chama `Store`, não `Tenant`**: a norma do DMPF nomeia a
  coluna de identidade `<agregado>_id`, e `tenant_id` já é o discriminador.
- **Fronteira com `iam` e `seller`**: o agregado Tenant sai do BC IAM de
  `docs/product/` e vira `Store` aqui; `iam` (SPEC-2FJ0KYKV) cuida de
  usuários, papéis e vínculo com tenant. Dados fiscais e KYB do seller são de
  `seller`.

## Identidade

- **bounded_context**: `tenancy`
- **name**: `tenancy`
- **Linguagem ubíqua**:
  - *Tenant* — unidade de isolamento de dados, a que pertence cada linha de
    cada contexto; é uma loja ou o operador.
  - *Loja* (`Store`) — o tenant de um seller, registrado neste contexto.
  - *Referência* (`ref`) — o identificador da loja: o valor do atributo
    `tenant_id` da Organization no Keycloak e o `tenant_id` que todo contexto
    grava para aquela loja.
  - *Operador* — o tenant reservado do marketplace, configurado e não
    registrado; guarda compradores, carrinho, pedido e o registro de lojas.
  - *Registro* — o ato de tornar uma loja conhecida da plataforma.
  - *Suspensão* — o ato de interromper a operação de uma loja ativa, com um
    motivo.
  - *Reativação* — o ato de devolver uma loja suspensa à operação.

## Agregados

### Store

- **Identidade**: `natural` por `ref`
- **Campos**:
  - `ref: string`
  - `name: string`
  - `status: active | suspended`
  - `suspensionReason: string`
  - `registeredAt: instant`
  - `statusChangedAt: instant`
- **Estados**: `active`, `suspended`
- **Transições permitidas**:
  - (novo) → `active` — por `Register`
  - `active` → `suspended` — por `Suspend`
  - `suspended` → `active` — por `Reactivate`
- **Invariantes de estado**:
  - `ref` tem de 3 a 63 caracteres em `a-z`, `0-9` e `-`, sem hífen no início
    nem no fim — `Register` rejeita com `tenancy/store/ref-invalid`. A forma
    de rótulo DNS mantém a `ref` usável em subdomínio na V2.
  - `name` nunca é vazio — `Register` rejeita com `tenancy/store/name-empty`.
  - `suspensionReason` não é vazio quando `status = suspended` e é vazio
    quando `status = active` — `Suspend` rejeita motivo vazio com
    `tenancy/store/reason-empty`, e `Reactivate` o limpa.
  - `statusChangedAt` é o `at` da última transição; não há rejeição temporal,
    porque o relógio do kernel não é monotônico.
- **Unicidade de negócio**: `name` único por tenant (no escopo do operador,
  por loja), por igualdade exata — `tenancy/store/duplicate-name`
  (ADR-066 do platform).

## Comandos (UPRs)

### Register

- **Agregado**: `Store`; **inicializa se ausente**
- **Comando**: `RegisterStore`
- **Entrada**:
  - `ref: string`
  - `name: string`
- **Pré-condições**:
  - `ref` na forma do invariante → `tenancy/store/ref-invalid`
  - `name` não vazio → `tenancy/store/name-empty`
- **Efeitos sobre o estado**:
  - quando ausente: `name ← cmd.name`, `status ← active`,
    `suspensionReason ← ""`, `registeredAt ← at`, `statusChangedAt ← at`
  - quando presente: nenhum — aceita sem alterar, inclusive o nome e o status,
    e sem emitir
- **Eventos emitidos**: `StoreRegistered` (só quando inicializa)
- **Resposta**: `ref: string`

### Suspend

- **Agregado**: `Store`; exige existente
- **Comando**: `SuspendStore`
- **Entrada**:
  - `reason: string`
- **Pré-condições**:
  - `status = active` → `tenancy/store/not-active`
  - `reason` não vazio → `tenancy/store/reason-empty`
- **Efeitos sobre o estado**:
  - `status ← suspended`
  - `suspensionReason ← cmd.reason`
  - `statusChangedAt ← at`
- **Eventos emitidos**: `StoreSuspended`
- **Resposta**: `ref: string`

### Reactivate

- **Agregado**: `Store`; exige existente
- **Comando**: `ReactivateStore`
- **Entrada**: nenhuma além da identidade do agregado
- **Pré-condições**:
  - `status = suspended` → `tenancy/store/not-suspended`
- **Efeitos sobre o estado**:
  - `status ← active`
  - `suspensionReason ← ""`
  - `statusChangedAt ← at`
- **Eventos emitidos**: `StoreReactivated`
- **Resposta**: `ref: string`

## Eventos de domínio

### StoreRegistered

- **Campos**:
  - `ref: string ← Store.ref`
  - `name: string ← Store.name`
  - `registeredAt: instant ← at`

### StoreSuspended

- **Campos**:
  - `ref: string ← Store.ref`
  - `reason: string ← Store.suspensionReason`
  - `suspendedAt: instant ← at`

### StoreReactivated

- **Campos**:
  - `ref: string ← Store.ref`
  - `reactivatedAt: instant ← at`

## Consultas

- `FindStore` — filtros: `ref`; cardinalidade: `one`
- `FindStoresByStatus` — filtros: `status`; cardinalidade: `many`

## Relações entre agregados

- Nenhuma: `Store` é o único agregado do contexto.

## Integração

- **Publica**: `StoreRegistered`, `StoreSuspended` e `StoreReactivated` —
  viram `store_registered.proto`, `store_suspended.proto` e
  `store_reactivated.proto` em
  `apps/backend/tenancy/contract/proto/company/tenancy/event/v1/`, package
  `company.tenancy.event.v1`, no tópico do contexto, com a DLQ e a ACL que o
  generator declara no `deploy/infra.json`. A `ref` no payload é o tenant de
  destino para quem projetar o status da loja (ADR-068 do platform, item 7); a
  origem é sempre o operador.
- **Consome**: nenhum.

## Borda e persistência

- **Borda**: só gRPC, `company.tenancy.service.v1.TenancyService`
  (`RegisterStore`, `SuspendStore`, `ReactivateStore`, `FindStore`,
  `FindStoresByStatus`), atrás da cadeia de interceptors do kernel. O REST
  público é do `bff`: `POST /tenancy/store`, `GET /tenancy/store?status=`,
  `GET /tenancy/store/{ref}`, `POST /tenancy/store/{ref}/suspend` e
  `POST /tenancy/store/{ref}/reactivate`, descritos em
  `apps/backend/tenancy/contract/openapi/v1/` (ADR-044 e ADR-053 do platform).
- **Forma do `name` e do `reason` na borda**: a mesma regra do `name` do
  golden `bookings` (SPEC-AHPRBZCT do platform), de 1 a 200 runas, aplicada
  pelo gRPC (`INVALID_ARGUMENT`) e pelo `bff` (400 `invalid-request`) antes do
  caso de uso. Por isso `name` e `reason` vazios chegando pela borda recebem
  `INVALID_ARGUMENT`/400; os códigos `tenancy/store/name-empty` e
  `tenancy/store/reason-empty` são o desfecho da UPR e do `application`, que os
  testes desses blocos provam. A forma da `ref` no `Register` fica com o
  domínio (`tenancy/store/ref-invalid`, 422 no `bff`); nas rotas com `{ref}` no
  path, a borda a recusa antes, com 400.
- **Banco**: database e role `tenancy`, schema `public`, tabela `stores`, com
  `tenant_id` à frente da chave `(tenant_id, store_id)`, em que `store_id` é a
  `ref`. Persistência híbrida: `status` é coluna tipada porque
  `FindStoresByStatus` filtra por ela, com o índice
  `stores_tenant_id_status_idx`; o resto do estado vai em `snapshot` `jsonb`.
  `name` é coluna gerada do snapshot, com
  `CONSTRAINT stores_tenant_id_name_key UNIQUE (tenant_id, name)` declarada no
  `Table` (ADR-066 do platform). O índice da identidade usado pela sonda de
  acesso entre tenants segue o golden.
- **Instante**: inteiro de nanossegundos no domínio.

## Políticas transversais

- **Multi-tenant**: todo agregado de todo contexto carrega `tenant_id`, imposto
  pelo choke point do kernel (ADR-051 do platform); neste contexto, o
  `tenant_id` de toda linha é o do operador.
- **Idempotência**: os três comandos entram pelo `bff` por `POST` com chave de
  idempotência declarada e passam por `usecase.RunIdempotent` (ADR-056 do
  platform); o replay da mesma chave devolve o mesmo desfecho sem nova
  auditoria. `Register` também é idempotente por identidade.
- **Autorização**: pelo gancho de autorização do bloco `application`, antes da
  transação (FND-04 §3.2), com `usecase.Permitted` na convenção
  `<name>:write|read` do golden `bookings`: comando exige `tenancy:write` e
  consulta exige `tenancy:read`, declaradas também nas rotas do `bff`; sem
  sujeito, vale o workload verificado pelo canal (IDN-18). O sujeito e as
  permissões não atravessam o gRPC, por desenho do DMPF (`app/rpc/metadata.go`
  do `bff` de referência; ADR-068 do platform, item 6): o contexto recebe a
  chamada do `bff` como workload, e quem confere `tenancy:write|read` é a rota do
  `bff`, cujo teste é a evidência dos cenários de permissão. Além disso, o tenant
  do contexto precisa ser o operador (`TENANCY_OPERATOR_TENANT`, obrigatória
  na `Config` da role `api`, e um operador vazio nega toda operação); outro
  tenant é recusado com `ports.ErrDenied`
  (`PermissionDenied`, 403 no `bff`), sem tocar o banco.
- **Auditoria**: pela trilha de auditoria do kernel (FND-08;
  `observability/audit`).

## Ambiente de desenvolvimento

- O `AUTH_DEV_MOCK` vale só dentro das suítes de teste (unitárias e e2e).
  Todo ambiente que se sobe para uso — o compose local e o overlay `dev` do
  k8s — autentica pelo Keycloak de referência, com o `bff` em OIDC contra o
  realm `dmpf`.
- Um seed complementar do projeto, idempotente e fora de `infra/` (que é saída
  do dmpf-plugin), roda depois do seed gerado e deixa prontos a Organization
  do operador `marketplace` (atributo `tenant_id=marketplace`), as roles
  `tenancy:read` e `tenancy:write`, um usuário operador membro dessa
  organização com as duas roles e o que o Swagger precisa para obter o token.
- O `bff` serve `POST /auth/token` só em desenvolvimento (com
  `AUTH_TOKEN_CLIENT_ID`): troca usuário e senha do operador por um JWT pelo
  password grant do Keycloak, para chamadas manuais; credencial recusada
  responde `401 invalid-credentials`, e Keycloak fora, `503`/`504`.
- No Swagger UI, o login do operador é o Authorize do esquema OAuth2 `keycloak`
  (authorization code) que o contrato do `tenancy` oferece ao lado do
  `bearerAuth`, pelo client `marketplace-dev`.

## Critérios de aceite

- [x] Os gates do workspace verdes para `tenancy` (`fmt-check`, `vet`,
  `build`, `lint`, `test-race`, `test-distributed` e `e2e` com Postgres,
  `dmpf-context-check.sh`, `modsync --check`, `infrasync --check` e
  verificador).
- [ ] O job "DMPF / DMPF" do `dmpf-ci.yml` verde no PR e o
  `dmpf-gate-check.sh` do plugin verde na execução local. O `gate-check` só
  roda no job `self-tests`, desligado por padrão no `dmpf-ci.yml`; a ADR-012 e
  o `AGENTS.md` passam a dizer isso em vez de "vermelho até o primeiro
  contexto".
- [x] As cinco rotas no `bff`, com teste de rota por desfecho, e o `tenancy`
  na readiness e no e2e do `bff`.
- [x] Ao subir o compose (`--profile dmpf --profile keycloak`), o operador
  obtém um JWT pelo comando do projeto e registra uma loja pelo `bff` sem
  `AUTH_DEV_MOCK`; o overlay `dev` do k8s renderiza sem `AUTH_DEV_MOCK`.
- [x] Cenários:

```text
DADO uma ref ainda desconhecida no operador
QUANDO Register chega com ref "loja-acme" e name "Loja Acme"
ENTÃO a loja nasce active, com registeredAt = statusChangedAt = at, e StoreRegistered vai à outbox com ref, name e registeredAt

DADO uma ref já registrada
QUANDO Register chega com a mesma ref e outro name
ENTÃO aceita sem alterar nome nem status e sem emitir evento

DADO qualquer entrada
QUANDO Register chega com ref "Loja_1", "ab", "-loja" ou de 64 caracteres
ENTÃO rejeita com tenancy/store/ref-invalid e nada é escrito

DADO qualquer entrada
QUANDO Register chega com name vazio
ENTÃO rejeita com tenancy/store/name-empty

DADO uma loja registrada com um name no operador
QUANDO Register chega com outra ref e o mesmo name, concorrente ou não
ENTÃO rejeita com tenancy/store/duplicate-name pelo desfecho, sem evento, e o replay da mesma chave devolve a mesma rejeição sem nova auditoria

DADO uma loja active
QUANDO Suspend chega com reason "fraude em análise"
ENTÃO a loja vai a suspended, com suspensionReason e statusChangedAt = at, e StoreSuspended é emitido com ref, reason e suspendedAt

DADO uma loja active
QUANDO Suspend chega com reason vazio
ENTÃO rejeita com tenancy/store/reason-empty e o estado não muda

DADO uma loja suspended
QUANDO Suspend chega
ENTÃO rejeita com tenancy/store/not-active e o estado não muda

DADO uma loja suspended
QUANDO Reactivate chega
ENTÃO a loja volta a active, com suspensionReason vazio e statusChangedAt = at, e StoreReactivated é emitido

DADO uma loja active
QUANDO Reactivate chega
ENTÃO rejeita com tenancy/store/not-suspended e o estado não muda

DADO uma ref não registrada
QUANDO Suspend ou Reactivate chega
ENTÃO recusa como não encontrado (NOT_FOUND no gRPC, 404 no bff)

DADO um principal com tenancy:write num tenant que não é o operador
QUANDO qualquer comando chega
ENTÃO recusa com PermissionDenied (403 no bff) e nada é escrito

DADO um principal no operador sem tenancy:write, ou sem tenant no contexto
QUANDO qualquer comando chega
ENTÃO recusa com PermissionDenied (403 no bff) e nada é escrito

DADO duas lojas active e uma suspended
QUANDO FindStoresByStatus consulta por active
ENTÃO devolve exatamente as duas, fora de qualquer UoW
```

## Escopo fora

- **Bloqueio de requisições de loja suspensa**: este contexto publica o status;
  quem reage é cada consumidor, na própria spec (por exemplo, `catalog` deixa
  de expor produtos). O `bff` não consulta o status.
- **Provisionamento no Keycloak**: no MVP, a Organization de cada loja (com o
  atributo `tenant_id`), a do operador e as permissões `tenancy:write|read`
  são criadas fora do sistema; são pré-condição de deploy fora de
  desenvolvimento, conferida por smoke autenticado. Automatizar pelo ciclo de
  comando externo de saída (ADR-070 do platform) fica para o onboarding
  self-service (V2). Os ambientes de desenvolvimento ficam fora desta
  exclusão: ver *Ambiente de desenvolvimento*.
- **Renomear loja e encerrar loja**: sem comando no MVP.
- **Tema, domínios customizados e projeção domínio→tenant** (V2).
- **Feature flags e credenciais por tenant via Vault** (V2).
- **Regras de comissão base por tenant e por categoria** (V3).
- **Atualização do roadmap e das specs afetadas pela decisão de tenant = loja**
  (`iam`, `cart`, `ordering`, `inventory`, `payment`): tarefa à parte.
