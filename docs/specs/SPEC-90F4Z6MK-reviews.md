---
id: SPEC-90F4Z6MK
slug: reviews
title: Bounded context — reviews
stage: backlog
priority: P3
depends_on: [SPEC-79ES5ZRS, SPEC-JCXRHS7T]
ticket_url: https://linear.app/mmda/issue/DEVS-155/spec-90f4z6mk-bounded-context-reviews
subtask_urls: []
created: 2026-10-21
---

# SPEC-90F4Z6MK: Bounded context — reviews

> **Esqueleto de backlog.** Avaliações de produtos e sellers por compradores com pedido entregue. Fase alvo: **V2**. As
> seções de modelo serão preenchidas e aprovadas na Etapa 1 do pipeline; até
> lá, `/dmpf-new-context` recusa esta spec.

## Contexto do backlog

- **Fase**: V2 — ver `docs/roadmap.md`.
- **Depende de**: SPEC-79ES5ZRS (`seller`), SPEC-JCXRHS7T (`ordering`).
- **Fontes**:
- `docs/product/Bounded Context/Bounded Context - Avaliações (Reviews & Ratings).md`
- `docs/product/Context Mapping/Context Mapping (Parte 2_ Completando o Mapa).md`

### Escopo da fase

- Não entra no MVP: escopo da V2.

### Pontos a resolver no detalhamento

- Nenhum doc define a fase; a V2 é decisão do backlog.

## Identidade

- **bounded_context**: `reviews`
- **name**: `reviews`
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

- Moderação por backoffice: lacuna sem doc de origem.
