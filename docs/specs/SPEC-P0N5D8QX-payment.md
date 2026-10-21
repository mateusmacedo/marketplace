---
id: SPEC-P0N5D8QX
slug: payment
title: Bounded context — payment
stage: backlog
priority: P1
depends_on: [SPEC-JCXRHS7T]
ticket_url: https://linear.app/mmda/issue/DEVS-152/spec-p0n5d8qx-bounded-context-payment
subtask_urls: []
created: 2026-10-21
---

# SPEC-P0N5D8QX: Bounded context — payment

> **Esqueleto de backlog.** Captura do pagamento do comprador e custódia (escrow) até a liberação. Fase alvo: **MVP**. As
> seções de modelo serão preenchidas e aprovadas na Etapa 1 do pipeline; até
> lá, `/dmpf-new-context` recusa esta spec.

## Contexto do backlog

- **Fase**: MVP — ver `docs/roadmap.md`.
- **Depende de**: SPEC-JCXRHS7T (`ordering`).
- **Fontes**:
- `docs/product/06_payment_split.md`
- `docs/product/Bounded Context/Bounded Context - Pagamentos (Payment Management).md`
- `docs/product/Context Mapping/Context Mapping (Parte 1_ Introdução e Relações Centrais).md`

### Escopo da fase

- 1 meio de pagamento via gateway, sem split no gateway.
- Escrow lógico do valor pago até o gatilho de liberação.
- Reembolso e registro de chargeback; nunca armazenar dados de cartão.

### Pontos a resolver no detalhamento

- Meio do MVP: Pix ou cartão (o PRD 00 cita os dois).
- Escrow fica aqui e a liberação em `settlement` (decisão do backlog); o PRD 06 e o PRD 08 divergiam.

## Identidade

- **bounded_context**: `payment`
- **name**: `payment`
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

- Split automático no gateway parceiro (V2).
- Comissão por categoria (V3).
