# Roadmap — Marketplace multivendor

Plataforma B2B2C multi-tenant e white label: sellers publicam catálogos e
compradores compram de vários sellers num checkout único, com escrow até a
entrega e repasse com comissão. A visão de produto está em
`docs/product/00_mapa_de_produto.md`; a modelagem por contexto, nos demais
documentos de `docs/product/`.

As fases seguem a "Estratégia de entrega" do mapa de produto. Cada bounded
context tem uma spec em `docs/specs/`, criada como esqueleto em `stage: backlog`
e detalhada quando o pipeline de feature a puxar.

## Decisões de base

- **Stack**: serviços em Go gerados pelo `@mateusmacedo/dmpf-plugin` (ADR-011).
  As menções a Java Quarkus em `docs/product/` não valem para a implementação.
- **Multi-tenant desde o MVP**: `tenant_id` como discriminador em todos os
  contextos; tema, domínios e feature flags por tenant só a partir da V2.
- **Catálogo e Inventário separados**, como no Context Mapping.
- **KYB dentro de `seller`**.
- **Escrow em `payment`, liberação em `settlement`**.

## Now — MVP

Critério de conclusão: um comprador encontra produtos de dois sellers, compra
num único pagamento, o pedido é dividido por seller e, após a confirmação manual
de entrega, o valor é repassado já descontada a comissão.

| Ordem | Spec | Contexto | Prioridade | Depende de |
| --- | --- | --- | --- | --- |
| 0 | `SPEC-DXAWGZ43` | `platform` | P0 | — |
| 1 | `SPEC-XB4NCW4J` | `tenancy` | P0 | `platform` |
| 2 | `SPEC-2FJ0KYKV` | `iam` | P0 | `tenancy` |
| 3 | `SPEC-79ES5ZRS` | `seller` | P1 | `iam` |
| 4 | `SPEC-EBP7QYYT` | `catalog` | P1 | `seller` |
| 5 | `SPEC-KF8WCFKA` | `inventory` | P1 | `catalog` |
| 6 | `SPEC-AMMJEEK7` | `pricing` | P1 | `catalog` |
| 7 | `SPEC-8CTAZYX8` | `cart` | P1 | `inventory`, `pricing` |
| 8 | `SPEC-JCXRHS7T` | `ordering` | P1 | `cart` |
| 9 | `SPEC-P0N5D8QX` | `payment` | P1 | `ordering` |
| 10 | `SPEC-8T0W1B7A` | `fulfillment` | P2 | `ordering` |
| 11 | `SPEC-SSCDV54S` | `settlement` | P2 | `payment`, `fulfillment` |
| 12 | `SPEC-DSG62Y94` | `notification` | P2 | `iam`, `ordering` |

## Next — V2

- Onboarding self-service de sellers e KYB automático (`seller`).
- Split automático via gateway parceiro (`payment`) e payout automático (`settlement`).
- White label: tema, domínios e feature flags por tenant (`tenancy`).
- Avaliações de produtos e sellers: `SPEC-90F4Z6MK` (`reviews`, P3).

## Later — V3 e V4

- **V3**: etiquetas e integração centralizada com transportadoras (`fulfillment`),
  comissão por categoria (`tenancy`, `settlement`), promoções e cupons
  (`pricing`, `cart`), cotação de frete por gateway logístico.
- **V4**: multi-armazém (`inventory`), produtos digitais e serviços (`catalog`).

## Lacunas sem documento de origem

Domínios citados nos documentos de produto, mas sem PRD nem modelagem. Ficam
fora do backlog até haver discovery próprio.

- **Search & Discovery** — busca e relevância; no MVP, a listagem do catálogo basta.
- **Fiscal / NF** — emissão de nota fiscal por sub-pedido.
- **Disputas, devoluções e RMA** — estados RETURNED e DISPUTED do pedido.
- **Backoffice e moderação** — operação de KYB, anúncios e avaliações.
- **Observabilidade** — o Cross-Cutting Parte 1 não existe em `docs/product/`.
- **IaC** — o Deployment Parte 4 não existe em `docs/product/`.
