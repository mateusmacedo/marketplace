**Bounded Context - Pagamentos (Payment Management)**

**1. Definição Descritiva Refinada**

* **Propósito Central:** Este Bounded Context (BC) é o especialista financeiro da plataforma, responsável por **gerenciar todas as interações com sistemas externos de processamento de pagamentos** (como gateways, adquirentes, bancos para Pix/Boletos) e por controlar o **ciclo de vida das transações financeiras** associadas aos pedidos dos clientes. Sua função principal é capturar os pagamentos de forma segura, processar reembolsos quando necessário e lidar com eventos relacionados a essas transações, abstraindo a complexidade dos diversos provedores e métodos de pagamento para o restante da plataforma.
* **Responsabilidades Principais:**
  + **Processamento de Pagamentos:** Receber a solicitação formal para processar um pagamento (originada pelo BC de Pedidos, geralmente via Saga), identificar o gateway de pagamento correto (configurado pelo Tenant), interagir com a API desse gateway de forma segura para submeter a transação (seja cartão de crédito, débito, boleto, Pix, voucher, etc.) e registrar o resultado oficial (Aprovado, Recusado, Pendente, Em Análise).
  + **Gerenciamento de Reembolsos (Estornos):** Receber a solicitação para reembolsar um pagamento anterior (originada pelo BC de Pedidos, geralmente em cenários de cancelamento ou devolução), interagir com o gateway correspondente para processar a transação de estorno/reembolso no meio de pagamento original e registrar o resultado (Confirmado, Falhou).
  + **Processamento de Notificações Assíncronas (Webhooks):** Implementar endpoints seguros para receber e processar notificações enviadas pelos gateways de pagamento (webhooks). Isso é crucial para métodos de pagamento assíncronos (confirmação de pagamento de boleto/Pix) ou para atualizações de status (disputas, conclusão de análise de fraude, etc.).
  + **Abstração de Provedores de Pagamento:** Criar uma camada de abstração (utilizando padrões como *Adapter* ou *Strategy*) que ofereça uma interface interna consistente para o restante do sistema, independentemente do gateway de pagamento específico que está sendo utilizado por um tenant. Isso facilita a integração de novos gateways e a manutenção.
  + **Gerenciamento de Disputas (Chargebacks):** Registrar e, potencialmente, acompanhar o ciclo de vida de contestações (chargebacks) iniciadas pelos portadores de cartão, recebendo notificações do gateway sobre abertura, solicitação de documentos e resolução da disputa.
  + **Gestão Segura de Dados de Pagamento (Tokenização):** Orquestrar a comunicação com o gateway para tokenizar dados sensíveis (como números de cartão de crédito) no momento da entrada, garantindo que a plataforma armazene apenas tokens não sensíveis, que podem ser usados (com consentimento e segurança) para cobranças futuras ou recorrentes.
* **Limites e Exclusões:**
  + **Não Define Valor ou Itens:** Recebe o valor final a ser cobrado e a referência do pedido do BC de Pedidos. Não tem conhecimento dos itens ou da composição do preço.
  + **Não Gerencia o Pedido em Si:** Atua *sobre* um pedido existente para processar seu pagamento e *informa* o resultado (via eventos) para que o BC de Pedidos possa atualizar o status do pedido.
  + **Não Define Produtos, Preços ou Estoque.**
  + **Não é Faturamento/Fiscal:** Não gera notas fiscais ou lida com a complexidade tributária (isso seria um BC Fiscal/Faturamento separado). O foco é estritamente na transação financeira.
* **Importância:** Crítico para a receita do negócio. Qualquer falha ou ineficiência resulta diretamente em perda de vendas ou problemas financeiros. A **segurança** é a principal preocupação não funcional aqui (aderência a padrões como PCI DSS é muitas vezes necessária). A flexibilidade para suportar múltiplos métodos e gateways de pagamento é um diferencial competitivo importante para a plataforma e seus tenants.

**2. Event Storming (Focado em Payment Management)**

* **Eventos Externos Chave (Consumidos - Comandos/Gatilhos):**
  + SolicitarProcessamentoPagamento 🟦 (Comando da Saga/BC Pedidos): **GATILHO PRIMÁRIO**. Contém pedidoId, valor, moeda, dadosPagamentoSeguros (ex: token do cartão, dados para boleto/Pix), gatewayPreferidoConfigTenant.
  + SolicitarReembolso 🟦 (Comando da Saga/BC Pedidos): Gatilho para reembolso. Contém pedidoId, pagamentoOriginalId, valorReembolso.
  + WebhookNotificacaoGateway 🟧 (Evento representando uma chamada externa do gateway, ex: WebhookBoletoPago, WebhookPixConfirmado, WebhookChargebackAberto).
* **Fluxo Interno Principal (Pagamento Cartão Aprovado):**
  + *(Handler consome SolicitarProcessamentoPagamento)*
  + IniciarPagamento 🟦 (Comando interno para Pagamento Aggregate 🟨). -> Cria Pagamento status 'PENDENTE'.
  + PagamentoIniciado 🟧 (Publicado). Persistido.
  + *(Política/Adapter: "Processar via Gateway X")* ⚙️
    1. Identifica Gateway X (via config Tenant).
    2. Obtém credenciais seguras (via ref. Tenant Config -> Vault).
    3. Formata e envia requisição para API do Gateway X ⬜.
  + *(Resposta Gateway X: Sucesso, Pagamento Aprovado)*
  + RegistrarAprovacaoPagamento 🟦 (Comando interno para Pagamento Aggregate 🟨). -> Atualiza status, armazena ID transação gateway.
  + PagamentoAprovado 🟧 (Publicado). Persistido. **EVENTO CONSUMIDO PELO BC PEDIDOS**.
* **Fluxo Interno (Pagamento Boleto e Confirmação via Webhook):**
  + *(Handler consome SolicitarProcessamentoPagamento com método Boleto)*
  + IniciarPagamento 🟦 -> PagamentoIniciado 🟧.
  + *(Política/Adapter: "Gerar Boleto via Gateway Y")* ⚙️ -> Chama API Gateway Y ⬜.
  + *(Resposta Gateway Y: Boleto Gerado)*
  + RegistrarDadosBoleto 🟦 (Comando interno para Pagamento Aggregate 🟨). -> Armazena linha digitável, URL, vencimento, status 'AGUARDANDO\_PAGAMENTO'.
  + BoletoGerado 🟧 (Publicado). Persistido. (Pode ser usado para notificar cliente).
  + *(... Dias depois ...)*
  + *(Controller recebe Webhook de Confirmação de Boleto do Gateway Y)* -> Valida autenticidade -> Publica evento interno WebhookBoletoPagoRecebido 🟧.
  + *(Listener consome WebhookBoletoPagoRecebido)* -> Contém dados para identificar o Pagamento.
  + ProcessarConfirmacaoExternaPagamento 🟦 (Comando interno para Pagamento Aggregate 🟨).
  + RegistrarAprovacaoPagamento 🟦 (Comando interno).
  + PagamentoAprovado 🟧 (Publicado).
* **Fluxo Interno (Solicitação de Reembolso):**
  + *(Handler consome SolicitarReembolso)* 🟦
  + IniciarReembolso 🟦 (Comando interno para Reembolso Aggregate 🟨). -> Cria Reembolso status 'PENDENTE'.
  + ReembolsoIniciado 🟧 (Publicado). Persistido.
  + *(Política/Adapter: "Processar Reembolso via Gateway X")* ⚙️ -> Chama API Gateway X (usando ID da transação original) ⬜.
  + *(Resposta Gateway X: Reembolso OK)*
  + RegistrarConfirmacaoReembolso 🟦 (Comando interno para Reembolso Aggregate 🟨). -> Atualiza status para 'CONCLUIDO'.
  + ReembolsoConfirmado 🟧 (Publicado). Persistido. **EVENTO CONSUMIDO PELA SAGA DE CANCELAMENTO/DEVOLUÇÃO**.
  + *(Resposta Gateway X: Falha no Reembolso)*
  + RegistrarFalhaReembolso 🟦 (Comando interno).
  + ReembolsoFalhou 🟧 (Publicado).

**3. Event Modelling (Exemplos de Eventos Chave)**

* PagamentoIniciado (v1): { ..., "aggregateId": "pay-uuid-111", "payload": { "pagamentoId": "...", "pedidoId": "...", "tenantId": "...", "valor": 250.00, "moeda": "BRL", "metodo": "BOLETO", "status": "PENDENTE", ... } }
* PagamentoAprovado (v1): { ..., "aggregateId": "pay-uuid-111", "payload": { "pagamentoId": "...", "pedidoId": "...", "tenantId": "...", "statusAnterior": "PENDENTE", "statusNovo": "APROVADO", "gateway": "PagarMe", "transacaoGatewayId": "tr\_...", "timestampAprovacao": "..." } }
* PagamentoFalhou (v1): { ..., "aggregateId": "pay-uuid-222", "payload": { "pagamentoId": "...", "pedidoId": "...", "tenantId": "...", "statusAnterior": "PENDENTE", "statusNovo": "FALHOU", "gateway": "Stripe", "codigoErro": "...", "mensagemErro": "...", "timestampFalha": "..." } }
* BoletoGerado (v1): { ..., "aggregateId": "pay-uuid-111", "payload": { "pagamentoId": "...", "pedidoId": "...", "tenantId": "...", "statusNovo": "AGUARDANDO\_PAGAMENTO", "boleto": { "linhaDigitavel": "...", "url": "...", "dataVencimento": "..." }, ... } }
* ReembolsoIniciado (v1): { ..., "aggregateId": "refund-uuid-abc", "payload": { "reembolsoId": "...", "pagamentoOriginalId": "pay-uuid-111", "pedidoId": "...", "tenantId": "...", "valor": 250.00, "moeda": "BRL", "status": "PENDENTE", ... } }
* ReembolsoConfirmado (v1): { ..., "aggregateId": "refund-uuid-abc", "payload": { "reembolsoId": "...", "statusAnterior": "PENDENTE", "statusNovo": "CONCLUIDO", "transacaoReembolsoGatewayId": "re\_...", "timestampConfirmacao": "..." } }

**4. Modelagem dos Componentes (DDD Tático)**

* **Aggregates:**
  + **Pagamento Aggregate:** (Representa a unidade de tentativa de pagamento para um pedido)
    - **Root:** Pagamento (Entity)
    - **ID:** PagamentoId (VO, UUID)
    - **Atributos:** PedidoId (VO), TenantId (VO), Valor (Money VO), Moeda (VO), MetodoPagamento (Enum VO), StatusPagamento (Enum VO com máquina de estados: Pendente, Aprovado, Falhou, AguardandoPgto, Estornado, etc.), GatewayUtilizado (String/Enum), TransacaoGatewayId (String), MensagemUltimoErro (String), TimestampCriacao, TimestampUltimaAtualizacao, Version.
    - **Value Objects Internos:** DadosBoletoGerado (VO), DadosPixGerado (VO), DadosCartaoTokenizadoSnapshot (VO - NUNCA dados reais).
    - **Coleções:** List<TentativaTransacao> (VOs/Entities? Log interno de interações com gateway).
    - **Invariantes:** Status transita corretamente. Consistência entre método e dados (ex: DadosBoletoGerado só existe para método BOLETO).
    - **Comandos (internos):** RegistrarAprovacao, RegistrarFalha, RegistrarDadosPagamentoPendente (Boleto/Pix), MarcarComoEstornadoParcialmente, MarcarComoEstornadoTotalmente.
    - **Eventos:** PagamentoIniciado, PagamentoAprovado, PagamentoFalhou, BoletoGerado, PixGerado, PagamentoParcialmenteEstornado, PagamentoTotalmenteEstornado.
  + **Reembolso Aggregate:** (Representa uma operação de devolução de valor)
    - **Root:** Reembolso (Entity)
    - **ID:** ReembolsoId (VO, UUID)
    - **Atributos:** PagamentoOriginalId (VO), PedidoId (VO), TenantId (VO), Valor (Money VO), Moeda (VO), StatusReembolso (Enum VO: Pendente, Confirmado, Falhou), Motivo (String), GatewayUtilizado (String/Enum), TransacaoReembolsoGatewayId (String), MensagemUltimoErro (String), Version.
    - **Invariantes:** Status transita corretamente. Valor > 0. Ligação com Pagamento Original.
    - **Comandos (internos):** ConfirmarProcessamento, RegistrarFalhaProcessamento.
    - **Eventos:** ReembolsoIniciado, ReembolsoConfirmado, ReembolsoFalhou.
* **Value Objects:** PagamentoId, PedidoId, TenantId, Money, Moeda, MetodoPagamento, StatusPagamento, DadosBoletoGerado, DadosPixGerado, DadosCartaoTokenizadoSnapshot, ReembolsoId, StatusReembolso.
* **Entities:** Pagamento, Reembolso.
* **CQRS:**
  + **Write Side:** Aggregates Pagamento e Reembolso garantem a consistência do estado. **PostgreSQL** é recomendado para persistência devido à necessidade de ACID, auditoria e consultas relacionais (ex: buscar todos pagamentos/reembolsos de um pedido). Segurança é primordial: **NUNCA armazenar dados sensíveis de cartão**. Usar apenas tokens ou referências seguras. Eventos publicados no RabbitMQ.
  + **Read Side (Projections):**
    - **ProjecaoStatusPagamentoPedido (Redis/PostgreSQL):** Cache rápido (chave payment:tenantId:pedidoId -> status) ou tabela indexada para o BC Pedidos verificar rapidamente o status. Atualizada por PagamentoAprovado, PagamentoFalhou, etc.
    - **ProjecaoTransacaoFinanceira (PostgreSQL/MongoDB):** Visão detalhada de todas as transações (pagamentos e reembolsos) para auditoria, conciliação financeira e telas administrativas. Atualizada por todos os eventos de Pagamento e Reembolso.
    - **ProjecaoPagamentosPendentes (PostgreSQL/MongoDB):** Lista de pagamentos aguardando confirmação externa (Boleto, Pix) ou análise.
* **Componentes Chave:**
  + **Gateway Adapters:** Serviços/Componentes que implementam a interface com cada gateway (StripeAdapter, PagarmeAdapter, etc.). Responsáveis pela tradução, chamada de API e tratamento de resposta/webhooks. Essenciais para a abstração.
  + **Webhook Controller:** Endpoint seguro que recebe chamadas dos gateways, valida a autenticidade e publica eventos internos (WebhookBoletoPagoRecebido, etc.) para serem processados pelos Listeners/Handlers.

**5. Sagas / Process Managers:**

* **Participante:** Este BC é principalmente um *participante* em sagas orquestradas por outros BCs (Pedidos).
  + Na SagaProcessamentoPedido: Recebe SolicitarProcessamentoPagamento, executa a lógica e publica PagamentoAprovado ou PagamentoFalhou.
  + Na SagaCancelamentoPedido: Recebe SolicitarReembolso, executa a lógica e publica ReembolsoConfirmado ou ReembolsoFalhou.

**6. Considerações Adicionais**

* **Segurança:** Repetindo: **Fundamental**. PCI DSS, tokenização, NUNCA logar dados sensíveis, validar webhooks, proteger credenciais de API (via Vault/Secrets Manager referenciado pelo Tenant Config).
* **Abstração e Extensibilidade:** O design dos Adapters deve facilitar a adição de novos métodos ou gateways de pagamento no futuro sem impactar o núcleo do BC.
* **Idempotência:** Implementar mecanismos (chaves de idempotência nas chamadas de API, verificações nos aggregates) para garantir que uma mesma solicitação processada múltiplas vezes (ex: devido a retentativas de rede) não resulte em múltiplas cobranças ou reembolsos.
* **Tratamento de Erros:** Lidar de forma robusta com falhas de comunicação com o gateway, recusas de pagamento, erros no processamento de reembolso. Publicar eventos claros de falha.
* **Webhooks:** A validação da origem e integridade dos webhooks é crucial para evitar fraudes. Cada gateway tem seu mecanismo (ex: assinatura de requisição).
* **Monitoramento:** Acompanhar taxas de sucesso/falha por gateway/método, latência das APIs, e erros é vital para a operação.