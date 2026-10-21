---
id: SPEC-79ES5ZRS
slug: seller
title: Bounded context — seller
stage: backlog
priority: P1
depends_on: [SPEC-2FJ0KYKV]
ticket_url: https://linear.app/mmda/issue/DEVS-146/spec-79es5zrs-bounded-context-seller
subtask_urls: []
created: 2026-10-21
---

# SPEC-79ES5ZRS: Bounded context — seller

> **Esqueleto de backlog.** Onboarding, verificação (KYB) e perfil operacional do vendedor. Fase alvo: **MVP**. As
> seções de modelo serão preenchidas e aprovadas na Etapa 1 do pipeline; até
> lá, `/dmpf-new-context` recusa esta spec.

## Contexto do backlog

- **Fase**: MVP — ver `docs/roadmap.md`.
- **Depende de**: SPEC-2FJ0KYKV (`iam`).
- **Fontes**:
- `docs/product/01_identity_organization.md`
- `docs/product/02_kyb_kyc.md`
- `docs/product/Bounded Context/Bounded Context - Gerenciamento de Vendedor (Seller Management).md`

### Escopo da fase

- Seller convidado, perfil da loja e status (pendente, aprovado, suspenso).
- KYB manual: submissão de documentos e revisão por operador.
- Dados de repasse do seller, consumidos por `payment` e `settlement`.

### Pontos a resolver no detalhamento

- Um ou vários sellers por tenant: o modelo do BC suporta vários.
- O PRD 06 diz que 'Identity (ou KYB)' fornece o ID de recebedor do gateway: fixar o dono.

## Identidade

- **bounded_context**: `seller`
- **name**: `seller`
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

- Onboarding self-service e KYB automático com bureaus e tiers de risco (V2).
