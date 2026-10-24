---
id: SPEC-8CTAZYX8
slug: cart
title: Bounded context — cart
stage: backlog
priority: P1
depends_on: [SPEC-KF8WCFKA, SPEC-AMMJEEK7]
ticket_url: https://linear.app/mmda/issue/DEVS-150/spec-8ctazyx8-bounded-context-cart
subtask_urls: []
created: 2026-10-21
---

# SPEC-8CTAZYX8: Bounded context — cart

> **Esqueleto de backlog.** Carrinho unificado com itens de N sellers e sessão de checkout. Fase alvo: **MVP**. As
> seções de modelo serão preenchidas e aprovadas na Etapa 1 do pipeline; até
> lá, `/dmpf-new-context` recusa esta spec.

## Contexto do backlog

- **Fase**: MVP — ver `docs/roadmap.md`.
- **Depende de**: SPEC-KF8WCFKA (`inventory`), SPEC-AMMJEEK7 (`pricing`).
- **Fontes**:
- `docs/product/04_cart_checkout.md`
- `docs/product/Bounded Context/Bounded Context - Carrinho de Compras (Shopping Cart _ Basket).md`
- `docs/product/Context Mapping/Context Mapping (Parte 1_ Introdução e Relações Centrais).md`

### Escopo da fase

- Carrinho com itens de vários sellers, agrupados por seller.
- Sessão de checkout com frete informado pelo seller (logística própria).
- Emissão de `CheckoutIniciado` para o `ordering`.

### Pontos a resolver no detalhamento

- Reserva de estoque no checkout (ver `inventory`).
- Cotação de frete aparece no PRD 04 e no BC Entrega: definir o dono a partir da V3.

## Identidade

- **bounded_context**: `cart`
- **name**: `cart`
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

- Cotação de frete por gateway logístico (V3).
- Aplicação de cupons (V3, junto com `pricing`).
