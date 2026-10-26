---
id: SPEC-2FJ0KYKV
slug: iam
title: Bounded context — iam
stage: building
priority: P0
depends_on: [SPEC-XB4NCW4J]
ticket_url: https://linear.app/mmda/issue/DEVS-145/spec-2fj0kykv-bounded-context-iam
subtask_urls: [https://linear.app/mmda/issue/DEVS-159, https://linear.app/mmda/issue/DEVS-160, https://linear.app/mmda/issue/DEVS-161, https://linear.app/mmda/issue/DEVS-162, https://linear.app/mmda/issue/DEVS-163, https://linear.app/mmda/issue/DEVS-164]
created: 2026-10-21
---

# SPEC-2FJ0KYKV: Bounded context — iam

Autoridade de acesso do marketplace: os papéis de cada loja, quem é membro de
cada loja e com quais papéis, e o registro dos compradores. O `iam` decide o
acesso e o projeta no Keycloak, de onde ele chega ao token que o `bff`
verifica. Fase alvo: **MVP**. Primeiro contexto do workspace com comando
externo de saída (ADR-070 do platform) e com consumo dos próprios eventos.

## Contexto

- **Fase**: MVP — ver `docs/roadmap.md`.
- **Depende de**: SPEC-XB4NCW4J (`tenancy`).
- **Fontes**:
  - `docs/product/01_identity_organization.md`
  - `docs/product/Bounded Context/Bounded Context - Identidade e Acesso (IAM).md`
  - `docs/product/Cross-Cutting Concerns/Cross-Cutting Concerns (Parte 2_ Fluxo de Autenticação e Autorização).md`
  - `docs/adr/012-borda-e-infra-alinhadas-ao-dmpf.md`
  - `docs/adr/013-tenant-e-a-loja-do-seller.md`
  - ADRs 051, 053, 056, 064, 066, 068 e 070 do platform no commit
    `ddd3d0c4202879f168c7f6bfafaefadd94a3a621`

### Fatos que moldam o modelo

Verificados no kernel rc.7 e na documentação do Keycloak 26:

- O `authn` do kernel tira as permissões só de claims do token
  (`OIDC_PERMISSION_CLAIMS`, padrão `scope` e `realm_access.roles`), e cada rota
  do `bff` exige uma permissão contra essa lista. O sujeito e as permissões não
  atravessam o gRPC até os contextos; o tenant atravessa.
- O tenant vem de `organization.*.tenant_id`, e o curinga só resolve com
  exatamente uma Organization no token. Quem é membro de duas lojas escolhe uma
  no login (scope `organization:<alias>`).
- Realm roles valem para o realm inteiro, e o Keycloak upstream não tem papel
  por Organization (keycloak#30180). Desde a 26.6 há Organization Groups, mas o
  mapper de grupos da Organization emite o caminho do grupo sempre com a barra
  inicial (`/catalog:write`), sem opção de nome puro: o nome do grupo não serve
  de permissão. Desde a 26.7, com `addGroupRoleMappings`, o mesmo mapper emite as
  roles mapeadas nos grupos do usuário naquela Organization, em
  `organization.<alias>.realm_access.roles` e
  `organization.<alias>.resource_access.<client>.roles`. O workspace usa a 26.8.1.
- As role mappings de grupo de Organization entram nas roles efetivas do usuário
  em qualquer token: o `realm_access` e o `resource_access` de topo as carregam
  sempre que o client pede o scope `roles`.
- O `authn` confere a audiência (`dmpf-bff`), não o client que pediu o token
  (`azp`): todo client com o mapper de audiência tem o token aceito, então o que
  chega como permissão depende dos mappers e scopes de cada client.
- O auto-cadastro no Keycloak não torna o usuário membro de uma Organization;
  isso só acontece por convite ou por IdP associado à Organization.
- O kernel rc.7 traz o ciclo do comando externo de saída: `application.Dispatch`,
  `Resolve`, `Abandon` e `SweepStale` sobre `ports.AttemptLedger`, com o laço
  `app.RunSweep`. O envio nasce sempre de uma mensagem consumida. Nenhum golden
  do platform (`bookings`, `orders`, `reservations`) o usa.

### Decisões do detalhamento

- **IdP é o Keycloak** (ADR-012). As menções a Logto, `org_id`, webhooks do IdP,
  RabbitMQ e Traefik de `docs/product/` não valem.
- **O `iam` é a autoridade do RBAC por loja e projeta no Keycloak.** Papéis,
  vínculos e convites são decididos aqui; o efeito chega ao Keycloak pelo
  comando externo de saída (ADR-070 do platform), e dali ao token. A borda segue
  só autenticando e exigindo a permissão da rota, como a ADR-012 fixa. Esta
  decisão é transversal e será registrada na ADR-014 na finalização.
- **Um Organization Group por papel, com client roles.** Cada papel de loja é um
  grupo da Organization da loja, com o nome da `ref`, que tem mapeadas as client
  roles de `marketplace-store` iguais às permissões do papel; o dono é o grupo
  reservado `owner`, com o catálogo inteiro. O membro pertence aos grupos dos
  seus papéis, e mudar as permissões de um papel muda só o grupo, sem reescrever
  os membros. O mapper de grupos com `addGroupRoleMappings` mora só no client
  `marketplace-seller`, e o `bff` lê
  `organization.*.resource_access.marketplace-store.roles` em
  `OIDC_PERMISSION_CLAIMS`: a permissão de loja só vale no tenant da loja. São
  client roles, e não realm roles, porque o vazamento para os claims de topo fica
  restrito ao `resource_access.marketplace-store`, que o `bff` não lê; um `iam:write`
  de loja como realm role chegaria ao `realm_access` de topo e valeria no tenant
  do operador. O token carrega permissões, não papéis, e as rotas continuam
  exigindo `<contexto>:write`.
- **Catálogo fechado de permissões**, na convenção `<contexto>:read|write` do
  golden `bookings` e do `tenancy`. Uma permissão fora dele é recusada.
- **Dono reservado.** `owner` é papel implícito, não gravado em `Role`, com o
  catálogo inteiro. O primeiro dono de cada loja é semeado no Keycloak (membro
  da Organization e do grupo `owner`, que nasce com ela) e, no primeiro acesso,
  se reconhece por `ClaimOwnership`, que estabelece a `Membership` dele com `owner`. Daí em
  diante, todo acesso da loja passa pelo `iam`.
- **Convite e vínculo são agregados distintos.** Convida-se por e-mail, e o
  `authn` só entrega sujeito, tenant e permissões. O `Invitation` vive pelo
  e-mail até o Keycloak devolver o `sub`; aí nasce a `Membership`, pelo `sub`.
- **Jornadas por client.** O comprador entra pelo client `marketplace-buyer`,
  com um claim fixo `organization.marketplace.tenant_id = marketplace`
  (mapper nativo, sem extensão), porque o auto-cadastro não o faz membro da
  Organization do operador. A loja entra pelo client `marketplace-seller`, com a
  Organization escolhida (`organization:<alias>`) e as roles dos grupos dela, sem
  o scope `roles`. Isso cumpre "um token de Buyer não serve nas rotas
  de Seller" do PRD: as rotas de comprador exigem o tenant do operador, e as de
  loja recusam o operador.
- **Comprador no tenant do operador** (ADR-013). A permissão de comprador é a
  realm role `iam:buyer`, dada pelo `iam` no registro e tirada no bloqueio. O
  bloqueio não desativa o usuário, para que a mesma pessoa siga operando uma
  loja ("jornadas que não colidem", PRD 01).
- **Service account do `iam` limitada pelo FGAP v2.** Ela tem só papéis de
  leitura de `realm-management`; o que altera no Keycloak vem das permissões do
  FGAP v2 do realm, que negam a Organization do operador e as contas do grupo
  `operators`: o operador e as service accounts. Os resíduos aceitos estão em
  "Resultado dos spikes".
- **Fronteira com `seller` e `tenancy`**: organização do seller, loja e dados
  bancários são de `seller`; o registro e o status da loja são de `tenancy`.

### Resultado dos spikes

Evidência: `apps/backend/bff/app/keycloak_iam_test.go` (tags `integration` e
`keycloak`, `pnpm nx test-keycloak bff`), no Keycloak 26.8.1 com o seed do
marketplace aplicado, verificando cada token pelo `authn.NewVerifier` do kernel
rc.7 com a configuração do `bff`.

- **S1 — negativo como especificado, resolvido por grupo por papel.** O mapper
  emite `/catalog:write`, que não satisfaz a rota. Com `addGroupRoleMappings` no
  `marketplace-seller`, o dono da `loja-demo` sai com o tenant `loja-demo` e as 16
  permissões do grupo `owner`, só nomes de role. Um membro de duas lojas que
  escolhe a outra sai sem nenhuma permissão da primeira, e os tokens do
  `marketplace-buyer` e do `dmpf-portal` não carregam nenhuma. No scope
  compartilhado `organization`, o mapper levava as roles da loja ao token do
  `dmpf-portal`; por isso mora no client do lojista.
- **S2 — positivo.** O `buyer-demo`, sem Organization, pelo `marketplace-buyer`,
  sai com `organization.marketplace.tenant_id = marketplace`; o `authn` resolve o
  tenant do operador e a permissão `iam:buyer`, sem `iam:write`. O client não tem
  o scope `organization`, para que nenhum mapper de Organization dispute o claim.
- **S3 — positivo, com resíduos aceitos.** A busca
  `GET /organizations?q=tenant_id:<tenant>` responde com `query-organizations` e
  `view-organizations`. Com `manage-users` e `manage-organizations`, o conjunto
  previsto, a service account concedeu `iam:write` e `tenancy:write` e entrou na
  Organization do operador: os dois papéis são conferidos antes do FGAP v2 e o
  anulam (`RolePermissionsV2.canMapRole` e `OrganizationPermissions.canManage`
  na tag 26.8.1). O conjunto adotado é `query-organizations`,
  `view-organizations`, `view-users`, `view-realm` e `view-clients`, com o FGAP v2
  ligado (`adminPermissionsEnabled`) e estas permissões no client
  `admin-permissions`, todas `UNANIMOUS`; as que negam usam uma user policy sem
  usuários:

  | Permissão | Recurso | Scopes | Efeito |
  | --- | --- | --- | --- |
  | `iam-map-buyer` | realm role `iam:buyer` | `map-role` | concede |
  | `iam-map-store-roles` | client `marketplace-store` | `map-roles` | concede |
  | `iam-users` | todos os usuários | `view`, `manage`, `map-roles`, `manage-group-membership` | concede |
  | `iam-organizations` | todas as Organizations | `view`, `manage` | concede |
  | `deny-operator-organization` | Organization `marketplace` | `manage` | nega |
  | `deny-operators-membership` | grupo de realm `operators` | `manage-members`, `manage-membership-of-members` | nega |
  | `deny-operator-map-roles` | cada conta do grupo `operators` | `map-roles` | nega |

  O grupo `operators` reúne as contas que a service account não pode gerir: o
  `operator` e as service accounts do `iam-admin`, ela mesma, e do
  `dmpf-session-events`. O seed cria os denies antes das concessões e os
  reescreve a cada execução, porque o digest na description só prova o que ele
  próprio escreveu.

  Assim, o teste mostra a service account fazendo tudo o que o adaptador usa:
  buscar a Organization pelo `tenant_id`, criar, buscar pelo nome e listar os
  grupos dela, pôr, ler e tirar client roles de `marketplace-store` num grupo,
  criar usuário e buscá-lo por e-mail, pôr e tirar o usuário da Organization e
  do grupo, e conceder e tirar `iam:buyer`. Recebe 403, por falta de concessão,
  ao conceder `iam:write` ou `tenancy:write`, criar client, se dar papéis de
  `realm-management` e editar uma permissão do FGAP; e, pelos denies, ao entrar
  na Organization do operador ou mudar o `tenant_id` dela, ao trocar a senha,
  desativar, excluir ou conceder role ao operador, ao tirá-lo da Organization
  dele ou do grupo `operators`, ao pô-lo na Organization ou no grupo `owner` de
  uma loja e ao desativar ou conceder role à service account do
  `dmpf-session-events`. Fora da suíte, por chamadas manuais num realm
  descartável no mesmo Keycloak, ficaram provados o contorno do FGAP por
  `manage-users` e `manage-organizations` e o 403 ao desligar o FGAP, criar realm
  role e fazer `partialImport`. **Resíduos aceitos
  no MVP**, porque o FGAP v2 não tem condição por tenant nem por alvo:
  - administra qualquer usuário fora do grupo `operators`: cria, troca a senha,
    desativa e exclui compradores e membros de qualquer loja;
  - concede `iam:buyer` a qualquer usuário exceto o `operator`, e mapeia as roles
    de `marketplace-store` em grupos de qualquer loja e direto em usuário (a
    role direta não chega a `organization.<alias>`, que o `bff` lê);
  - cria e exclui Organizations e altera atributos das lojas, inclusive o
    `tenant_id`: uma loja com `tenant_id=marketplace` faria os membros dela
    entrarem no tenant do operador com as permissões de loja, `iam:write`
    incluída. O adaptador do `iam` nunca altera Organization;
  - lê usuários e Organizations do realm inteiro e a configuração do próprio
    FGAP (`view-realm`);
  - devolver `manage-users` ou `manage-organizations` a ela desfaz as recusas em
    silêncio; o seed tira todo papel de `realm-management` fora dos cinco de
    leitura, recusa na pós-condição os proibidos que cheguem por composite, e o
    `converged.sh` roda o seed quando encontra um deles.
  Quem opera pelo operador, e toda service account nova, entra no grupo
  `operators` e no deny de `map-roles` por usuário. O broker administrativo, que fecharia os resíduos,
  fica fora do MVP.

## Identidade

- **bounded_context**: `iam`
- **name**: `iam`
- **Linguagem ubíqua**:
  - *Permissão* — o direito de ler ou escrever num contexto, na forma
    `<contexto>:read|write`; é o que a rota do `bff` exige.
  - *Catálogo* — o conjunto fechado de permissões que um papel de loja pode
    conter.
  - *Papel* (`Role`) — um nome de loja para um conjunto de permissões do
    catálogo.
  - *Grupo do papel* — o Organization Group de nome `<ref>` na Organization da
    loja, com as client roles de `marketplace-store` das permissões do papel; o
    do dono é `owner`.
  - *Dono* (`owner`) — o papel reservado com o catálogo inteiro; não é
    definido, concedido nem revogado por comando.
  - *Membro* (`Membership`) — o vínculo de um sujeito com uma loja, com seus
    papéis.
  - *Convite* (`Invitation`) — o pedido de vínculo de um e-mail a uma loja, até
    o Keycloak devolver o sujeito.
  - *Sujeito* (`subject`) — o `sub` do usuário no Keycloak.
  - *Permissões efetivas* — a união das permissões dos papéis ativos de um
    membro, ou o catálogo inteiro para o dono.
  - *Projeção no Keycloak* — o efeito que o `iam` grava no Keycloak para que o
    token reflita o acesso decidido aqui.
  - *Versão de acesso* (`accessVersion`) — o contador de mudanças de acesso de
    um papel, membro ou comprador; a projeção é confirmada por versão. As
    permissões efetivas de um membro no token dependem da versão sincronizada
    dele (os grupos) e da de cada papel seu (as roles de cada grupo).
  - *Comprador* (`Buyer`) — o consumidor registrado no tenant do operador.
  - *Bloqueio* — o ato de tirar do comprador a permissão de comprar, com um
    motivo.

### Catálogo de permissões

`iam:read`, `iam:write`, `seller:read`, `seller:write`, `catalog:read`,
`catalog:write`, `inventory:read`, `inventory:write`, `pricing:read`,
`pricing:write`, `ordering:read`, `ordering:write`, `fulfillment:read`,
`fulfillment:write`, `settlement:read`, `settlement:write`.

Cada permissão do catálogo é uma client role do client confidencial
`marketplace-store`, sem fluxo de login, que existe só para guardá-las.

Fora do catálogo, com outra origem: `tenancy:read|write` e `iam:read|write` do
operador são realm roles do pessoal do operador (seed); `iam:buyer` é a realm
role que o `iam` concede ao comprador.

## Agregados

### Role

Vive no tenant da loja.

- **Identidade**: `natural` por `ref`
- **Campos**:
  - `ref: string`
  - `name: string`
  - `permissions: string[]` — ordenadas e sem repetição
  - `status: active | retired`
  - `accessVersion: int` — começa em 1 e sobe a cada mudança de permissões ou
    de status
  - `syncedVersion: int` — a maior versão de acesso confirmada no Keycloak
  - `definedAt: instant`
  - `changedAt: instant`
- **Estados**: `active`, `retired`
- **Transições permitidas**:
  - (novo) → `active` — por `DefineRole`
  - `active` → `active` — por `ChangeRolePermissions` e `ConfirmRoleSynced`
  - `active` → `retired` — por `RetireRole`
  - `retired` → `retired` — por `ConfirmRoleSynced`
- **Invariantes de estado**:
  - `ref` tem de 3 a 63 caracteres em `a-z`, `0-9` e `-`, sem hífen no início
    nem no fim — `iam/role/ref-invalid`.
  - `ref` não é `owner` — `iam/role/reserved`.
  - `name` nunca é vazio — `iam/role/name-empty`.
  - `permissions` nunca é vazio — `iam/role/permissions-empty`.
  - Toda permissão está no catálogo — `iam/role/permission-unknown`.
  - `syncedVersion ≤ accessVersion`.
- **Unicidade de negócio**: `name` único por tenant, por igualdade exata —
  `iam/role/duplicate-name` (ADR-066 do platform).

### Invitation

Vive no tenant da loja.

- **Identidade**: `natural` por `email`, normalizado (sem espaços nas pontas,
  em minúsculas)
- **Campos**:
  - `email: string`
  - `roles: string[]` — refs de papel, ordenadas e sem repetição
  - `status: pending | provisioned | failed`
  - `round: int` — a rodada do convite, que começa em 1 e sobe a cada reabertura
  - `subject: string` — vazio até `provisioned`
  - `failureReason: string` — vazio fora de `failed`
  - `invitedAt: instant`
  - `settledAt: instant` — zero enquanto `pending`
- **Estados**: `pending`, `provisioned`, `failed`
- **Transições permitidas**:
  - (novo) → `pending` — por `InviteMember`
  - `failed` → `pending` — por `InviteMember` (reabertura)
  - `provisioned` → `pending` — por `InviteMember` (reconvite, de quem saiu ou
    para somar papéis)
  - `pending` → `provisioned` — por `ConfirmInvitation`
  - `pending` → `failed` — por `FailInvitation`
- **Invariantes de estado**:
  - `email` tem até 254 caracteres, exatamente um `@`, parte local não vazia e
    domínio com ao menos um `.` — `iam/invitation/email-invalid`.
  - `roles` nunca é vazio — `iam/invitation/roles-empty`; toda ref tem a forma
    de `Role.ref` — `iam/invitation/role-invalid`; `owner` não é convidável —
    `iam/invitation/role-reserved`.
  - `subject` não é vazio se e só se `status = provisioned`.
  - `failureReason` não é vazio se e só se `status = failed`.
- **Unicidade de negócio**: nenhuma.

### Membership

Vive no tenant da loja.

- **Identidade**: `natural` por `subject`
- **Campos**:
  - `subject: string`
  - `email: string` — vazio quando o vínculo nasce por `ClaimOwnership`
  - `roles: string[]` — refs de papel, ordenadas e sem repetição; pode conter
    `owner`
  - `status: active | removed`
  - `accessVersion: int` — começa em 1 e sobe a cada mudança de acesso
  - `syncedVersion: int` — a maior versão de acesso confirmada no Keycloak
  - `joinedAt: instant`
  - `changedAt: instant`
- **Estados**: `active`, `removed`
- **Transições permitidas**:
  - (novo) → `active` — por `EstablishMembership`
  - `removed` → `active` — por `EstablishMembership` (retorno)
  - `active` → `active` — por `EstablishMembership`, `GrantRole`, `RevokeRole`
    e `ConfirmMemberAccessSynced`
  - `active` → `removed` — por `RemoveMember`
  - `removed` → `removed` — por `ConfirmMemberAccessSynced`
- **Invariantes de estado**:
  - `subject` tem de 1 a 255 caracteres — `iam/membership/subject-invalid`.
  - `roles` nunca é vazio enquanto `active` — `iam/membership/last-role`.
  - Toda ref de `roles` tem a forma de `Role.ref` ou é `owner` —
    `iam/membership/role-invalid`.
  - `syncedVersion ≤ accessVersion`.
- **Unicidade de negócio**: nenhuma.

### Buyer

Vive no tenant do operador.

- **Identidade**: `natural` por `subject`
- **Campos**:
  - `subject: string`
  - `name: string`
  - `status: active | blocked`
  - `blockReason: string`
  - `accessVersion: int` — começa em 1 e sobe a cada mudança de status
  - `syncedVersion: int`
  - `registeredAt: instant`
  - `changedAt: instant`
- **Estados**: `active`, `blocked`
- **Transições permitidas**:
  - (novo) → `active` — por `RegisterBuyer`
  - `active` → `active` — por `UpdateBuyerProfile` e `ConfirmBuyerAccessSynced`
  - `active` → `blocked` — por `BlockBuyer`
  - `blocked` → `active` — por `UnblockBuyer`
  - `blocked` → `blocked` — por `ConfirmBuyerAccessSynced`
- **Invariantes de estado**:
  - `subject` tem de 1 a 255 caracteres — `iam/buyer/subject-invalid`.
  - `name` nunca é vazio — `iam/buyer/name-empty`.
  - `blockReason` não é vazio se e só se `status = blocked` —
    `iam/buyer/reason-empty`.
  - `syncedVersion ≤ accessVersion`.
- **Unicidade de negócio**: nenhuma.

## Comandos (UPRs)

Os comandos marcados como **passo externo** são aplicados pelo ciclo do
ADR-070, na UoW que grava o desfecho do Keycloak, e os marcados como
**interno** chegam por consumo; nenhum dos dois tem RPC nem rota.

### DefineRole

- **Agregado**: `Role`; **inicializa se ausente**
- **Comando**: `DefineRole`
- **Entrada**: `ref: string`, `name: string`, `permissions: string[]`
- **Pré-condições**:
  - `ref` na forma do invariante → `iam/role/ref-invalid`
  - `ref ≠ owner` → `iam/role/reserved`
  - `name` não vazio → `iam/role/name-empty`
  - `permissions` não vazio → `iam/role/permissions-empty`
  - toda permissão no catálogo → `iam/role/permission-unknown`
- **Efeitos sobre o estado**:
  - quando ausente: `name ← cmd.name`, `permissions ← cmd.permissions`
    ordenadas e sem repetição, `status ← active`, `accessVersion ← 1`,
    `syncedVersion ← 0`, `definedAt ← at`, `changedAt ← at`
  - quando presente: nenhum — aceita sem alterar e sem emitir
- **Eventos emitidos**: `RoleDefined` (só quando inicializa)
- **Resposta**: `ref: string`

### ChangeRolePermissions

- **Agregado**: `Role`; exige existente
- **Comando**: `ChangeRolePermissions`
- **Entrada**: `permissions: string[]`
- **Pré-condições**:
  - `status = active` → `iam/role/retired`
  - `permissions` não vazio → `iam/role/permissions-empty`
  - toda permissão no catálogo → `iam/role/permission-unknown`
- **Efeitos sobre o estado**:
  - quando o conjunto normalizado difere do atual: `permissions ← cmd.permissions`
    ordenadas e sem repetição, `accessVersion ← accessVersion + 1`,
    `changedAt ← at`
  - quando é igual: nenhum — aceita sem emitir
- **Eventos emitidos**: `RolePermissionsChanged` (só quando muda)
- **Resposta**: `ref: string`

### RetireRole

- **Agregado**: `Role`; exige existente
- **Comando**: `RetireRole`
- **Entrada**: nenhuma além da identidade
- **Pré-condições**:
  - `status = active` → `iam/role/retired`
- **Efeitos sobre o estado**:
  - `status ← retired`
  - `accessVersion ← accessVersion + 1`
  - `changedAt ← at`
- **Eventos emitidos**: `RoleRetired`
- **Resposta**: `ref: string`

### ConfirmRoleSynced

- **Agregado**: `Role`; exige existente — **passo externo** da sincronização
  do grupo do papel
- **Comando**: `ConfirmRoleSynced`
- **Entrada**: `version: int`
- **Pré-condições**:
  - `version ≤ accessVersion` → `iam/role/version-ahead`
- **Efeitos sobre o estado**:
  - `syncedVersion ← max(syncedVersion, cmd.version)`
- **Eventos emitidos**: nenhum
- **Resposta**: nenhuma

### InviteMember

- **Agregado**: `Invitation`; **inicializa se ausente**
- **Comando**: `InviteMember`
- **Entrada**: `email: string`, `roles: string[]`
- **Pré-condições**:
  - `email` na forma do invariante → `iam/invitation/email-invalid`
  - `roles` não vazio → `iam/invitation/roles-empty`
  - toda ref na forma de `Role.ref` → `iam/invitation/role-invalid`
  - nenhuma ref é `owner` → `iam/invitation/role-reserved`
- **Efeitos sobre o estado**:
  - quando ausente: `roles ← cmd.roles` ordenadas e sem repetição,
    `status ← pending`, `round ← 1`, `subject ← ""`, `failureReason ← ""`,
    `invitedAt ← at`, `settledAt ← 0`
  - quando presente e `failed` ou `provisioned`: `roles ← cmd.roles`,
    `status ← pending`, `round ← round + 1`, `subject ← ""`,
    `failureReason ← ""`, `invitedAt ← at`, `settledAt ← 0`
  - quando presente e `pending`: nenhum — aceita sem alterar e sem emitir
- **Eventos emitidos**: `MemberInvited` (quando inicializa ou reabre)
- **Resposta**: `email: string`, `status: string`

### ConfirmInvitation

- **Agregado**: `Invitation`; exige existente — **passo externo** do convite
- **Comando**: `ConfirmInvitation`
- **Entrada**: `subject: string`
- **Pré-condições**:
  - `status = pending` → `iam/invitation/not-pending`
  - `subject` de 1 a 255 caracteres → `iam/invitation/subject-invalid`
- **Efeitos sobre o estado**:
  - `status ← provisioned`
  - `subject ← cmd.subject`
  - `settledAt ← at`
- **Eventos emitidos**: `InvitationProvisioned`
- **Resposta**: nenhuma

### FailInvitation

- **Agregado**: `Invitation`; exige existente — **passo externo** do convite
- **Comando**: `FailInvitation`
- **Entrada**: `reason: string`
- **Pré-condições**:
  - `status = pending` → `iam/invitation/not-pending`
  - `reason` não vazio → `iam/invitation/reason-empty`
- **Efeitos sobre o estado**:
  - `status ← failed`
  - `failureReason ← cmd.reason`
  - `settledAt ← at`
- **Eventos emitidos**: `InvitationFailed`
- **Resposta**: nenhuma

### EstablishMembership

- **Agregado**: `Membership`; **inicializa se ausente**
- **Comando**: `EstablishMembership` — chamado pelo caso de uso
  `ClaimOwnership` (borda, com `roles = [owner]` e `email` vazio) e pelo consumo
  de `InvitationProvisioned` (**interno**, com o e-mail e os papéis do convite)
- **Entrada**: `subject: string`, `email: string`, `roles: string[]`
- **Pré-condições**:
  - `subject` de 1 a 255 caracteres → `iam/membership/subject-invalid`
  - `roles` não vazio → `iam/membership/last-role`
  - toda ref na forma de `Role.ref` ou `owner` → `iam/membership/role-invalid`
- **Efeitos sobre o estado**:
  - quando ausente: `email ← cmd.email`, `roles ← cmd.roles` ordenadas e sem
    repetição, `status ← active`, `accessVersion ← 1`, `syncedVersion ← 0`,
    `joinedAt ← at`, `changedAt ← at`
  - quando presente e `removed`: `email ← cmd.email` se não vazio,
    `roles ← cmd.roles`, `status ← active`, `accessVersion ← accessVersion + 1`,
    `changedAt ← at`
  - quando presente e `active`: `email ← cmd.email` se o atual for vazio;
    `roles ← roles ∪ cmd.roles`; se `roles` mudou,
    `accessVersion ← accessVersion + 1` e `changedAt ← at`; senão, aceita sem
    emitir
- **Eventos emitidos**: `MembershipEstablished` (quando inicializa ou retorna),
  `MemberAccessChanged` (quando um vínculo ativo ganha papel)
- **Resposta**: `subject: string`
- **Guarda do `ClaimOwnership`** (pré-condição relacional, ADR-067): antes de
  decidir, o caso de uso lê na UoW se a loja tem `Membership` `active` com
  `owner` de outro `subject`; se tem, recusa com `iam/membership/owner-claimed`.
  Só o primeiro a reclamar vira dono, e o primeiro é o dono semeado: até ele
  reclamar, a loja não tem `Membership`, e ninguém mais tem `iam:write` nela
  para convidar. Quem já é o dono reclama de novo sem efeito.

### GrantRole

- **Agregado**: `Membership`; exige existente
- **Comando**: `GrantRole`
- **Entrada**: `role: string`
- **Pré-condições**:
  - `status = active` → `iam/membership/removed`
  - `role` na forma de `Role.ref` → `iam/membership/role-invalid`
  - `role ≠ owner` → `iam/membership/owner-reserved`
- **Efeitos sobre o estado**:
  - quando `role` não está em `roles`: `roles ← roles ∪ {role}`,
    `accessVersion ← accessVersion + 1`, `changedAt ← at`
  - quando já está: nenhum — aceita sem emitir
- **Eventos emitidos**: `MemberAccessChanged` (só quando muda)
- **Resposta**: `subject: string`

### RevokeRole

- **Agregado**: `Membership`; exige existente
- **Comando**: `RevokeRole`
- **Entrada**: `role: string`
- **Pré-condições**:
  - `status = active` → `iam/membership/removed`
  - `role ≠ owner` → `iam/membership/owner-protected`
  - `role` em `roles` → `iam/membership/role-not-held`
  - `roles` continua não vazio depois da revogação → `iam/membership/last-role`
- **Efeitos sobre o estado**:
  - `roles ← roles ∖ {role}`
  - `accessVersion ← accessVersion + 1`
  - `changedAt ← at`
- **Eventos emitidos**: `MemberAccessChanged`
- **Resposta**: `subject: string`

### RemoveMember

- **Agregado**: `Membership`; exige existente
- **Comando**: `RemoveMember`
- **Entrada**: nenhuma além da identidade
- **Pré-condições**:
  - `status = active` → `iam/membership/removed`
  - `owner` fora de `roles` → `iam/membership/owner-protected`
- **Efeitos sobre o estado**:
  - `status ← removed`
  - `accessVersion ← accessVersion + 1`
  - `changedAt ← at`
- **Eventos emitidos**: `MemberRemoved`
- **Resposta**: `subject: string`

### ConfirmMemberAccessSynced

- **Agregado**: `Membership`; exige existente — **passo externo** da
  sincronização de acesso
- **Comando**: `ConfirmMemberAccessSynced`
- **Entrada**: `version: int`
- **Pré-condições**:
  - `version ≤ accessVersion` → `iam/membership/version-ahead`
- **Efeitos sobre o estado**:
  - `syncedVersion ← max(syncedVersion, cmd.version)`
- **Eventos emitidos**: nenhum
- **Resposta**: nenhuma

### RegisterBuyer

- **Agregado**: `Buyer`; **inicializa se ausente**
- **Comando**: `RegisterBuyer`
- **Entrada**: `subject: string` — preenchido pelo `bff` com o sujeito do token,
  nunca pelo cliente; `name: string`
- **Pré-condições**:
  - `subject` de 1 a 255 caracteres → `iam/buyer/subject-invalid`
  - `name` não vazio → `iam/buyer/name-empty`
- **Efeitos sobre o estado**:
  - quando ausente: `name ← cmd.name`, `status ← active`, `blockReason ← ""`,
    `accessVersion ← 1`, `syncedVersion ← 0`, `registeredAt ← at`,
    `changedAt ← at`
  - quando presente: nenhum — aceita sem alterar, inclusive um comprador
    bloqueado, e sem emitir
- **Eventos emitidos**: `BuyerRegistered` (só quando inicializa)
- **Resposta**: `subject: string`, `status: string`

### UpdateBuyerProfile

- **Agregado**: `Buyer`; exige existente
- **Comando**: `UpdateBuyerProfile`
- **Entrada**: `name: string`
- **Pré-condições**:
  - `status = active` → `iam/buyer/blocked`
  - `name` não vazio → `iam/buyer/name-empty`
- **Efeitos sobre o estado**:
  - `name ← cmd.name`
  - `changedAt ← at`
- **Eventos emitidos**: `BuyerProfileUpdated`
- **Resposta**: `subject: string`

### BlockBuyer

- **Agregado**: `Buyer`; exige existente
- **Comando**: `BlockBuyer`
- **Entrada**: `reason: string`
- **Pré-condições**:
  - `status = active` → `iam/buyer/not-active`
  - `reason` não vazio → `iam/buyer/reason-empty`
- **Efeitos sobre o estado**:
  - `status ← blocked`
  - `blockReason ← cmd.reason`
  - `accessVersion ← accessVersion + 1`
  - `changedAt ← at`
- **Eventos emitidos**: `BuyerBlocked`
- **Resposta**: `subject: string`

### UnblockBuyer

- **Agregado**: `Buyer`; exige existente
- **Comando**: `UnblockBuyer`
- **Entrada**: nenhuma além da identidade
- **Pré-condições**:
  - `status = blocked` → `iam/buyer/not-blocked`
- **Efeitos sobre o estado**:
  - `status ← active`
  - `blockReason ← ""`
  - `accessVersion ← accessVersion + 1`
  - `changedAt ← at`
- **Eventos emitidos**: `BuyerUnblocked`
- **Resposta**: `subject: string`

### ConfirmBuyerAccessSynced

- **Agregado**: `Buyer`; exige existente — **passo externo** da sincronização
  da permissão de comprador
- **Comando**: `ConfirmBuyerAccessSynced`
- **Entrada**: `version: int`
- **Pré-condições**:
  - `version ≤ accessVersion` → `iam/buyer/version-ahead`
- **Efeitos sobre o estado**:
  - `syncedVersion ← max(syncedVersion, cmd.version)`
- **Eventos emitidos**: nenhum
- **Resposta**: nenhuma

## Eventos de domínio

### RoleDefined

- **Campos**: `ref: string ← Role.ref`, `name: string ← Role.name`,
  `permissions: string[] ← Role.permissions`,
  `accessVersion: int ← Role.accessVersion`, `definedAt: instant ← at`

### RolePermissionsChanged

- **Campos**: `ref: string ← Role.ref`,
  `permissions: string[] ← Role.permissions`,
  `accessVersion: int ← Role.accessVersion`, `changedAt: instant ← at`

### RoleRetired

- **Campos**: `ref: string ← Role.ref`,
  `accessVersion: int ← Role.accessVersion`, `retiredAt: instant ← at`

### MemberInvited

- **Campos**: `email: string ← Invitation.email`,
  `roles: string[] ← Invitation.roles`, `round: int ← Invitation.round`,
  `invitedAt: instant ← at`

### InvitationProvisioned

- **Campos**: `email: string ← Invitation.email`,
  `subject: string ← Invitation.subject`, `roles: string[] ← Invitation.roles`,
  `provisionedAt: instant ← at`

### InvitationFailed

- **Campos**: `email: string ← Invitation.email`,
  `reason: string ← Invitation.failureReason`, `failedAt: instant ← at`

### MembershipEstablished

- **Campos**: `subject: string ← Membership.subject`,
  `email: string ← Membership.email`, `roles: string[] ← Membership.roles`,
  `accessVersion: int ← Membership.accessVersion`, `establishedAt: instant ← at`

### MemberAccessChanged

- **Campos**: `subject: string ← Membership.subject`,
  `roles: string[] ← Membership.roles`,
  `accessVersion: int ← Membership.accessVersion`, `changedAt: instant ← at`

### MemberRemoved

- **Campos**: `subject: string ← Membership.subject`,
  `accessVersion: int ← Membership.accessVersion`, `removedAt: instant ← at`

### BuyerRegistered

- **Campos**: `subject: string ← Buyer.subject`, `name: string ← Buyer.name`,
  `accessVersion: int ← Buyer.accessVersion`, `registeredAt: instant ← at`

### BuyerProfileUpdated

- **Campos**: `subject: string ← Buyer.subject`, `name: string ← Buyer.name`,
  `updatedAt: instant ← at`

### BuyerBlocked

- **Campos**: `subject: string ← Buyer.subject`,
  `reason: string ← Buyer.blockReason`,
  `accessVersion: int ← Buyer.accessVersion`, `blockedAt: instant ← at`

### BuyerUnblocked

- **Campos**: `subject: string ← Buyer.subject`,
  `accessVersion: int ← Buyer.accessVersion`, `unblockedAt: instant ← at`

## Consultas

- `FindRole` — filtros: `ref`; cardinalidade: `one`
- `FindRolesByStatus` — filtros: `status`; cardinalidade: `many`
- `FindInvitation` — filtros: `email`; cardinalidade: `one`
- `FindInvitationsByStatus` — filtros: `status`; cardinalidade: `many`
- `FindMembership` — filtros: `subject`; cardinalidade: `one`
- `FindMembershipsByStatus` — filtros: `status`; cardinalidade: `many`
- `FindBuyer` — filtros: `subject`; cardinalidade: `one`
- `FindBuyersByStatus` — filtros: `status`; cardinalidade: `many`

## Relações entre agregados

- `Membership.roles[]` → `Role.ref` (exceto `owner`). A existência e o status do
  papel não são conferidos pela UPR: um papel ausente ou `retired` não
  contribui permissões efetivas, porque o grupo dele fica sem client roles. Uma
  ref ainda não definida passa a contribuir quando o `DefineRole` dela chega:
  o SyncRole dá as roles ao grupo em que o membro já está.
- `Invitation.roles[]` → `Role.ref`, com a mesma regra.
- `InvitationProvisioned.subject` → `Membership.subject`.

## Integração

- **Publica**: todos os eventos acima, em
  `apps/backend/iam/contract/proto/company/iam/event/v1/` (`role_defined.proto`,
  `role_permissions_changed.proto`, `role_retired.proto`,
  `member_invited.proto`, `invitation_provisioned.proto`,
  `invitation_failed.proto`, `membership_established.proto`,
  `member_access_changed.proto`, `member_removed.proto`,
  `buyer_registered.proto`, `buyer_profile_updated.proto`,
  `buyer_blocked.proto`, `buyer_unblocked.proto`), package
  `company.iam.event.v1`, no tópico do contexto, com a DLQ e a ACL que o
  generator declara no `deploy/infra.json`.
- **Consome**, do próprio tópico, pelas disposições de FND-04 §6.4:

  | Mensagem | O que faz |
  | --- | --- |
  | `company.iam.event.v1.InvitationProvisioned` | `EstablishMembership` com `subject`, `email` e `roles` do evento |
  | `company.iam.event.v1.RoleDefined`, `RolePermissionsChanged`, `RoleRetired` | comando externo **SyncRole** |
  | `company.iam.event.v1.MemberInvited` | comando externo **ProvisionMember** |
  | `company.iam.event.v1.MembershipEstablished`, `MemberAccessChanged` | comando externo **SyncMemberAccess** |
  | `company.iam.event.v1.MemberRemoved` | comando externo **RevokeMemberAccess** |
  | `company.iam.event.v1.BuyerRegistered`, `BuyerBlocked`, `BuyerUnblocked` | comando externo **SyncBuyerAccess** |

- **Comandos externos de saída** (ADR-070 do platform), com o Keycloak como
  parceiro pela Admin API, por uma service account própria do `iam`. A
  Organization da loja é encontrada pelo atributo `tenant_id` igual ao tenant
  da mensagem. Cada comando converge o Keycloak ao estado atual lido no
  `iam`, então a reentrega e a ordem trocada não desfazem nada:

  | Comando externo | Chave de saída | Efeito no Keycloak | Passo aplicado |
  | --- | --- | --- | --- |
  | SyncRole | tenant, `ref`, `accessVersion` | garante o grupo `<ref>` na Organization e deixa nele exatamente as client roles de `marketplace-store` das permissões do papel `active`, ou nenhuma quando `retired` | `ConfirmRoleSynced` com a versão |
  | ProvisionMember | tenant, `email`, `round` | garante o usuário pelo e-mail (cria com as ações obrigatórias de senha e verificação quando não existe) e o torna membro da Organization | aceito: `ConfirmInvitation` com o `sub`; recusa final: `FailInvitation` com o motivo |
  | SyncMemberAccess | tenant, `subject`, `accessVersion` | converge pelo `status` atual da `Membership`: `active` garante o membro na Organization e exatamente nos grupos dos seus papéis, `owner` incluído, criando sem roles o grupo de papel que faltar (as roles dele são do SyncRole); `removed` faz o mesmo que o RevokeMemberAccess | `ConfirmMemberAccessSynced` com a versão |
  | RevokeMemberAccess | tenant, `subject`, `accessVersion` | converge pelo `status` atual: `removed` tira o membro dos grupos e da Organization; `active` (o membro voltou) faz o mesmo que o SyncMemberAccess | `ConfirmMemberAccessSynced` com a versão |
  | SyncBuyerAccess | tenant, `subject`, `accessVersion` | concede `iam:buyer` quando o comprador está `active` e a tira quando `blocked` | `ConfirmBuyerAccessSynced` com a versão |

  A tentativa vencida é encerrada pela varredura (`app.RunSweep` sobre
  `app.SweepExternalAttempts`) como `UnknownSent`, sem reenvio; a reentrega da
  mensagem reconverge. Criar um grupo que já existe (409, a corrida entre
  SyncRole e SyncMemberAccess) relê o grupo pelo nome e segue. A Organization
  ausente para o tenant, o grupo `owner` ausente e a recusa da Admin API (403)
  são recusa final: a tentativa termina por `Abandon`, sem passo aplicado, e o
  `syncedVersion` atrás do `accessVersion` sinaliza a divergência. O grupo
  `owner`, com o catálogo inteiro, nasce com a Organization semeada da loja e
  segue o catálogo pela mesma semeadura; nenhum comando externo o cria nem muda
  as roles dele. A service account age sob o FGAP v2 de "Resultado dos spikes".

## Borda e persistência

- **Borda**: só gRPC, `company.iam.service.v1.IamService`, um RPC por comando de
  borda e por consulta pública (`DefineRole`, `ChangeRolePermissions`,
  `RetireRole`, `InviteMember`, `ClaimOwnership`, `GrantRole`, `RevokeRole`,
  `RemoveMember`, `RegisterBuyer`, `UpdateBuyerProfile`, `BlockBuyer`,
  `UnblockBuyer`, `FindRole`, `FindRolesByStatus`, `FindInvitation`,
  `FindInvitationsByStatus`, `FindMembership`, `FindMembershipsByStatus`,
  `FindBuyer`, `FindBuyersByStatus`), atrás da cadeia de interceptors do
  kernel. O REST público é do `bff`, descrito em
  `apps/backend/iam/contract/openapi/v1/`:

  | Rota | Operação | Permissão | Tenant |
  | --- | --- | --- | --- |
  | `POST /iam/ownership/claim` | `ClaimOwnership` | `iam:write` | loja |
  | `POST /iam/role` | `DefineRole` | `iam:write` | loja |
  | `GET /iam/role?status=` | `FindRolesByStatus` | `iam:read` | loja |
  | `GET /iam/role/{ref}` | `FindRole` | `iam:read` | loja |
  | `POST /iam/role/{ref}/permissions` | `ChangeRolePermissions` | `iam:write` | loja |
  | `POST /iam/role/{ref}/retire` | `RetireRole` | `iam:write` | loja |
  | `POST /iam/invitation` | `InviteMember` | `iam:write` | loja |
  | `GET /iam/invitation?status=` | `FindInvitationsByStatus` | `iam:read` | loja |
  | `GET /iam/invitation/{email}` | `FindInvitation` | `iam:read` | loja |
  | `GET /iam/member?status=` | `FindMembershipsByStatus` | `iam:read` | loja |
  | `GET /iam/member/{subject}` | `FindMembership` | `iam:read` | loja |
  | `POST /iam/member/{subject}/grant-role` | `GrantRole` | `iam:write` | loja |
  | `POST /iam/member/{subject}/revoke-role` | `RevokeRole` | `iam:write` | loja |
  | `POST /iam/member/{subject}/remove` | `RemoveMember` | `iam:write` | loja |
  | `POST /iam/buyer` | `RegisterBuyer` | `profile` | operador |
  | `GET /iam/buyer/me` | `FindBuyer` do próprio sujeito | `iam:buyer` | operador |
  | `POST /iam/buyer/me/profile` | `UpdateBuyerProfile` do próprio sujeito | `iam:buyer` | operador |
  | `GET /iam/buyer?status=` | `FindBuyersByStatus` | `iam:read` | operador |
  | `GET /iam/buyer/{subject}` | `FindBuyer` | `iam:read` | operador |
  | `POST /iam/buyer/{subject}/block` | `BlockBuyer` | `iam:write` | operador |
  | `POST /iam/buyer/{subject}/unblock` | `UnblockBuyer` | `iam:write` | operador |

- **Sujeito pela borda**: em `ClaimOwnership`, `RegisterBuyer` e nas rotas
  `/iam/buyer/me`, o `bff` preenche `subject` com o sujeito do token; o corpo
  não traz sujeito.
- **Forma na borda**: `name` e `reason` seguem a regra do `name` do golden
  `bookings` (de 1 a 200 runas), aplicada pelo gRPC (`INVALID_ARGUMENT`) e pelo
  `bff` (400 `invalid-request`) antes do caso de uso; `ref` e `email` no path
  são recusados pela borda com 400 quando fora da forma. No corpo, a forma de
  `ref`, `email`, `roles` e `permissions` fica com o domínio (422 no `bff`).
- **Banco**: database e role `iam`, schema `public`, persistência híbrida, com
  `tenant_id` à frente de cada chave:
  - `roles` — chave `(tenant_id, role_id)`, `role_id` é a `ref`; `status`
    tipada (`FindRolesByStatus`), índice `roles_tenant_id_status_idx`; `name`
    gerada do snapshot com `CONSTRAINT roles_tenant_id_name_key UNIQUE (tenant_id, name)`.
  - `invitations` — chave `(tenant_id, invitation_id)`, `invitation_id` é o
    `email`; `status` tipada (`FindInvitationsByStatus`), índice
    `invitations_tenant_id_status_idx`.
  - `memberships` — chave `(tenant_id, membership_id)`, `membership_id` é o
    `subject`; `status` tipada (`FindMembershipsByStatus`), índice
    `memberships_tenant_id_status_idx`; `owner` booleana gerada do snapshot
    (`roles` contém `owner`), que a guarda do `ClaimOwnership` filtra.
  - `buyers` — chave `(tenant_id, buyer_id)`, `buyer_id` é o `subject`;
    `status` tipada (`FindBuyersByStatus`), índice `buyers_tenant_id_status_idx`.
  - O resto do estado em `snapshot` `jsonb`; o ledger de tentativas e a inbox
    são as tabelas de plataforma do kernel.
- **Instante**: inteiro de nanossegundos no domínio.

## Políticas transversais

- **Multi-tenant**: todo agregado carrega `tenant_id`, imposto pelo choke point
  do kernel (ADR-051 do platform). `Role`, `Invitation` e `Membership` vivem no
  tenant de cada loja; `Buyer`, no do operador (ADR-013).
- **Idempotência**: os comandos de borda entram pelo `bff` por `POST` com chave
  de idempotência declarada e passam por `usecase.RunIdempotent` (ADR-056 do
  platform); os internos e os passos externos, pela inbox e pelo
  `ports.AttemptLedger` (ADR-070 do platform). `DefineRole`, `InviteMember`,
  `EstablishMembership` e `RegisterBuyer` também são idempotentes por
  identidade.
- **Autorização**: a permissão de cada rota é conferida no `bff`, porque o
  sujeito e as permissões não atravessam o gRPC (ADR-068 do platform, item 6);
  o teste de rota é a evidência dos cenários de permissão. No contexto, o gancho
  do bloco `application` confere a espécie do tenant antes da transação:
  `Role`, `Invitation` e `Membership` recusam o tenant do operador, e `Buyer`
  exige o tenant do operador (`IAM_OPERATOR_TENANT`, obrigatória na `Config` da
  role `api`; vazia nega toda operação), com `ports.ErrDenied`
  (`PermissionDenied`, 403 no `bff`), sem tocar o banco.
- **Auditoria**: pela trilha de auditoria do kernel (FND-08;
  `observability/audit`), inclusive dos passos externos.
- **Segredo**: a credencial da service account do `iam` no Keycloak entra por
  ambiente e nunca vai a log, evento ou snapshot. Os papéis só de leitura e as
  permissões do FGAP v2 de "Resultado dos spikes" são pré-requisito do `iam` em
  todo ambiente, como os clients `marketplace-store` e `marketplace-seller` e o
  grupo `owner` de cada loja; no de desenvolvimento, o seed os declara, e nos
  demais, a configuração do realm de cada ambiente (fora desta spec).

## Ambiente de desenvolvimento

- O seed complementar do projeto (`apps/backend/bff/deploy/keycloak/seed.sh`)
  deixa prontos, de forma idempotente e com pós-condições: o client
  `marketplace-buyer` (claim fixo do tenant do operador, sem o scope
  `organization`) e o comprador `buyer-demo`, sem Organization e com a realm role
  `iam:buyer`; o client `marketplace-store` com exatamente as 16 client roles do
  catálogo; o
  client `marketplace-seller` (scope `organization`, sem o scope `roles`, com o
  mapper de grupos e `addGroupRoleMappings`); as realm roles `iam:read` e
  `iam:write` para o operador, que fica no grupo de realm `operators` com as
  service accounts; o client `iam-admin`, com a service account e o FGAP v2 de
  "Resultado dos spikes"; e a
  loja de demonstração `loja-demo`, com a Organization (`tenant_id=loja-demo`), o
  grupo `owner` com o catálogo e o usuário `loja-demo-owner`, membro só dela e
  desse grupo.
- O segredo da service account vem de `KEYCLOAK_IAM_ADMIN_SECRET`: com default no
  compose de desenvolvimento e no `scripts/test-keycloak.sh` do `bff`, e pelo
  Secret `keycloak-iam-admin`, sem default, no Job do overlay `dev`.
- O `bff` lê `OIDC_PERMISSION_CLAIMS=scope,realm_access.roles,organization.*.resource_access.marketplace-store.roles`
  no compose, no overlay `dev` e no modelo de secrets do `hmg`.
- O `AUTH_DEV_MOCK` vale só dentro das suítes de teste, como no `tenancy`.

## Critérios de aceite

- [ ] Os gates do workspace verdes para `iam` (`fmt-check`, `vet`, `build`,
  `lint`, `test-race`, `test-distributed` e `e2e` com Postgres,
  `dmpf-context-check.sh`, `modsync --check`, `infrasync --check` e
  verificador).
- [x] Spikes concluídos, com o resultado registrado nesta spec (em "Resultado
  dos spikes") antes da implementação dos comandos externos:
  - **S1** — o claim `organization.<alias>.groups` sai com a barra inicial; a
    decisão voltou para aprovação e ficou o grupo por papel, cujas client roles
    de `marketplace-store` o mapper com `addGroupRoleMappings` entrega em
    `organization.<alias>.resource_access`, só no tenant da loja escolhida.
  - **S2** — o claim fixo `organization.marketplace.tenant_id` do client
    `marketplace-buyer` resolve o tenant no `authn` para um usuário sem
    Organization.
  - **S3** — as permissões mínimas da service account na Admin API para
    Organizations, grupos de Organization com client roles de
    `marketplace-store`, usuários e a realm role `iam:buyer`, e a busca da
    Organization pelo atributo `tenant_id`: papéis só de leitura com o FGAP v2,
    e os resíduos aceitos.
- [ ] As rotas no `bff`, com teste de rota por desfecho, e o `iam` na readiness
  e no e2e do `bff`.
- [ ] Ao subir o compose com o Keycloak, o dono da `loja-demo` se reconhece,
  define um papel, convida um membro, e o token do membro pelo
  `marketplace-seller` traz as permissões do papel; um comprador se cadastra
  pelo `marketplace-buyer` e, no novo token, recebe `iam:buyer`.
- [ ] Cenários:

```text
DADO uma ref ainda desconhecida na loja
QUANDO DefineRole chega com ref "expedicao", name "Expedição" e permissions [fulfillment:write, ordering:read, ordering:read]
ENTÃO o papel nasce active com permissions [fulfillment:write, ordering:read] e RoleDefined vai à outbox

DADO um papel já definido
QUANDO DefineRole chega com a mesma ref e outro conteúdo
ENTÃO aceita sem alterar e sem emitir

DADO qualquer entrada
QUANDO DefineRole chega com ref "owner", com ref "Exp_1", com permissions vazio ou com a permissão "billing:write"
ENTÃO rejeita com iam/role/reserved, iam/role/ref-invalid, iam/role/permissions-empty e iam/role/permission-unknown, respectivamente, e nada é escrito

DADO um papel com um name na loja
QUANDO DefineRole chega com outra ref e o mesmo name
ENTÃO rejeita com iam/role/duplicate-name, sem evento

DADO um papel active
QUANDO ChangeRolePermissions chega com outro conjunto
ENTÃO as permissões mudam e RolePermissionsChanged é emitido; com o mesmo conjunto, aceita sem emitir

DADO um papel retired
QUANDO ChangeRolePermissions ou RetireRole chega
ENTÃO rejeita com iam/role/retired

DADO um papel "expedicao" active com accessVersion 1 e dois membros com ele
QUANDO ChangeRolePermissions troca as permissões
ENTÃO a accessVersion do papel vai a 2, RolePermissionsChanged sai com ela, o SyncRole deixa o grupo "expedicao" só com as novas client roles e as Memberships não mudam

DADO um papel com accessVersion 3 e syncedVersion 1
QUANDO o SyncRole da versão 3 é aceito e, depois, o da versão 2 chega atrasado
ENTÃO syncedVersion fica em 3 e o grupo segue com as permissões atuais do papel

DADO um papel com accessVersion 2
QUANDO ConfirmRoleSynced chega com a versão 3
ENTÃO rejeita com iam/role/version-ahead e syncedVersion não muda

DADO um membro removido cujo MemberRemoved é reentregue depois de ele voltar a active
QUANDO o RevokeMemberAccess atrasado é enviado
ENTÃO o membro segue na Organization e nos grupos dos seus papéis, porque o comando converge pelo status atual

DADO um papel "expedicao" active com membros
QUANDO RetireRole chega
ENTÃO o papel vai a retired, a accessVersion sobe, e o SyncRole tira todas as client roles do grupo "expedicao", sem tirar os membros dele

DADO um e-mail ainda não convidado
QUANDO InviteMember chega com " Ana@Loja.com " e roles [expedicao]
ENTÃO o convite nasce pending com email "ana@loja.com", round 1, e MemberInvited é emitido

DADO qualquer entrada
QUANDO InviteMember chega com email "ana", roles vazio ou roles [owner]
ENTÃO rejeita com iam/invitation/email-invalid, iam/invitation/roles-empty e iam/invitation/role-reserved, respectivamente

DADO um convite failed
QUANDO InviteMember chega para o mesmo e-mail
ENTÃO o convite volta a pending com round 2 e MemberInvited é emitido de novo

DADO um convite pending
QUANDO o ProvisionMember é aceito pelo Keycloak com o sub "kc-123"
ENTÃO o convite vai a provisioned com subject "kc-123", InvitationProvisioned é emitido, e o consumo dele estabelece a Membership "kc-123" com os papéis do convite

DADO um convite pending
QUANDO o ProvisionMember recebe recusa final do Keycloak
ENTÃO o convite vai a failed com o motivo e InvitationFailed é emitido

DADO um convite provisioned
QUANDO ConfirmInvitation ou FailInvitation chega por reentrega
ENTÃO rejeita com iam/invitation/not-pending e o estado não muda

DADO um dono semeado no Keycloak sem Membership no iam
QUANDO ClaimOwnership chega pelo bff com o sujeito do token
ENTÃO a Membership nasce active com roles [owner] e accessVersion 1, e MembershipEstablished é emitido

DADO uma loja cujo dono já reclamou
QUANDO ClaimOwnership chega com o sujeito de outro membro, que tem iam:write
ENTÃO rejeita com iam/membership/owner-claimed e nenhuma Membership muda; com o sujeito do próprio dono, aceita sem emitir

DADO um membro active
QUANDO GrantRole chega com um papel que ele não tem
ENTÃO o papel entra, a accessVersion sobe e MemberAccessChanged é emitido; com papel que ele já tem, aceita sem emitir

DADO um membro active
QUANDO GrantRole chega com "owner"
ENTÃO rejeita com iam/membership/owner-reserved

DADO um membro active com um único papel
QUANDO RevokeRole chega com esse papel
ENTÃO rejeita com iam/membership/last-role

DADO o dono da loja
QUANDO RevokeRole com "owner" ou RemoveMember chega para ele
ENTÃO rejeita com iam/membership/owner-protected

DADO um membro active que não é dono
QUANDO RemoveMember chega
ENTÃO vai a removed, a accessVersion sobe e MemberRemoved é emitido

DADO um membro removed cujo convite está provisioned
QUANDO InviteMember chega para o mesmo e-mail e o convite reaberto é provisionado
ENTÃO o convite volta a pending com round 2 e subject vazio, depois a provisioned, e a Membership volta a active com os papéis do convite e MembershipEstablished é emitido

DADO um membro com accessVersion 3 e syncedVersion 1
QUANDO o SyncMemberAccess da versão 3 é aceito e, depois, o da versão 2 chega atrasado
ENTÃO syncedVersion fica em 3

DADO um membro com o papel "expedicao" retired e o papel "catalogo" active
QUANDO o SyncMemberAccess é enviado
ENTÃO o membro fica nos grupos "expedicao" e "catalogo", e o token traz só as permissões de "catalogo", porque o grupo de um papel retired não tem client roles

DADO um membro com catalog:write na loja A que também é membro da loja B
QUANDO entra pelo marketplace-seller escolhendo a loja B
ENTÃO o token traz o tenant de B e nenhuma permissão da loja A, e o token dele pelo marketplace-buyer ou pelo dmpf-portal não traz nenhuma permissão de loja

DADO um sujeito sem Buyer no operador
QUANDO RegisterBuyer chega pelo bff com name "Bia"
ENTÃO o comprador nasce active com accessVersion 1, BuyerRegistered é emitido e o SyncBuyerAccess concede iam:buyer

DADO um comprador já registrado, active ou blocked
QUANDO RegisterBuyer chega de novo
ENTÃO aceita sem alterar e sem emitir

DADO um comprador active
QUANDO BlockBuyer chega com reason "chargeback recorrente"
ENTÃO vai a blocked, BuyerBlocked é emitido e o SyncBuyerAccess tira iam:buyer, sem desativar o usuário no Keycloak

DADO um comprador blocked
QUANDO UpdateBuyerProfile ou BlockBuyer chega
ENTÃO rejeita com iam/buyer/blocked e iam/buyer/not-active, respectivamente

DADO um comprador blocked
QUANDO UnblockBuyer chega
ENTÃO volta a active, BuyerUnblocked é emitido e o SyncBuyerAccess concede iam:buyer de novo

DADO um principal com iam:write no tenant do operador
QUANDO DefineRole, InviteMember ou qualquer comando de Membership chega
ENTÃO recusa com PermissionDenied (403 no bff) e nada é escrito

DADO um principal com iam:write no tenant de uma loja
QUANDO BlockBuyer ou UnblockBuyer chega
ENTÃO recusa com PermissionDenied (403 no bff) e nada é escrito

DADO um papel com [catalog:write] e sem seller:write, concedido a um membro sincronizado
QUANDO o membro chama uma rota que exige catalog:write e outra que exige seller:write
ENTÃO a primeira passa pela autorização e a segunda recebe 403
```

## Entrega

Uma spec, decomposta em tasks no Linear, cada uma em sua branch:

1. DEVS-159 — spikes S1 a S3 e o seed de desenvolvimento.
2. DEVS-160 — domínio, aplicação, persistência e gRPC de `Role`,
   `Invitation` e `Membership`, com os consumos internos.
3. DEVS-161 — domínio, aplicação, persistência e gRPC de `Buyer`.
4. DEVS-162 — adaptador Keycloak (`ports.ExternalCommand`) sobre a Admin API:
   grupo por papel, client role mappings de `marketplace-store`, membros,
   `iam:buyer`, sob o FGAP v2 da service account.
5. DEVS-163 — ciclo do comando externo de saída: consumos (SyncRole incluído),
   passos (`ConfirmRoleSynced` incluído), ledger e varredura.
6. DEVS-164 — rotas do `bff` e `OIDC_PERMISSION_CLAIMS`.

## Escopo fora

- **Organização do seller, loja e dados bancários**: contexto `seller`; o
  registro e o status da loja são de `tenancy`.
- **Endereços do comprador e métodos de pagamento salvos**: no MVP, o endereço
  de entrega é capturado no checkout pelo `ordering`.
- **Login, senha, MFA e login social**: configuração do Keycloak; a exigência de
  MFA para alteração financeira é do `seller`.
- **Mascaramento de PII do comprador**: responsabilidade de cada contexto que
  expõe o dado.
- **Reação à loja suspensa**: cada consumidor do `tenancy` reage na própria
  spec; o `iam` não bloqueia membros.
- **Transferência de propriedade e mais de um dono**: o `owner` só nasce por
  `ClaimOwnership` e não sai por comando no MVP.
- **Papéis do operador e permissões fora do catálogo**: `tenancy:*`, `iam:*` do
  operador e `iam:buyer` são realm roles semeadas ou concedidas como acima, sem
  `Role` no `iam`.
- **Exclusão do usuário com ofuscação de dados pessoais** (PRD 01): sem comando
  no MVP.
- **Revogação imediata da sessão no bloqueio**: a permissão sai do token na
  renovação; derrubar sessões abertas fica para quando a deny-list do `bff`
  aceitar revogação por evento de domínio.
- **Provisionamento automático do primeiro dono**: no MVP, ele é semeado no
  Keycloak, como a Organization da loja e o grupo `owner` dela (ADR-013).
- **Broker administrativo para a service account do `iam`**: fecharia os
  resíduos do FGAP v2 de "Resultado dos spikes"; no MVP, eles ficam aceitos.
