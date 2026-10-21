---
id: SPEC-DXAWGZ43
slug: platform
title: Plataforma do marketplace — workspace DMPF e borda
stage: backlog
priority: P0
depends_on: []
ticket_url: https://linear.app/mmda/issue/DEVS-143/spec-dxawgz43-plataforma-do-marketplace-workspace-dmpf-e-borda
subtask_urls: []
created: 2026-10-21
---

# SPEC-DXAWGZ43: Plataforma do marketplace — workspace DMPF e borda

> **Esqueleto de backlog.** Fundação técnica sobre a qual os bounded contexts
> são gerados. Fase alvo: **MVP**. Detalhamento na Etapa 1 do pipeline.

## Resumo

Preparar o workspace para gerar os bounded contexts do marketplace com o
`@mateusmacedo/dmpf-plugin` e expor a borda HTTP autenticada: `init` do DMPF com
a identidade real do repositório, infraestrutura local e o `bff` com
autenticação via Logto atrás do Traefik.

## Contexto

- **Fase**: MVP — ver `docs/roadmap.md`.
- **Decisão de stack**: serviços em Go via DMPF (ADR-011); as menções a Java
  Quarkus em `docs/product/` não valem para a implementação.
- **Fontes**:
- `docs/product/Fundamentos - Conceitos White Label, Multi-Tenant e Visão Arquitetural Inicial.md`
- `docs/product/Cross-Cutting Concerns/Cross-Cutting Concerns (Parte 2_ Fluxo de Autenticação e Autorização).md`
- `docs/product/Cross-Cutting Concerns/Cross-Cutting Concerns (Parte 3_ Design das APIs - REST, GraphQL, CLI).md`
- `docs/product/Cross-Cutting Concerns/Cross-Cutting Concerns Parte 4_ Comunicação Síncrona Interna com gRPC.md`
- `docs/product/Deployment e Infraestrutura/Deployment e Infraestrutura Parte 1_ Conceitos Fundamentais e Containerização.md`
- `docs/product/Deployment e Infraestrutura/Deployment e Infraestrutura Parte 2_ Orquestração de Containers com Kubernetes.md`
- `docs/product/Deployment e Infraestrutura/Deployment e Infraestrutura Parte 3_ Pipelines de CI_CD.md`
- `docs/guides/primeiro-bounded-context.md`

### Escopo da fase

- `init` do DMPF: `modulePrefix`, `composeProject`, `imageRegistry` e
  `bufModule` reais no `dmpf.json` (hoje `example.com/change-me` bloqueia o
  generator `bounded-context`).
- Infraestrutura local: PostgreSQL, RabbitMQ e Redis no compose do DMPF.
- `bff` com as rotas REST da borda e propagação do tenant.
- Autenticação: Logto como IdP e JWT validado no Traefik.

### Pontos a resolver no detalhamento

- `modulePrefix`: `github.com/mateusmacedo/marketplace` (remote `origin`).
- Stack do `bff` e campo `edge` do `dmpf.json` (hoje vazio).
- GraphQL para consultas (Cross-Cutting Parte 3) no MVP ou depois.

## Requisitos

[PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Camadas afetadas

[PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Localização de código

[PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Design

[PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Decisões técnicas

[PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Verificação e testes

### Critérios de aceite

- [ ] [PENDENTE — detalhar na Etapa 1 do pipeline, quando a spec for puxada do backlog.]

## Escopo fora

- Kubernetes, Helm e Argo CD de ambientes remotos.
- IaC (Deployment Parte 4 não existe em `docs/product/`).
