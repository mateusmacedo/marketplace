# PRD — Payment & Split

## Propósito

Processar as capturas de fundos (Cartão, Pix, Boleto), garantir que os valores pagos não circulem indevidamente nas contas da plataforma (Compliance Regulatório/Bacen), e calcular com precisão milimétrica a divisão dos recebíveis entre Vendedores, Transportadoras e Plataforma (Take Rate).

## Escopo

- Integração com Gateways Sub-adquirentes (Stripe, Pagar.me, Adyen).
- Cálculo das Regras de Split (Comissões).
- Gerenciamento de Escrow (Custódia/Conta Escrow).
- Processamento de Captura, Estorno (Refund) Parcial ou Total, e Chargebacks.
- Antecipação de Recebíveis (se o modelo permitir oferecer adiantamento).

## Fora do escopo

- Liberação do dinheiro na conta bancária do vendedor (Isso é feito pelo Settlement & Payout após a entrega).
- Avaliação de crédito (Empréstimo).

## Modelo de domínio

- `PaymentIntent`: A intenção originada do checkout.
- `Transaction`: Registro do processamento no provedor.
- `SplitRule`: Parametrização percentual ou fixa de repasse (Ex: 10% Plataforma, 90% Seller).
- `EscrowLedger`: Saldo bloqueado aguardando eventos operacionais.
- `ChargebackDispute`: Contestação do titular do cartão no banco.

## Requisitos funcionais

1. Isolar dados PCI (não trafegar número de cartão em claro pelos nossos serviços, usar tokens/Vault do provedor).
2. Definir os recebedores primários do fluxo financeiro no momento da autorização (Split At-Authorization) garantindo aderência regulatória.
3. Reter valores bloqueados (Escrow) até que a plataforma receba o evento `OrderDelivered`.
4. Calcular taxa da plataforma abatendo cupons corretamente. Exemplo: se plataforma deu cupom subsidiado, ela deve abater do seu *take rate*, e não do repasse do seller.
5. Lidar com falhas transitórias e re-tentativas de autorização com o gateway.
6. Gerenciar cancelamentos parciais estornando o valor exato no cartão do cliente e estornando a comissão proporcional da plataforma.
7. Informar ao Order Management sobre sucesso ou falha da operação de captura financeira.

## Estados

`PENDING → AUTHORIZED → CAPTURED → REFUNDED_PARTIALLY | REFUNDED | CHARGEBACK`

## Invariantes

- A soma das partições de um Split Rule não pode ultrapassar 100% ou o valor exato da Transação (Prevenção contra criação ou destruição de dinheiro).
- Operação em `AUTHORIZED` não gera pedido aprovado se não transitar para `CAPTURED`.
- O saldo vai para a Conta Escrow do Recebedor; ele ainda não pode sacar.

## Comandos e eventos

Comandos: `ProcessPayment`, `CapturePayment`, `CalculateSplit`, `IssueRefund`, `HandleChargeback`.

Eventos: `PaymentAuthorized`, `PaymentCaptured`, `SplitCalculated`, `RefundProcessed`, `ChargebackReceived`.

## APIs principais

- `POST /payments/intents`
- `POST /payments/webhooks/gateway` (Recepção de notificações externas)
- `POST /payments/{id}/refunds`

## Critérios de aceitação

- Uma compra de R$100 com take rate de 15% gera uma partição intocável de R$85 para o Seller e R$15 para a Plataforma.
- Um estorno parcial repassa o prejuízo mantendo as proporções ou aplicando regras de prioridade (ex: plataforma estorna seu take rate primeiro).
- A integração absorve delays do arranjo Pix, atualizando o sistema de OMS minutos após o cliente pagar, se houver atraso bancário.

## Métricas

- Taxa de conversão de autorizações de cartão (Autorizados / Tentativas).
- Rejeições por Antifraude (Gateway).
- Volume em Escrow.
- Disputas de Chargeback abertas.

## Dependências

Checkout fornece o Intent. Identity & Organization (ou KYB) fornece os IDs dos recebedores no Gateway. Order Management aguarda e emite os comandos baseados em transições operacionais.