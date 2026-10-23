# ADR-013: O tenant é a loja do seller, e o operador guarda o que é de todos

## Status

Aceito — 2026-10-23. Implementa SPEC-XB4NCW4J (DEVS-144).

## Contexto

O marketplace é multi-tenant desde o MVP, com `tenant_id` como discriminador em
toda tabela de todo contexto (`docs/roadmap.md`, decisões de base), imposto pelo
choke point do kernel DMPF (ADR-051 do platform). Faltava dizer o que é um
tenant. Os documentos de produto divergem: o PRD 01
(`docs/product/01_identity_organization.md`) chama de tenant a loja do seller;
o Fundamentos e o BC IAM chamam de tenant a organização white label que opera um
marketplace inteiro.

O kernel rc.7 impõe duas restrições que pesam na escolha:

- Toda leitura e escrita é escopada ao tenant do contexto de execução. Não existe
  escrita de plataforma com alcance total (ADR-068 do platform, item 14).
- A escrita entre tenants existe só por projeção declarada: um consumidor de
  evento grava no tenant de destino por `ports.Projector`, sob uma
  `ProjectionPolicy` que fecha por omissão e com trilha de auditoria que falha
  fechada (ADR-068 do platform, item 8).

O tenant é resolvido pelo `authn` a partir do atributo `tenant_id` da
Organization do Keycloak (`organization.*.tenant_id`; ADR-012).

## Decisão

**O tenant é a loja do seller.** Cada loja é uma Organization no Keycloak, cujo
atributo `tenant_id` é a `ref` da loja, e cada linha que pertence à loja carrega
esse `tenant_id`.

**O operador é um tenant reservado, configurado e não registrado.** Uma
Organization própria do marketplace guarda os dados de quem não pertence a uma
loja (compradores, carrinho, pedido) e o registro das lojas do contexto
`tenancy`. Nos contextos que precisam dele, o operador entra por configuração
(em `tenancy`, `TENANCY_OPERATOR_TENANT`; em desenvolvimento, `marketplace`).

**O que cruza da operação para a loja chega por projeção declarada.** O pedido
nasce no tenant do operador; o sub-pedido de cada loja e a reserva de estoque
chegam ao tenant da loja por `ports.Projector`, com a `ref` da loja como destino
explícito no payload (ADR-068 do platform, item 7). Nenhum contexto escreve
diretamente no tenant de outro.

**O registro de lojas mora no escopo do operador.** Em `tenancy`, o agregado
`Store` (natural por `ref`) é gravado no tenant do operador, onde cada linha é
uma loja; listar as lojas é consulta comum, sem `PlatformReader`. O agregado não
se chama `Tenant` porque a norma do DMPF nomeia a coluna de identidade
`<agregado>_id`, e `tenant_id` já é o discriminador.

## Alternativas descartadas

| Alternativa | Por que foi rejeitada |
| ----------- | --------------------- |
| Tenant = organização white label (Fundamentos, BC IAM) | Escolha do produto pelo vocabulário do PRD 01: o isolamento que importa é o da loja (catálogo, pedidos e dados bancários por loja). O white label passa a ser configuração do operador. |
| Comprador sem tenant | O kernel exige `tenant_id` em toda linha; não há escopo "sem tenant" para dados de negócio. |
| Registro de lojas no tenant de cada loja, escrito pelo operador | A rc.7 não tem escrita de plataforma com alcance total; o operador não grava no tenant de outra loja. |
| Escrita direta entre tenants no checkout | Contraria o IDN-11; o único caminho entre tenants que o kernel aceita é a projeção declarada e auditada. |
| Banco ou schema por tenant | Já descartado nas decisões de base do roadmap: o isolamento é por discriminador, imposto pelo kernel. |

## Consequências

**Positivas:**

- O isolamento de catálogo, estoque, sub-pedidos e repasse por loja vem do
  choke point do kernel, sem filtro manual.
- O caminho entre operação e loja é único, declarado e auditado, e fecha por
  omissão.
- O registro de lojas não precisa de leitura nem escrita de plataforma.

**Negativas:**

- Há dois tipos de tenant (loja e operador), e cada contexto precisa saber em
  qual deles mora cada agregado.
- `cart`, `ordering`, `inventory` e `payment` dependem de projeção declarada
  (`ProjectionPolicy`, trilha de auditoria) desde o MVP, o que custa mais que um
  checkout num tenant só.
- O roadmap e as specs de `iam`, `cart`, `ordering`, `inventory` e `payment`,
  escritos antes desta decisão, precisam ser revistos.
- No MVP, a Organization de cada loja e a do operador, com as permissões, são
  criadas fora do sistema; são pré-condição de deploy fora de desenvolvimento.
