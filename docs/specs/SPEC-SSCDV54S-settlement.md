---
id: SPEC-SSCDV54S
slug: settlement
title: Bounded context — settlement
stage: backlog
priority: P2
depends_on: [SPEC-P0N5D8QX, SPEC-8T0W1B7A]
ticket_url: https://linear.app/mmda/issue/DEVS-156/spec-sscdv54s-bounded-context-settlement
subtask_urls: []
created: 2026-10-21
---

# SPEC-SSCDV54S: Bounded context — settlement

> **Esqueleto de backlog.** Cálculo de comissão, liberação do escrow e repasse ao seller. Fase alvo: **MVP**. As
> seções de modelo serão preenchidas e aprovadas na Etapa 1 do pipeline; até
> lá, `/dmpf-new-context` recusa esta spec.

## Contexto do backlog

- **Fase**: MVP — ver `docs/roadmap.md`.
- **Depende de**: SPEC-P0N5D8QX (`payment`), SPEC-8T0W1B7A (`fulfillment`).
- **Fontes**:
- `docs/product/08_settlement_payout.md`
- `docs/product/Context Mapping/Context Mapping (Parte 2_ Completando o Mapa).md`

### Escopo da fase

- Cálculo do repasse: valor menos take rate, taxas logísticas e chargebacks.
- Liberação do escrow após a confirmação de entrega.
- Repasse registrado e executado manualmente.

### Pontos a resolver no detalhamento

- Não há doc de BC: o PRD 08 é a única fonte.
- Take rate do MVP: valor fixo da plataforma ou por tenant.

## Identidade

- **bounded_context**: `settlement`
- **name**: `settlement`
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

- Payout automático via gateway (V2).
- Conciliação automática e comissão por categoria (V3).
