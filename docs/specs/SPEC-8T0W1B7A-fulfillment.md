---
id: SPEC-8T0W1B7A
slug: fulfillment
title: Bounded context — fulfillment
stage: backlog
priority: P2
depends_on: [SPEC-JCXRHS7T]
ticket_url: https://linear.app/mmda/issue/DEVS-153/spec-8t0w1b7a-bounded-context-fulfillment
subtask_urls: []
created: 2026-10-21
---

# SPEC-8T0W1B7A: Bounded context — fulfillment

> **Esqueleto de backlog.** Acompanhamento de envio por sub-pedido e confirmação de entrega. Fase alvo: **MVP**. As
> seções de modelo serão preenchidas e aprovadas na Etapa 1 do pipeline; até
> lá, `/dmpf-new-context` recusa esta spec.

## Contexto do backlog

- **Fase**: MVP — ver `docs/roadmap.md`.
- **Depende de**: SPEC-JCXRHS7T (`ordering`).
- **Fontes**:
- `docs/product/07_fulfillment_logistics.md`
- `docs/product/Bounded Context/Bounded Context - Entrega (Fulfillment _ Shipment).md`

### Escopo da fase

- Remessa por sub-pedido com código de rastreio informado pelo seller.
- Confirmação manual de entrega, que dispara a liberação do escrow.

### Pontos a resolver no detalhamento

- Quem confirma a entrega manual no MVP: comprador, seller ou operador.

## Identidade

- **bounded_context**: `fulfillment`
- **name**: `fulfillment`
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

- Etiquetas e integração centralizada com transportadoras (V3).
- Picking e packing, seguro e cotação de frete.
