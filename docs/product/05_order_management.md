# PRD — Order Management (OMS)

## Propósito

Gerenciar o ciclo de vida completo do pedido, desmembrando a compra única do cliente em pedidos operacionais isolados para cada Vendedor, e orquestrando as trocas de estado com Pagamento, Estoque e Logística.

## Escopo

- Criação do Pedido Global (Visão do Comprador).
- Criação dos Sub-pedidos (Visão do Vendedor).
- Máquina de estados (Aguardando Pagamento, Aprovado, Faturado, Enviado, Entregue, Cancelado).
- Orquestração de estornos parciais e devoluções (RMA).
- Centro de comunicação transacional (notificações para Buyer e Seller).

## Fora do escopo

- Interação direta com gateways adquirentes (Delegado ao Payment & Split).
- Geração física de etiquetas (Delegado ao Fulfillment).
- Chat direto entre comprador e vendedor (Pode ser um produto apartado).

## Modelo de domínio

- `GlobalOrder`: A compra inteira. Agrupa o pagamento único do comprador.
- `SellerOrder`: A fração do pedido direcionada a um Vendedor.
- `OrderItem`: Referência persistida do produto vendido (snapshot de preço e nome).
- `OrderStatusTimeline`: Histórico imutável de transições de status.
- `CancellationRequest`: Solicitação de cancelamento (pelo comprador ou plataforma).
- `ReturnRequest`: Processo de logística reversa.

## Requisitos funcionais

1. Receber o `OrderIntent` do Checkout e persistir o documento imutável do pedido.
2. Aguardar a confirmação assíncrona de pagamento (Webhook) para mudar o status de `PAYMENT_PENDING` para `APPROVED`.
3. Notificar o(s) Vendedor(es) apenas quando seus `SellerOrders` estiverem `APPROVED`.
4. Permitir que o vendedor atualize os estados operacionais (`INVOICED`, `SHIPPED`).
5. Processar cancelamentos pré-envio estornando o pagamento total ou parcial (se cancelado um SellerOrder apenas).
6. Coordenar a logística reversa se o pedido for devolvido (RMA).
7. Proteger o comprador contra atualização indevida (ex: Seller marca como Entregue mas transportadora não confirma).

## Estados

*Global Order*: `PENDING_PAYMENT → PAYMENT_FAILED | APPROVED → PARTIALLY_FULFILLED | FULFILLED → CLOSED`
*Seller Order*: `APPROVED → INVOICED → PREPARING → SHIPPED → DELIVERED | RETURNED | CANCELLED`

## Invariantes

- Um `SellerOrder` não pode avançar de estado sem que o `GlobalOrder` esteja `APPROVED`.
- Cancelamentos antes da nota fiscal (Invoiced) não requerem logística reversa.
- Os preços persistidos no `OrderItem` nunca mudam, mesmo que o catálogo mude o preço no futuro.
- A soma financeira dos `SellerOrders` deve ser idêntica ao valor do `GlobalOrder`.

## Comandos e eventos

Comandos: `PlaceOrder`, `ConfirmPayment`, `FailPayment`, `InvoiceOrder`, `ShipOrder`, `ConfirmDelivery`, `CancelOrder`.

Eventos: `OrderPlaced`, `PaymentConfirmed`, `SellerOrderApproved`, `OrderInvoiced`, `OrderShipped`, `OrderDelivered`, `OrderCancelled`, `RefundRequested`.

## APIs principais

- `GET /orders/global/{id}` (Visão Buyer/Admin)
- `GET /orders/seller?status=APPROVED` (Visão Seller)
- `POST /orders/seller/{id}/invoice`
- `POST /orders/seller/{id}/cancel`

## Critérios de aceitação

- Um pedido pago contendo Produto A (Vendedor X) e Produto B (Vendedor Y) gera alertas paralelos para X e Y.
- Se o Vendedor X cancelar seu pedido, a plataforma orquestra um estorno financeiro *parcial* apenas referente à parte do Vendedor X, mantendo a operação do Vendedor Y intacta.
- Comprador pode ver o status individual do pacote X e do pacote Y no painel.

## Métricas

- SLA de postagem do Vendedor (Tempo entre Approved e Shipped).
- Taxa de cancelamento por ruptura de estoque (Seller cancela pós pago).
- Tempo médio de resolução de Devoluções.

## Dependências

Cart & Checkout inicia o fluxo. Payment & Split é chamado para processar/estornar valores. Fulfillment alimenta eventos de trânsito (SHIPPED/DELIVERED).