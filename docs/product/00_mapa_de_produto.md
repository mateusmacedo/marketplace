# PRD — Plataforma Multivendor de Marketplace

## Controle do documento

- Versão: 1.0
- Status: proposta para refinamento
- Abrangência: Brasil
- Entidades iniciais: Sellers (Vendedores), Buyers (Compradores), Produtos Físicos
- Papel da empresa: Plataforma intermediadora, processadora de pagamentos (split) e orquestradora de pedidos.

## Visão do produto

Construir uma plataforma B2B2C que permita a vendedores cadastrar e gerenciar seus catálogos de produtos, e a compradores descobrir, comparar e adquirir itens de múltiplos vendedores em uma experiência de compra unificada. A plataforma não é dona do estoque, não atua como transportadora primária, mas garante a segurança da transação retendo valores (escrow) até a entrega e orquestrando o repasse (split) e comissionamento.

## Objetivos

1. Reduzir o tempo e esforço para vendedores iniciarem vendas online (Time-to-Market).
2. Prover uma experiência de checkout unificado para itens de múltiplos vendedores.
3. Garantir a divisão correta e transparente de recebíveis (Split de Pagamento) no momento da transação.
4. Proteger compradores e vendedores através de regras de custódia (Escrow) e resolução de disputas.
5. Produzir rastreabilidade ponta a ponta: do anúncio ao repasse financeiro.

## Não objetivos

- Comprar, armazenar ou deter a propriedade do estoque dos vendedores.
- Operar a logística física de entrega (apenas orquestração com parceiros).
- Conceder crédito direto ao consumidor (embora possa plugar parceiros financeiros).
- Substituir sistemas de ERP dos grandes vendedores (foco em integração).

## Bounded contexts

| Contexto | Responsabilidade | PRD |
| --- | --- | --- |
| Identity & Organization | Identidade de compradores, vendedores e usuários | `01-prd-identity-organization.md` |
| KYB/KYC | Verificação cadastral, risco e prevenção a fraudes de Sellers | `02-prd-kyb-kyc.md` |
| Catalog & Inventory | Gestão de anúncios, categorias, SKUs, preços e estoque | `03-prd-catalog-inventory.md` |
| Cart & Checkout | Carrinho unificado, cálculo de frete, impostos e captura de intenção | `04-prd-cart-checkout.md` |
| Order Management | Orquestração do ciclo de vida do pedido, split lógico por Seller | `05-prd-order-management.md` |
| Payment & Split | Processamento, regras de comissão (take rate) e divisão financeira | `06-prd-payment-split.md` |
| Fulfillment & Logistics | Acompanhamento de envio, rastreio e confirmação de entrega | `07-prd-fulfillment-logistics.md` |
| Settlement & Payout | Liberação de escrow, repasse financeiro e conciliação | `08-prd-settlement-payout.md` |

## Jornada principal

`ONBOARDING (Sellers/Buyers) → DISCOVERY (Search/Catalog) → CHECKOUT → PAYMENT SPLIT → ORDER ROUTING → FULFILLMENT → DELIVERY CONFIRMATION → PAYOUT`

Estados de exceção do pedido principal: `PAYMENT_FAILED`, `CANCELLED_BY_SELLER`, `OUT_OF_STOCK`, `DELIVERY_FAILED`, `RETURNED`, `DISPUTED`.

## Regras transversais

1. O vendedor é a fonte da verdade para disponibilidade de estoque e preço até o momento do checkout.
2. Um pedido só é repassado ao vendedor após autorização e reserva de saldo (captura) do pagamento.
3. O carrinho de compras pode conter itens de *N* vendedores, gerando *N* sub-pedidos operacionais e 1 pedido global financeiro.
4. O dinheiro pago pelo comprador fica em conta gráfica/escrow protegida até o gatilho de liberação (entrega).
5. O repasse (Payout) só ocorre após abatimento de comissões (Take Rate), taxas logísticas e chargebacks.
6. Falhas na integração externa (Gateways, Transportadoras) são tratadas por sagas e retentativas assíncronas.

## Indicadores de produto

- GMV (Gross Merchandise Volume) transacionado.
- Conversão de Checkout (Visitas no carrinho vs. Pedidos Pagos).
- Tempo médio de onboarding aprovado de Sellers.
- Taxa de cancelamento pós-pagamento (ruptura de estoque).
- Tempo médio entre pagamento e entrega confirmada.
- Índice de disputas e chargebacks por milhão transacionado.

## Estratégia de entrega

1. MVP: Vendedores convidados, catálogo simples, 1 meio de pagamento (Pix/Cartão s/ split no gateway), logística própria do seller.
2. V2: Self-service onboarding, Split de Pagamento automático via Gateway parceiro.
3. V3: Integração logística centralizada (geração de etiquetas pela plataforma), regras complexas de comissionamento por categoria.
4. V4: Multi-armazém, produtos digitais e serviços.

## Critério de conclusão do produto inicial

Um comprador consegue encontrar produtos de dois vendedores diferentes, colocar no mesmo carrinho, fazer um único pagamento. O sistema deve dividir os pedidos, rotear para cada vendedor preparar o envio e, após a confirmação manual de entrega, dividir o dinheiro calculando a comissão da plataforma, sem exposição a fraude básica.