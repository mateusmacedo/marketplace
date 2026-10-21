# PRD — Catalog & Inventory

## Propósito

Centralizar, padronizar e distribuir a disponibilidade de produtos e SKUs de múltiplos vendedores, atuando como a fonte primária de verdade para o descobrimento (vitrine) e a base para a cotação no carrinho de compras.

## Escopo

- Gestão da árvore de categorias global do marketplace.
- Cadastro e normalização de Produtos (PIM) e seus SKUs (Variações).
- Controle de preço (de/por) e estoque em tempo real (ou near-real-time).
- Aprovação e moderação de anúncios (Trust & Safety).
- Mapeamento de atributos obrigatórios por categoria.

## Fora do escopo

- Exibição (Search Engine e Vitrines); este contexto foca no *dado* não na *busca* indexada.
- Cálculo final de descontos de carrinho (cupons promocionais do checkout).

## Modelo de domínio

- `CategoryTree`: Estrutura taxonômica do marketplace.
- `GlobalProduct` (opcional): Catálogo mestre (BuyBox) se produtos iguais forem agrupados.
- `SellerListing`: O anúncio específico de um vendedor.
- `SKU`: Item mantido em estoque (Stock Keeping Unit).
- `InventoryLevel`: Quantidade disponível, reservada e vendida.
- `PriceSchedule`: Política de precificação temporal (promoções).
- `ModerationStatus`: Estado de conformidade do anúncio.

## Requisitos funcionais

1. Permitir que o Seller cadastre produtos um-a-um via painel ou via upload de planilha (Lote).
2. Forçar preenchimento de dimensões (A x L x C) e Peso para garantir cálculo de frete futuro.
3. Atualizar estoque via chamadas de API de alta concorrência.
4. Decrementar estoque logicamente (Reserva) no momento da criação do pedido e fisicamente na confirmação de pagamento.
5. Devolver estoque reservado se o pagamento falhar ou o carrinho expirar.
6. Sinalizar produtos que infringem políticas (Moderação Ativa) e ocultá-los da vitrine pública.
7. Disparar webhooks ou mensagens de atualização de catálogo para indexação nos motores de busca (Elasticsearch/Algolia).

## Estados do Listing (Anúncio)

`DRAFT → PENDING_MODERATION → ACTIVE → PAUSED_BY_SELLER → OUT_OF_STOCK → BLOCKED`

## Invariantes

- Um SKU não pode ter quantidade de estoque negativa.
- A exclusão de um produto inativa o anúncio, mas não o apaga do histórico de pedidos passados.
- Dimensões e peso não podem ser nulos para SKUs físicos.
- Apenas Sellers com status KYC `APPROVED` podem ter anúncios em `ACTIVE`.

## Comandos e eventos

Comandos: `CreateListing`, `UpdatePrice`, `AdjustInventory`, `ReserveInventory`, `ReleaseInventory`, `ModerateListing`.

Eventos: `ListingCreated`, `PriceChanged`, `InventoryUpdated`, `InventoryReserved`, `InventoryDepleted`, `ListingBlocked`.

## APIs principais

- `POST /listings`
- `PUT /listings/{skuId}/inventory`
- `PUT /listings/{skuId}/price`
- `POST /catalog/reservations`
- `POST /catalog/reservations/{id}/release`

## Critérios de aceitação

- Atualizações de preço ou estoque refletem nos motores de busca em menos de 10 segundos.
- Uma compra concorrente por dois usuários no último item em estoque garante a compra ao primeiro que concluir o checkout, falhando a reserva do segundo graciosamente.
- Produtos marcados como `BLOCKED` por moderadores perdem sua URL pública instantaneamente.

## Métricas

- Taxa de rejeição de anúncios na moderação.
- Rupturas de estoque (vendeu sem ter).
- Latência nas atualizações de inventário (P99).
- Quantidade de SKUs ativos no marketplace.

## Dependências

KYB/KYC valida o status da loja. Search & Discovery consome os eventos para atualizar o índice. Cart & Checkout consulta preço e reserva estoque.