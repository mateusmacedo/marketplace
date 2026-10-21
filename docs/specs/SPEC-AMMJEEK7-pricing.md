---
id: SPEC-AMMJEEK7
slug: pricing
title: Bounded context — pricing
stage: backlog
priority: P1
depends_on: [SPEC-EBP7QYYT]
ticket_url: https://linear.app/mmda/issue/DEVS-149/spec-ammjeek7-bounded-context-pricing
subtask_urls: []
created: 2026-10-21
---

# SPEC-AMMJEEK7: Bounded context — pricing

> **Esqueleto de backlog.** Preço de venda por SKU e congelamento do preço na compra. Fase alvo: **MVP**. As
> seções de modelo serão preenchidas e aprovadas na Etapa 1 do pipeline; até
> lá, `/dmpf-new-context` recusa esta spec.

## Contexto do backlog

- **Fase**: MVP — ver `docs/roadmap.md`.
- **Depende de**: SPEC-EBP7QYYT (`catalog`).
- **Fontes**:
- `docs/product/03_catalog_inventory.md`
- `docs/product/04_cart_checkout.md`
- `docs/product/Bounded Context/Bounded Context - Precificação e Promoções (Pricing & Promotions).md`
- `docs/product/Context Mapping/Context Mapping (Parte 1_ Introdução e Relações Centrais).md`
- `docs/product/Context Mapping/Context Mapping (Parte 2_ Completando o Mapa).md`

### Escopo da fase

- Preço base por SKU definido pelo seller.
- Cálculo síncrono do preço para carrinho e checkout (gRPC).

### Pontos a resolver no detalhamento

- Preço congelado: o Context Mapping manda o Order consultar o Pricing, mas o payload de `CheckoutIniciado` já traz o preço.
- O Pricing espera `PedidoConfirmado`, que o Order não emite.

## Identidade

- **bounded_context**: `pricing`
- **name**: `pricing`
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

- Promoções, cupons e promoção de frete (V3).
