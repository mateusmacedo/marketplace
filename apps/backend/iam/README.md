# iam

Bounded context `iam`, gerado pelo generator `bounded-context`
(`tools/dmpf-plugin`): um módulo Go, um package por bloco.

Projeto Nx `iam`, tags `type:app`, `scope:backend`, `stack:go` e
`layer:apps` — a camada é a do estágio mais alto entre os blocos, porque é
o primeiro em que toda dependência do módulo está de pé. Import path do módulo:
`github.com/mateusmacedo/marketplace/apps/backend/iam`.

## Blocos e unidades

| Package | Bloco | Unidade | Resumo |
| --- | --- | --- | --- |
| `domain` | `domain` | `iam/domain` | Agregados síncronos e determinísticos, com o desfecho da UPR — sem porta, relógio, contexto nem tipo de wire. |
| `ports` | `port` | `iam/ports` | Fronteira declarada — repositórios, outbox e inbox em tipos do domínio, sem nenhuma realização. |
| `application` | `application` | `iam/application` | Lado do chamador da UPR: casos de uso de escrita na sequência canônica e a disposição do consumo. |
| `provider` | `provider` | `iam/provider-postgres` | Realização em PostgreSQL das portas transacionais do contexto, com esquema e mapeadores próprios, e o adaptador Keycloak dos comandos externos de saída. |
| `app` | `app` | `iam/app` | Composição entre a entrega do transporte e o serviço de aplicação, com a borda gRPC do contexto. |

`block` e `bounded_context` vêm das opções do generator e vão literalmente ao
`dmpf-units.json` — nunca são inferidos do nome do diretório (ADR-012).

## Borda, banco e configuração

O contexto serve só gRPC (`company.iam.service.v1.IamService`), atrás da cadeia de
interceptors do kernel; o REST público é do `bff` (ADR-044, ADR-053). O
`ServiceDesc` de `app/rpc/service.go` declara os 20 métodos de `contract/proto`,
dos agregados `Role`, `Invitation`, `Membership` e `Buyer`, e o teste do
descritor (`kernelgrpc.Uncovered` vazio) reprova método declarado sem handler;
sem nenhum método, o papel `api` recusaria a partida com `serve.ErrNoMethods`.

O banco é próprio: database `iam`, cuja dona é a role migradora
`iam_migrator`, e role de runtime `iam`, sem posse (ADR-063);
schema `public`, tabelas sem prefixo e agregado no plural. O DDL vive em
`provider/schema.sql` e é aplicado no ready do `api` quando `MIGRATE=true`, pela
migradora de `PG_MIGRATE_DSN`, junto com a outbox e a inbox do kernel, seguido
dos grants do kernel e de `provider.Grants`; sem `MIGRATE`, o ready espera que
a role de runtime tenha esses grants. Toda tabela do `schema.sql` precisa de um
grant em `provider.Grants` (`postgres.Mutable` para a que um `Table` escreve),
ou a partida é recusada.
O agregado cujo estado inteiro vive na coluna `snapshot` declara a tabela com
`postgres.SnapshotTable`, sobre um struct de estado privado do provider cujas
tags JSON fixam os bytes gravados:

```go
var orderTable = postgres.SnapshotTable("orders", "order_id",
	func(s domain.Snapshot) orderState { return orderState{Status: int(s.Status)} },
	func(state orderState) domain.Snapshot { return domain.Snapshot{Status: domain.Status(state.Status)} },
	func(s domain.Snapshot, id domain.OrderID) domain.Snapshot { s.ID = id; return s },
)
```

Todo comando passa pela inbox, com `consumer_name` `iam.commands`
(`application.CommandConsumer`), na mesma transação do efeito (FND-04 §7.6,
ADR-056). O generator não escreve casos de uso: cabe a quem escreve o contexto
passar cada comando por `usecase.Execute`, que o corre em `usecase.RunIdempotent`,
e declarar o método em `rpc.Commands()`, que o interceptor usa para exigir a
metadata `idempotency-key`.
O `kernelgrpc.StatusOf`, com que os handlers devolvem o erro, já traduz a
divergência, o comando em andamento e a chave ausente.
Os prazos (`IdempotencyWait` 1s, `IdempotencyRetention` 24h, `OutboxRetention`
168h, `PurgeInterval` 15min, `PurgeBatch` 1000) são a seção `Policies` da
`Config` (`kernelapp.Policies`) e vêm de `Defaults()`; o `api` purga as entradas
de comando vencidas e o `relay`, a outbox publicada.

A configuração vem só do ambiente, sem prefixo `DMPF_`: `PG_DSN`, `GRPC_ADDR`
(`:9090`), `GRPC_INSECURE` ou o trio mTLS (`GRPC_TLS_CERT_FILE`,
`GRPC_TLS_KEY_FILE`, `GRPC_CLIENT_CA_FILE` e `GRPC_TRUSTED_CLIENTS`) e, no
relay e no consumer, `KAFKA_BROKERS`, `KAFKA_IAM_TOPIC`, `KAFKA_IAM_DLQ` e
`KAFKA_GROUP`; `IAM_OPERATOR_TENANT` é obrigatória no `api` e no consumer. As variáveis `GRPC_*` formam a seção `API` da `Config`, lida por
`kernelgrpc.ReadAPIEnv`; o `requirements()` compõe o `Missing()` dela com o do
Kafka e com as variáveis do contexto, e a falta de uma variável obrigatória
recusa a partida nomeando-a. O `PG_MIGRATE_DSN` só é lido no `api` com
`MIGRATE=true`, que o exige. Os dois vêm do `deploy/.env.migrator.example`, que
só o `serve-api` e o `docker:run` carregam (este pela cópia
`deploy/.env.migrator`, que o `deploy-env` cria): o `deploy/.env.example` vale
também para o `relay` e o `consumer`, e a credencial da migradora não chega a
eles.

A unicidade de negócio é do índice (ADR-066), declarada em `postgres.Unique` e
em `provider.Uniques`: `roles_tenant_id_name_key` dá um nome de papel por loja,
e `memberships_tenant_id_owner_key` dá um só dono ativo por loja. A coluna
`owner` de `memberships` é gerada do `snapshot`: `true` quando o vínculo está
ativo com o papel `owner`, `NULL` fora disso, e a constraint não conta `NULL`.
O `ClaimOwnership` não lê antes de decidir: a violação vira a recusa
`iam/membership/owner-claimed`.

## Resolução de dependências

O `go.mod` nasce com `module` e `go` apenas, porque o módulo ainda não importa
irmão nenhum. A partir do primeiro import, o `require` do irmão é gravado pelo
`dmpf-modsync` — que o generator já rodou uma vez ao criar o módulo e que você
roda de novo a cada import novo:

```bash
go run github.com/mateusmacedo/dmpf/tools/dmpf-conformance/cmd/modsync@v1.0.0-rc.7 --root . --write
```

Dentro do workspace quem resolve é o `go.work` da raiz, onde o generator
registrou a entrada `use` e o modsync mantém o `replace` versionado; fora dele,
o `require` resolve pela tag publicada do irmão. O CI confere os dois lados com
`--check`. O generator não grava `go.sum`: o deste módulo, com o bloco
`// indirect` do `go.mod`, vem do alvo `iam:tidy` (`pnpm nx run iam:tidy`).

## Antes do primeiro commit

Unidade nova muda a classificação: regrave o baseline do verificador.

```bash
go run github.com/mateusmacedo/dmpf/tools/dmpf-conformance/cmd/conformance@v1.0.0-rc.7 --root . --write-baseline
```

## Operador e autorização

As lojas são os tenants de `Role`, `Invitation` e `Membership`; os compradores
(`Buyer`, tabela `buyers`) vivem no tenant do operador. O `api` e o consumer
exigem `IAM_OPERATOR_TENANT` (em desenvolvimento, `marketplace`, declarado no
`deploy/.env.example`, no `deploy/compose.yml` e nos ConfigMaps dos overlays
`dev` e `hmg`); o `relay` não a lê.

A autorização (`app/authorization.go`) compõe o `usecase.Permitted` do kernel
com a conferência do tipo de tenant de cada operação. Os comandos da loja pedem
`iam:write` e as consultas, `iam:read`, e o tenant do operador é recusado neles.
Os de `Buyer` pedem `profile` (`RegisterBuyer`), `iam:buyer`
(`UpdateBuyerProfile`), `iam:write` (`BlockBuyer` e `UnblockBuyer`) e `iam:read`
(consultas), e só o tenant do operador é aceito. A recusa é `ports.ErrDenied`
(`PermissionDenied` no gRPC), antes da transação. Com o operador vazio, tudo é
recusado. O `bff` não repassa o sujeito nem as permissões do token ao contexto:
sem sujeito, vale o workload verificado pelo canal (IDN-18), e quem distingue
`iam:buyer` de `iam:read` em `/iam/buyer/me` é a rota do `bff`.

## Consumo de `InvitationProvisioned`

O papel `consumer` lê o tópico do próprio contexto (`KAFKA_IAM_TOPIC`, grupo
`KAFKA_GROUP`, `iam-membership` em desenvolvimento) e estabelece o vínculo do
convite provisionado pelo `ConsumeInvitationProvisioned`, que pede `iam:consume`.
A inbox usa o nome lógico `iam-membership` (`app.ConsumerName`), comum a todas
as réplicas. O `app/sink.go` filtra pelo tipo do evento: entrega de outro tipo
recebe `Ack` sem efeito, e envelope que não desempacota, ou cujo e-mail está
fora da forma de um endereço, é falha de validação terminal, contida na
quarentena do Postgres, sem abrir UoW. A fronteira
(`kernelapp.Boundary.Sources`) admite só o `Source` do relay do `iam`. A
retenção da inbox (`InboxRetention`, 192h) não pode ficar abaixo da janela de
redelivery do canal (INB-14), ou a partida é recusada. A ACL de leitura com o
grupo vem do `deploy/infra.json`, aplicada pelo `infrasync`, e lê o nome do
grupo de `KAFKA_IAM_GROUP` no `.env` da infra local. O `deploy/compose.yml`
repassa a mesma variável ao `KAFKA_GROUP` do consumer, para que a ACL e o
consumer não divirjam.

O generator não gera o papel consumer. Por isso os alvos `serve-consumer` e
`docker:run-consumer` do `project.json` foram escritos à mão, na forma dos
alvos do relay com o papel trocado, iguais aos do contexto de referência
`reservations` do DMPF. É a única exceção à regra de não escrever o
`project.json` à mão, e vale até o generator ganhar suporte a consumer.

## Keycloak como parceiro dos comandos externos

O `iam` projeta papéis, vínculos e compradores no Keycloak por comandos externos
de saída (ADR-070 do platform). Os arquivos `provider/keycloak*.go` realizam
`ports.ExternalCommand` para os cinco comandos da SPEC-2FJ0KYKV ("Integração"):
`SyncRole()`, `ProvisionMember()`, `MemberAccess()` (que serve ao SyncMemberAccess
e ao RevokeMemberAccess, porque os dois convergem pelo `status` do vínculo) e
`BuyerAccess()`. Cada comando lê o Keycloak e converge ao estado desejado que
recebe, então a reentrega e a ordem trocada não duplicam efeito. O adaptador
nunca altera Organization.

- **Payload**: o estado desejado é declarado em `ports` (`RoleAccess`,
  `MemberProvision`, `MemberAccess`, `BuyerAccess`), e quem o serializa é a
  porta `ports.ExternalPayloads`, realizada por `provider.KeycloakPayloads`.
  `ports` e `application` não importam `encoding/json` (depguard), e o JSON grava
  o status como texto (`active`, `retired`, `removed`, `blocked`).
- **Tenant**: vem do contexto de execução. Os comandos de loja recusam o tenant
  do operador, e o `BuyerAccess` só o aceita.
- **Transporte**: usa o cliente `kernelhttp` do kernel, com rotas declaradas, o
  prazo derivado do `ctx` e uma ficha de resiliência própria (`keycloak`). O token
  client credentials fica em cache e, diante de um 401, é renovado uma vez antes
  de a resposta valer como recusa.
- **Desfechos**: `provider.KeycloakOutcomes()` (versão `keycloak-admin-26.8/1`)
  mapeia cada status em `ports.ExternalClass`. Os 404 e 409 esperados por um passo
  (grupo que já existe, membro que já saiu) são tratados no próprio passo.
  Organization ausente, grupo `owner` ausente e o 403 do FGAP são recusa final.
- **Perfil**: `Profile()` declara `KeyNone` (a Admin API não deduplica),
  `EfficacyOnSubmit` e nenhuma consulta de estado.

A configuração vem do ambiente:

| Variável | Uso |
| --- | --- |
| `KEYCLOAK_IAM_URL` | base do Keycloak |
| `KEYCLOAK_IAM_REALM` | realm (`dmpf` em desenvolvimento) |
| `KEYCLOAK_IAM_CLIENT_ID` | client da service account (`iam-admin`) |
| `KEYCLOAK_IAM_ADMIN_SECRET` | segredo da service account |

As quatro variáveis vazias desligam o adaptador. Se só parte delas vier preenchida,
ou se a URL for inválida, a partida é recusada. Os nomes levam `IAM` para que o
`KEYCLOAK_URL` do workspace, que o `.env.example` da raiz entrega a todo teste,
não ligue o adaptador por acaso. O segredo é redigido em `%v`, `%#v`,
JSON e texto, e nunca vai a erro nem a log. `app.NewKeycloak` monta o adaptador e
valida o perfil por `application.ValidateExternalProfile`. Com o adaptador
desligado ele recusa, então quem o chama consulta `cfg.Keycloak.Enabled()` antes. Nenhuma role exige as
variáveis enquanto o ciclo do comando externo não estiver ligado ao consumer. A
service account só tem papéis de leitura de `realm-management`; o que ela altera
vem do FGAP v2 do realm (SPEC-2FJ0KYKV, "Resultado dos spikes").

Os testes unitários usam um dublê `httptest` da Admin API e rodam no
`test-race`. Os de integração têm as tags `integration` e `keycloak` e rodam
contra o Keycloak de testes com o seed do marketplace, pelo script do `bff`. Rode
um de cada vez com os do `bff`, porque o script derruba o Keycloak ao sair:

```bash
cd apps/backend/iam
bash ../../../node_modules/@mateusmacedo/dmpf-plugin/scripts/test-env.sh \
  bash ../bff/scripts/test-keycloak.sh ./provider/...
```
