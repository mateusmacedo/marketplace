---
id: SPEC-DSG62Y94
slug: notification
title: Bounded context — notification
stage: backlog
priority: P2
depends_on: [SPEC-2FJ0KYKV, SPEC-JCXRHS7T]
ticket_url: https://linear.app/mmda/issue/DEVS-154/spec-dsg62y94-bounded-context-notification
subtask_urls: []
created: 2026-10-21
---

# SPEC-DSG62Y94: Bounded context — notification

> **Esqueleto de backlog.** Notificações transacionais a compradores e sellers. Fase alvo: **MVP**. As
> seções de modelo serão preenchidas e aprovadas na Etapa 1 do pipeline; até
> lá, `/dmpf-new-context` recusa esta spec.

## Contexto do backlog

- **Fase**: MVP — ver `docs/roadmap.md`.
- **Depende de**: SPEC-2FJ0KYKV (`iam`), SPEC-JCXRHS7T (`ordering`).
- **Fontes**:
- `docs/product/Bounded Context/Bounded Context - Notificações (Notification).md`
- `docs/product/05_order_management.md`
- `docs/product/Context Mapping/Context Mapping (Parte 1_ Introdução e Relações Centrais).md`

### Escopo da fase

- E-mail transacional disparado por eventos do pedido (Conformist ao `ordering`).

### Pontos a resolver no detalhamento

- Lista de eventos que geram notificação no MVP.

## Identidade

- **bounded_context**: `notification`
- **name**: `notification`
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

- Outros canais e preferências de notificação por usuário.
