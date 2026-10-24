---
id: SPEC-EBP7QYYT
slug: catalog
title: Bounded context — catalog
stage: backlog
priority: P1
depends_on: [SPEC-79ES5ZRS]
ticket_url: https://linear.app/mmda/issue/DEVS-147/spec-ebp7qyyt-bounded-context-catalog
subtask_urls: []
created: 2026-10-21
---

# SPEC-EBP7QYYT: Bounded context — catalog

> **Esqueleto de backlog.** Produtos, SKUs, categorias e anúncios dos sellers. Fase alvo: **MVP**. As
> seções de modelo serão preenchidas e aprovadas na Etapa 1 do pipeline; até
> lá, `/dmpf-new-context` recusa esta spec.

## Contexto do backlog

- **Fase**: MVP — ver `docs/roadmap.md`.
- **Depende de**: SPEC-79ES5ZRS (`seller`).
- **Fontes**:
- `docs/product/03_catalog_inventory.md`
- `docs/product/Bounded Context/Bounded Context (BC) de Gerenciamento de Catálogo.md`
- `docs/product/Context Mapping/Context Mapping (Parte 2_ Completando o Mapa).md`

### Escopo da fase

- Produto, SKU e categoria por seller.
- Estados do anúncio e publicação condicionada ao status do seller.
- Listagem e consulta de anúncios publicados (projeção de leitura).

### Pontos a resolver no detalhamento

- O doc de BC do catálogo é rascunho (57 linhas): o PRD 03 é a fonte principal.
- O PRD 03 une catálogo, inventário e preço; aqui o catálogo fica separado (decisão do backlog).

## Identidade

- **bounded_context**: `catalog`
- **name**: `catalog`
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

- Preço e promoções (`pricing`) e saldo de estoque (`inventory`).
- Busca full-text e relevância: lacuna Search & Discovery, sem doc de origem.
- Produtos digitais e serviços (V4).
