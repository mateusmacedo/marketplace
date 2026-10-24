---
id: SPEC-XB4NCW4J
slug: tenancy
title: Bounded context — tenancy
stage: backlog
priority: P0
depends_on: [SPEC-DXAWGZ43]
ticket_url: https://linear.app/mmda/issue/DEVS-144/spec-xb4ncw4j-bounded-context-tenancy
subtask_urls: []
created: 2026-10-21
---

# SPEC-XB4NCW4J: Bounded context — tenancy

> **Esqueleto de backlog.** Tenant como unidade de isolamento da plataforma multi-tenant e white label. Fase alvo: **MVP**. As
> seções de modelo serão preenchidas e aprovadas na Etapa 1 do pipeline; até
> lá, `/dmpf-new-context` recusa esta spec.

## Contexto do backlog

- **Fase**: MVP — ver `docs/roadmap.md`.
- **Depende de**: SPEC-DXAWGZ43 (`platform`).
- **Fontes**:
- `docs/product/Bounded Context/Bounded Context - Configuração do Tenant (Tenant Configuration).md`
- `docs/product/Fundamentos - Conceitos White Label, Multi-Tenant e Visão Arquitetural Inicial.md`
- `docs/product/Cross-Cutting Concerns/Cross-Cutting Concerns (Parte 2_ Fluxo de Autenticação e Autorização).md`

### Escopo da fase

- Cadastro do tenant e seu ciclo de vida (ativo/suspenso).
- `tenant_id` como discriminador obrigatório em todos os contextos.
- Resolução do tenant a partir do JWT do Logto (`org_id`).

### Pontos a resolver no detalhamento

- Estratégia de isolamento: o Fundamentos lista discriminador, schema e banco por tenant sem escolher; os docs de BC usam discriminador implícito.
- Vocabulário: no PRD 01 'tenant' é a loja do seller; no Fundamentos e no BC IAM é a organização white label.

## Identidade

- **bounded_context**: `tenancy`
- **name**: `tenancy`
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

- Tema, domínios customizados e projeção domínio→tenant para o Traefik (V2).
- Feature flags e credenciais por tenant via Vault (V2).
- Regras de comissão base por tenant e por categoria (V3).
