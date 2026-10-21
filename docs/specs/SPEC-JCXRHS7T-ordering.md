---
id: SPEC-JCXRHS7T
slug: ordering
title: Bounded context — ordering
stage: backlog
priority: P1
depends_on: [SPEC-8CTAZYX8]
ticket_url: https://linear.app/mmda/issue/DEVS-151/spec-jcxrhs7t-bounded-context-ordering
subtask_urls: []
created: 2026-10-21
---

# SPEC-JCXRHS7T: Bounded context — ordering

> **Esqueleto de backlog.** Pedido global financeiro e sub-pedidos por seller, orquestrados por saga. Fase alvo: **MVP**. As
> seções de modelo serão preenchidas e aprovadas na Etapa 1 do pipeline; até
> lá, `/dmpf-new-context` recusa esta spec.

## Contexto do backlog

- **Fase**: MVP — ver `docs/roadmap.md`.
- **Depende de**: SPEC-8CTAZYX8 (`cart`).
- **Fontes**:
- `docs/product/05_order_management.md`
- `docs/product/Bounded Context/Bounded Context - Pedidos (Order Management).md`
- `docs/product/Context Mapping/Context Mapping (Parte 1_ Introdução e Relações Centrais).md`
- `docs/product/Context Mapping/Context Mapping (Parte 2_ Completando o Mapa).md`

### Escopo da fase

- 1 pedido global e N sub-pedidos operacionais, um por seller.
- Saga do pedido: pagamento, baixa de estoque, roteamento ao seller e entrega.
- Estados de exceção: PAYMENT_FAILED, CANCELLED_BY_SELLER, OUT_OF_STOCK, DELIVERY_FAILED.

### Pontos a resolver no detalhamento

- O BC Pedidos não modela multi-seller (sem VendedorId): adotar o GlobalOrder e o SellerOrder do PRD 05.
- Catálogo de eventos integrado com `inventory` e `pricing` (nomes não casam entre os docs).

## Identidade

- **bounded_context**: `ordering`
- **name**: `ordering`
- **Linguagem ubíqua**: [PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Agregados

[PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Comandos (UPRs)

[PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Eventos de domínio

[PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Consultas

[PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Relações entre agregados

[PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Integração

[PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Borda e persistência

[PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Políticas transversais

- **Multi-tenant**: todo agregado carrega `tenant_id` (ver `SPEC-XB4NCW4J`).
- **Idempotência**: [PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]
- **Autorização**: [PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]
- **Auditoria**: [PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Critérios de aceite

- [ ] Os gates do workspace verdes para o módulo do contexto
  (`fmt-check`, `vet`, `build`, `lint`, `test-race`, `test-distributed`,
  `dmpf-context-check.sh`, verificador).
- [ ] [PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Escopo fora

- Devoluções, RMA e disputas (RETURNED, DISPUTED): lacuna sem doc de origem.
- Emissão fiscal: lacuna Fiscal/NF.
