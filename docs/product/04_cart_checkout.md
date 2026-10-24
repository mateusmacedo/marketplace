# PRD — Cart & Checkout

## Propósito

Agrupar intenções de compra de múltiplos vendedores em uma única experiência contínua, calcular custos logísticos reais, aplicar regras de desconto e preparar o payload final (Order) para processamento financeiro.

## Escopo

- Gerenciamento de sessão de carrinho (persistência e recuperação).
- Separação visual e lógica dos itens por Vendedor (Seller).
- Cotação de frete multi-origem em tempo real (Integration via Gateways Logísticos).
- Aplicação de cupons (Plataforma vs. Seller).
- Captura de dados de entrega, faturamento e método de pagamento.
- Conversão de Carrinho em Intenção de Pagamento.

## Fora do escopo

- Processamento de cartões de crédito (Feito pelo Payment & Split).
- Decremento final de estoque (Apenas solicita a Reserva ao Catalog).
- Roteamento logístico (Apenas consulta as opções e custos de frete).

## Modelo de domínio

- `Cart`: Recipiente efêmero de itens, vinculado a um BuyerProfile ou sessão anônima.
- `CartItem`: Referência ao SKU, quantidade e preço congelado na adição.
- `SellerPackage`: Agrupamento de CartItems por Vendedor/Origem para cotação de frete.
- `ShippingQuote`: Opções de entrega (Normal, Expresso) retornadas para um pacote.
- `CheckoutSession`: O processo irreversível de finalização (Checkout transparente).

## Requisitos funcionais

1. Adicionar, alterar quantidade e remover itens preservando o histórico da sessão.
2. Agrupar logicamente os itens por Vendedor, calculando sub-totais separados.
3. Consultar provedores de frete com base no CEP do Comprador e os respectivos CEPs de origem dos Vendedores.
4. Somar o custo de fretes distintos em um único total transparente para o comprador.
5. Revalidar preço e disponibilidade de estoque imediatamente antes do botão de pagamento.
6. Aplicar regras matemáticas de cupom: desconto restrito a produtos da plataforma ou produtos do vendedor X.
7. Bloquear checkout se qualquer item se tornar indisponível, oferecendo ao usuário a opção de "remover itens esgotados".
8. Gerar a carga estruturada para Ordem e Pagamento garantindo os centavos cravados (prevenção a erros de arredondamento).

## Estados do Carrinho / Checkout

`ACTIVE_CART → ABANDONED`
`CHECKOUT_INITIATED → QUOTING_SHIPPING → PAYMENT_PENDING → CONVERTED_TO_ORDER`

## Invariantes

- O valor total do Checkout é *estritamente* a soma dos itens + frete - descontos.
- Um carrinho convertido em ordem se torna imutável e sua sessão morre.
- A reserva de estoque ocorre no momento em que a `CheckoutSession` entra em `PAYMENT_PENDING`, com TTL (Time-To-Live) curto (ex: 15 minutos para cartão, 2 horas para boleto/Pix).

## Comandos e eventos

Comandos: `AddItem`, `RemoveItem`, `CalculateShipping`, `ApplyCoupon`, `InitiateCheckout`, `FinalizeCheckout`.

Eventos: `ItemAdded`, `CartAbandoned`, `CheckoutInitiated`, `ShippingCalculated`, `CheckoutFailed`, `OrderIntentCreated`.

## APIs principais

- `POST /carts/{id}/items`
- `POST /carts/{id}/shipping-quotes`
- `POST /checkout/sessions`
- `POST /checkout/sessions/{id}/complete`

## Critérios de aceitação

- Um carrinho contendo itens do Vendedor A e Vendedor B apresenta 2 opções de frete (uma para cada), e o usuário precisa selecionar ambas.
- Mudança de preço no catálogo enquanto o item está no carrinho atualiza o valor apenas ao entrar na página de Checkout, avisando o usuário.
- Se a API dos Correios falhar, o sistema deve usar uma tabela de contingência pre-carregada ou avisar o comprador sem quebrar o site todo.

## Métricas

- Taxa de abandono de carrinho (Cart Abandonment Rate).
- Tempo gasto na página de checkout.
- Erros de cotação de frete (Timeout).
- Carrinhos perdidos por mudança de preço/estoque.

## Dependências

Catalog & Inventory fornece produtos e reservas. Order Management recebe a intenção de ordem. Identity & Organization fornece os endereços de Buyers e Sellers.