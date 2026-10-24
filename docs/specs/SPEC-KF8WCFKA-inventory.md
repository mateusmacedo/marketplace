---
id: SPEC-KF8WCFKA
slug: inventory
title: Bounded context — inventory
stage: backlog
priority: P1
depends_on: [SPEC-EBP7QYYT]
ticket_url: https://linear.app/mmda/issue/DEVS-148/spec-kf8wcfka-bounded-context-inventory
subtask_urls: []
created: 2026-10-21
---

# SPEC-KF8WCFKA: Bounded context — inventory

> **Esqueleto de backlog.** Saldo por SKU e reserva de estoque durante a compra. Fase alvo: **MVP**. As
> seções de modelo serão preenchidas e aprovadas na Etapa 1 do pipeline; até
> lá, `/dmpf-new-context` recusa esta spec.

## Contexto do backlog

- **Fase**: MVP — ver `docs/roadmap.md`.
- **Depende de**: SPEC-EBP7QYYT (`catalog`).
- **Fontes**:
- `docs/product/03_catalog_inventory.md`
- `docs/product/Bounded Context/Bounded Context - Inventário (Inventory Management).md`
- `docs/product/Context Mapping/Context Mapping (Parte 1_ Introdução e Relações Centrais).md`
- `docs/product/Context Mapping/Context Mapping (Parte 2_ Completando o Mapa).md`

### Escopo da fase

- Saldo por SKU informado pelo seller.
- Reserva com expiração, confirmação (baixa) e liberação.

### Pontos a resolver no detalhamento

- Momento da reserva diverge entre docs: PRD 04 (em PAYMENT_PENDING, TTL de 15 min ou 2 h), BC Inventário (no checkout, baixa no pagamento) e BC Carrinho (nunca reserva).
- Evento de gatilho: o Inventário espera `PedidoFinalizado`, o Order emite `PedidoCriado`.
- Baixa por `PagamentoAprovado` direto ou por comando da saga do `ordering`.

## Identidade

- **bounded_context**: `inventory`
- **name**: `inventory`
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

- Multi-armazém e multi-local (V4).
