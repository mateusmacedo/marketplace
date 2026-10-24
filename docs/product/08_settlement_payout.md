# PRD — Settlement & Payout

## Propósito

Liquiditar as operações financeiras em custódia (Escrow), processar transferências efetivas (saques/payouts) para os domicílios bancários dos vendedores e conciliar divergências financeiras garantindo saúde do fluxo de caixa.

## Escopo

- Movimentação do saldo `Escrow` para `AvailableBalance` após confirmação de recebimento.
- Criação de Ordens de Pagamento (Transferência bancária, TED/Pix).
- Extrato financeiro da loja e relatórios operacionais.
- Ciclos de liquidação automáticos (ex: saques semanais) ou sob demanda (manual via painel).
- Prevenção de saldo negativo (cobranças retroativas ou abatimento em recebíveis futuros por chargeback).

## Fora do escopo

- Divisão primária da comissão na autorização (Split at Auth, tratado no Payment & Split).
- Tratativa de emissão de NF da venda do produto (Tratado no OMS / Seller). A plataforma emite apenas a NF de prestação de serviços sobre o Take Rate.

## Modelo de domínio

- `LedgerAccount`: Conta gráfica do vendedor.
- `LedgerEntry`: Partida dobrada financeira (Crédito, Débito).
- `PayoutRequest`: Intenção de transferir saldo disponível para o banco.
- `PayoutTransaction`: A efetivação da transferência no BACEN/Arranjo Pix.
- `ConciliationBatch`: Processo de batimento entre sistema e Gateway/Bancos.

## Requisitos funcionais

1. Escutar o evento `OrderDelivered` (ou liberação antecipada programada) e creditar o montante descontado do vendedor em sua conta gráfica de `Available`.
2. Prover histórico imutável (Ledger) explicando toda mudança de saldo (Venda A, Estorno B, Tarifa Logística C).
3. Reter reservas financeiras baseadas em risco (Rolling Reserve) se o KYC assim parametrizar.
4. Processar `PayoutRequest` automaticamente descontando taxas de TED/Pix.
5. Suspender repasses se o Seller estiver sob investigação por anomalia (Fraude detectada tardiamente).
6. Executar rotinas de conciliação batendo lançamentos internos vs. arquivo de retorno CNAB/API de recebimentos dos Gateways.
7. Aplicar o débito de Chargebacks em vendas futuras se o saldo atual for zero (Saldo devedor).

## Estados do Payout

`REQUESTED → PROCESSING → COMPLETED | FAILED_BANK_REJECTION`

## Invariantes

- A soma de todos os saldos da plataforma deve bater com a conta centralizadora de garantia no banco físico.
- Nenhuma modificação em `LedgerEntry` é permitida. Correções exigem lançamentos contrapostos (Estornos compensatórios).
- Repasses só podem ocorrer para conta bancária previamente homologada no KYC, pertencente ao mesmo CNPJ do vendedor.

## Comandos e eventos

Comandos: `ReleaseEscrow`, `RequestPayout`, `ExecutePayout`, `ApplyAdjustment`, `ConciliateTransactions`.

Eventos: `BalanceAvailable`, `PayoutRequested`, `PayoutCompleted`, `PayoutFailed`, `LedgerAdjusted`.

## APIs principais

- `GET /finances/sellers/{id}/balance`
- `GET /finances/sellers/{id}/statement`
- `POST /finances/sellers/{id}/payouts`

## Critérios de aceitação

- Vendedor vê o valor de "Pedidos em Rota" no saldo bloqueado, e, magicamente no dia em que a transportadora confirma a entrega, o valor despenca para Saldo Disponível.
- Solicitar um saque envia o Pix para a conta do lojista e adiciona o lançamento no extrato com ID rastreável da liquidação.
- Se a conta bancária do vendedor estiver errada e o banco rejeitar, o sistema devolve o saldo para a conta disponível da plataforma cobrando taxa administrativa configurável, gerando notificação.

## Métricas

- Latência do Repasse (Tempo entre PayoutRequest e dinheiro na conta).
- Erros de conciliação diários (diferença entre sistema e banco).
- Volume de capital retido por estornos/saldo devedor de Sellers.

## Dependências

Payment & Split processou e proveu as métricas exatas, originando o crédito. Fulfillment informou que o risco se extinguiu, destrancando o dinheiro. KYB homologou a conta de destino.