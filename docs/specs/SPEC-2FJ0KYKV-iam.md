---
id: SPEC-2FJ0KYKV
slug: iam
title: Bounded context — iam
stage: backlog
priority: P0
depends_on: [SPEC-XB4NCW4J]
ticket_url: https://linear.app/mmda/issue/DEVS-145/spec-2fj0kykv-bounded-context-iam
subtask_urls: []
created: 2026-10-21
---

# SPEC-2FJ0KYKV: Bounded context — iam

> **Esqueleto de backlog.** Identidade de compradores e usuários de sellers, papéis e autorização. Fase alvo: **MVP**. As
> seções de modelo serão preenchidas e aprovadas na Etapa 1 do pipeline; até
> lá, `/dmpf-new-context` recusa esta spec.

## Contexto do backlog

- **Fase**: MVP — ver `docs/roadmap.md`.
- **Depende de**: SPEC-XB4NCW4J (`tenancy`).
- **Fontes**:
- `docs/product/01_identity_organization.md`
- `docs/product/Bounded Context/Bounded Context - Identidade e Acesso (IAM).md`
- `docs/product/Cross-Cutting Concerns/Cross-Cutting Concerns (Parte 2_ Fluxo de Autenticação e Autorização).md`

### Escopo da fase

- Buyer e usuários de seller vinculados a um tenant.
- Papéis e RBAC consultado pelos demais contextos (gRPC).
- Integração com o Logto como IdP (Authorization Code com PKCE).

### Pontos a resolver no detalhamento

- O PRD 01 carrega SellerOrganization, Store e dados bancários: definir a fronteira com `seller`.
- Logto Organization mapeada para tenant: confirmar 1:1.

## Identidade

- **bounded_context**: `iam`
- **name**: `iam`
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

- Organização do seller, loja e dados bancários (contexto `seller`).
