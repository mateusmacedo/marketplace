**Bounded Context - Pedidos (Order Management)**

**1. Definição Descritiva Refinada**

* **Propósito Central:** Este Bounded Context (BC) é o responsável por gerenciar o **ciclo de vida completo** de um pedido de cliente dentro do marketplace. Ele orquestra o fluxo desde a sua criação – que representa a formalização da intenção de compra originada no Carrinho – até a sua conclusão final, seja ela a entrega bem-sucedida, um cancelamento ou uma devolução. Atua como o registro oficial e durável da transação comercial.
* **Responsabilidades Principais:**
  + **Criação do Pedido:** Capturar os dados finais e "congelados" do carrinho (via evento CheckoutIniciado) e criar uma representação persistente e, em grande parte, imutável do pedido, incluindo itens, quantidades, preços finais, dados do cliente, endereços de entrega/cobrança e método de pagamento/envio selecionado.
  + **Orquestração do Fluxo Pós-Criação:** Coordenar as etapas subsequentes do processo, agindo como um maestro. Ele **dispara ações** em outros BCs especializados, como solicitar o processamento do pagamento (ao BC de Pagamento), solicitar a reserva ou débito do estoque (ao BC de Inventário) e solicitar o início do processo de envio (ao BC de Entrega). É crucial entender que ele *não executa* essas tarefas, mas *inicia* e *reage* aos resultados.
  + **Gerenciamento de Status do Pedido:** Rastrear e atualizar o estado do pedido ao longo de todo o seu ciclo de vida (ex: Pendente Pagamento, Pagamento Aprovado, Aguardando Envio, Em Separação, Enviado, Em Trânsito, Entregue, Cancelado, Devolvido Parcialmente, etc.). Essas atualizações ocorrem em resposta a eventos recebidos de outros BCs ou ações internas.
  + **Persistência dos Dados do Pedido:** Armazenar de forma segura e durável todos os detalhes relevantes do pedido, garantindo que o "snapshot" da compra (o que foi comprado, por quanto, por quem, para onde) esteja disponível para consultas futuras, auditoria, histórico do cliente e processos administrativos.
  + **Gerenciamento de Solicitações (Cancelamento/Devolução):** Lidar com o início dos fluxos de cancelamento (antes do envio) ou devolução (após o recebimento), orquestrando as ações compensatórias necessárias (como solicitar reembolso ao BC de Pagamento e ajuste de estoque ao BC de Inventário).
  + **Fornecimento de Informações do Pedido:** Disponibilizar dados sobre os pedidos para diferentes consumidores: clientes (em sua área "Meus Pedidos"), administradores do tenant/vendedores (para gerenciamento operacional) e outros BCs que possam precisar de informações contextuais sobre um pedido específico.
* **Limites e Exclusões:**
  + **Não Processa Pagamentos:** Delega 100% da interação com gateways e processamento financeiro ao BC de Pagamento. Apenas envia um comando "Pague este pedido" e ouve "Pagamento OK" ou "Pagamento Falhou".
  + **Não Gerencia Estoque:** Delega a reserva e o débito de itens ao BC de Inventário. Envia comandos "Reserve/Debite estoque para este pedido" e ouve a confirmação ou falha.
  + **Não Realiza o Envio Físico:** Delega a logística de picking, packing, expedição e rastreamento ao BC de Entrega/Fulfillment. Envia um comando "Envie este pedido" e ouve os eventos de status do envio.
  + **Não Define Produtos ou Preços:** Trabalha com os dados de produtos e preços que foram "congelados" no momento da criação do pedido, originados do Carrinho (que por sua vez obteve da Precificação).
  + **Não Gerencia o Carrinho:** Sua vida começa quando o carrinho termina (evento CheckoutIniciado).
* **Importância:** É um BC **central e orquestrador** na arquitetura. Conecta e coordena as atividades de vários outros BCs críticos (Pagamento, Inventário, Entrega, Notificações). Representa o contrato de compra firmado com o cliente e é a entidade transacional primária do ponto de vista do negócio e do cliente. A confiabilidade, consistência e correta orquestração dos fluxos neste BC são absolutamente vitais para a operação do marketplace.

**2. Event Storming (Focado em Order Management)**

* **Eventos Externos Chave (Consumidos):**
  + CheckoutIniciado 🟧 (Do Carrinho): **GATILHO PRINCIPAL**. Contém todos os dados para criar o pedido.
  + PagamentoAprovado / PagamentoFalhou 🟧 (Do Pagamento): Informa o resultado da tentativa de pagamento.
  + EstoqueReservadoParaPedido / FalhaReservaEstoquePedido 🟧 (Do Inventário/Saga): Confirma se o estoque necessário está disponível e reservado (se aplicável).
  + DebitoEstoqueConfirmado 🟧 (Do Inventário): Confirma que o estoque foi baixado após a venda.
  + PedidoProntoParaEnvio 🟧 (Da Entrega/Fulfillment - talvez): Indica que a separação foi concluída.
  + PedidoEnviado / PedidoEntregue 🟧 (Da Entrega/Fulfillment): Atualiza o status logístico.
  + SolicitacaoCancelamentoAprovada 🟧 (De Suporte/Admin): Inicia o fluxo de cancelamento formal.
  + DevolucaoItemAutorizada / ItemDevolvidoConferido 🟧 (De Devolução/Logística Reversa): Informa progresso do fluxo de devolução.
* **Fluxo Interno Principal (Criação e Processamento Feliz Inicial):**
  + *(Listener consome CheckoutIniciado)* 🟧 -> Payload rico do carrinho.
  + CriarPedido 🟦 (Comando interno para Pedido Aggregate 🟨).
  + PedidoCriado 🟧 (Publicado) -> Estado inicial "PENDENTE\_PAGAMENTO", contém snapshot completo. Persistido. **EVENTO FUNDAMENTAL**.
  + *(Política/Saga: "Iniciar Processamento Pós-Criação")* ⚙️
    - Dispara SolicitarProcessamentoPagamento 🟦 (Comando para BC Pagamento).
    - (Estratégia A: Reservar Antes) Dispara ReservarEstoqueParaPedido 🟦 (Comando para BC Inventário/Saga de Reserva).
  + *(Listener consome PagamentoAprovado)* 🟧
  + MarcarPedidoComoPago 🟦 (Comando interno para Pedido Aggregate 🟨).
  + PedidoPago 🟧 (Publicado) -> Status "PAGO" ou "AGUARDANDO\_ENVIO". Persistido.
  + *(Política/Saga: "Liberar Próximas Etapas Pós-Pagamento")* ⚙️
    - (Estratégia B: Debitar Após Pgto) Dispara ConfirmarDebitoEstoquePedido 🟦 (Comando para BC Inventário).
    - Dispara SolicitarEnvioPedido 🟦 (Comando para BC Entrega/Fulfillment).
* **Fluxo Interno (Pagamento Falha):**
  + *(Listener consome PagamentoFalhou)* 🟧
  + RegistrarFalhaPagamentoPedido 🟦 (Comando interno para Pedido Aggregate 🟨).
  + PagamentoPedidoFalhou 🟧 (Publicado) -> Status "FALHA\_PAGAMENTO". Persistido.
  + *(Política/Saga: "Compensar Ações Iniciais")* ⚙️
    - (Se reserva foi feita) Dispara LiberarReservaEstoque 🟦 (Comando para BC Inventário).
* **Fluxo Interno (Pedido Enviado):**
  + *(Listener consome PedidoEnviado)* 🟧 -> Contém codigoRastreio, etc.
  + RegistrarEnvioPedido 🟦 (Comando interno para Pedido Aggregate 🟨).
  + PedidoEnviado 🟧 (Publicado) -> Status "ENVIADO", dados de rastreio armazenados. Persistido.
* **Fluxo Interno (Cancelamento Solicitado pelo Cliente Antes do Envio):**
  + *(Listener consome SolicitacaoCancelamentoAprovada)* 🟧
  + IniciarProcessoCancelamento 🟦 (Comando interno para Pedido Aggregate 🟨).
  + CancelamentoPedidoIniciado 🟧 (Publicado) -> Status "EM\_CANCELAMENTO". Persistido.
  + *(Política/Saga: "Orquestrar Cancelamento")* ⚙️
    - Dispara SolicitarReembolso 🟦 (Comando para BC Pagamento).
    - Dispara CancelarEnvioPendente 🟦 (Comando para BC Entrega, se já solicitado).
    - Dispara LiberarEstoque / ReverterDebito 🟦 (Comando para BC Inventário).
  + *(Listeners consomem ReembolsoConfirmado, EnvioCancelado, EstoqueAjustado)* 🟧
  + FinalizarCancelamentoPedido 🟦 (Comando interno para Pedido Aggregate 🟨).
  + PedidoCancelado 🟧 (Publicado) -> Status "CANCELADO". Persistido.

**3. Event Modelling (Exemplos de Eventos Chave)**

* PedidoCriado (v1)
  {
   "eventId": "uuid-evt-ord-1",
   "eventType": "PedidoCriado",
   "eventVersion": "1",
   "timestamp": "2025-04-25T00:10:00Z",
   "aggregateId": "pedido-uuid-abc", // PedidoId
   "correlationId": "cart-uuid-789", // ID do carrinho que originou
   "payload": {
   "pedidoId": "pedido-uuid-abc",
   "tenantId": "tenant-uuid-123",
   "userId": "user-456",
   "statusPedido": "PENDENTE\_PAGAMENTO",
   "itensSnapshot": [
   { "skuId": "sku-book-123", "produtoId": "prod-book-abc", "nomeSnapshot": "Livro DDD Essencial", "quantidade": 2, "precoUnitarioFinal": 71.91, "moeda": "BRL", "subtotalItem": 143.82 }
   ],
   "totaisSnapshot": {
   "subtotalItens": 143.82,
   "descontoPromocional": 0.00,
   "descontoCupom": 10.00,
   "valorFrete": 15.00, // Exemplo, pode vir depois
   "valorTotal": 148.82,
   "moeda": "BRL"
   },
   "enderecoEntrega": { "rua": "...", "numero": "...", "cidade": "...", "estado": "...", "cep": "..." },
   "enderecoCobranca": { "...same..." },
   "dadosPagamentoEscolhidoSnapshot": { "metodo": "CARTAO\_CREDITO", "parcelas": 1, "ultimos4Digitos": "1234" }, // Representação segura
   "timestampCriacao": "2025-04-25T00:09:58Z"
   }
  }
* PedidoPago (v1): { ..., "aggregateId": "pedido-uuid-abc", "payload": { "pedidoId": "...", "tenantId": "...", "statusPedidoAnterior": "PENDENTE\_PAGAMENTO", "statusPedidoNovo": "AGUARDANDO\_ENVIO", "timestampPagamento": "..." } }
* PedidoEnviado (v1): { ..., "aggregateId": "pedido-uuid-abc", "payload": { "pedidoId": "...", "tenantId": "...", "statusPedidoAnterior": "AGUARDANDO\_ENVIO", "statusPedidoNovo": "ENVIADO", "dadosRastreio": { "codigo": "PX...", "transportadora": "...", "url": "..." }, "timestampEnvio": "..." } }
* PedidoCancelado (v1): { ..., "aggregateId": "pedido-uuid-abc", "payload": { "pedidoId": "...", "tenantId": "...", "statusPedidoAnterior": "EM\_CANCELAMENTO", "statusPedidoNovo": "CANCELADO", "motivo": "...", "timestampCancelamento": "..." } }

**4. Modelagem dos Componentes (DDD Tático)**

* **Aggregates:**
  + **Pedido Aggregate:** (O agregado dominante, representa a unidade transacional)
    - **Root:** Pedido (Entity)
    - **ID:** PedidoId (VO, UUID)
    - **Atributos:** TenantId (VO), UserId (VO), StatusPedido (Enum VO - com máquina de estados bem definida), TimestampCriacao, TimestampUltimaAtualizacao, Version (long - para concorrência otimista).
    - **Value Objects (Snapshots Imutáveis):**
      * Endereco (para Entrega e Cobrança).
      * DadosPagamentoSnapshot.
      * DadosEnvioSnapshot.
      * List<ItemPedido> (VOs - a lista de itens comprados).
        + ItemPedido (VO): SkuId, ProdutoId, NomeSnapshot, ImagemUrlSnapshot, Quantidade, PrecoUnitarioFinal (Money VO), SubtotalItem (Money VO).
      * TotaisPedidoSnapshot (VO: SubtotalItens, Descontos, Frete, Total, Moeda).
    - **Atributos (Dados de Rastreio/Status):** DadosRastreio (VO: Codigo, Transportadora, Url), TimestampPagamento, TimestampEnvio, TimestampEntrega, TimestampCancelamento.
    - **Invariantes:**
      * Transições de StatusPedido devem seguir regras da máquina de estados.
      * Os snapshots (Itens, Endereços, Totais) são imutáveis após a criação do pedido.
      * Um pedido só pode ser marcado como enviado se estiver pago (ou regra de negócio específica).
      * Um pedido só pode ser cancelado sob certas condições de status.
    - **Comandos (geralmente internos, acionados por eventos/sagas):** ConfirmarPagamentoEfetuado, RegistrarFalhaPagamento, RegistrarEnvio, RegistrarEntrega, IniciarCancelamentoProcesso, ConfirmarCancelamentoEfetuado, AssociarDadosRastreio. (O comando CriarPedido seria chamado pela lógica que consome CheckoutIniciado).
    - **Eventos:** PedidoCriado, PedidoPago, PagamentoPedidoFalhou, PedidoEnviado, PedidoEntregue, CancelamentoPedidoIniciado, PedidoCancelado, DadosRastreioAtualizados.
* **Value Objects:** PedidoId, TenantId, UserId, StatusPedido, Endereco, Money, ItemPedido, SkuId, ProdutoId, Quantidade, DadosPagamentoSnapshot, DadosEnvioSnapshot, TotaisPedidoSnapshot, DadosRastreio.
* **Entities:** Pedido.
* **CQRS:**
  + **Write Side:** O Aggregate Pedido é a autoridade sobre o estado do pedido. **PostgreSQL** é a escolha mais robusta e comum para persistir o Aggregate Pedido, oferecendo durabilidade ACID, capacidade de consulta relacional e suporte a JSONB para os VOs de snapshot. O controle de concorrência otimista (Version) é importante. Eventos são publicados no RabbitMQ após commit da transação.
  + **Read Side (Projections):** Essenciais para consultas eficientes sem sobrecarregar o Aggregate.
    - **ProjecaoListaPedidosUsuario (PostgreSQL/MongoDB):** Tabela/Coleção denormalizada com dados chave (pedidoId, data, status, valorTotal, talvez primeiroItemNome/Imagem) otimizada para a tela "Meus Pedidos". Indexada por userId e tenantId. Atualizada pelos principais eventos de status do pedido.
    - **ProjecaoDetalhePedido (PostgreSQL/MongoDB):** Armazena a visão completa e atualizada de um pedido específico, combinando dados do snapshot inicial com status e dados de rastreio mais recentes. Usada pela tela de detalhes do pedido e por UIs administrativas. Atualizada por todos os eventos relevantes do pedido.
    - **ProjecaoPedidosAdmin (PostgreSQL/Elasticsearch):** Visão para administradores, com capacidade avançada de busca e filtragem por status, datas, clientes, produtos, etc. Elasticsearch é ideal se a busca for complexa, caso contrário, PostgreSQL com bons índices.
    - **ProjecaoPedidosPendentes (PostgreSQL/MongoDB):** Listas otimizadas para processos operacionais (ex: Pedidos Pagos Aguardando Envio, Pedidos com Falha no Pagamento).

**5. Sagas / Process Managers:**

* **SagaProcessamentoPedido:** (Fundamental - orquestra o fluxo principal)
  + **Trigger:** PedidoCriado.
  + **Orquestração:** Coordena chamadas para Pagamento, Inventário (reserva/débito) e Envio, reagindo aos resultados (sucesso/falha) e atualizando o status do Pedido. Gerencia compensações em caso de falha.
  + **Estado:** Persistido em DB (ex: tabela sagas\_processamento\_pedido).
* **SagaCancelamentoPedido:**
  + **Trigger:** CancelamentoPedidoIniciado.
  + **Orquestração:** Coordena chamadas para Reembolso (Pagamento), Cancelamento de Envio (Entrega), Devolução de Estoque (Inventário). Confirma o cancelamento no Pedido após sucesso das compensações.
  + **Estado:** Persistido em DB.

**6. Considerações Adicionais**

* **Orquestração vs. Coreografia:** Este BC exemplifica bem a combinação: ele *orquestra* o fluxo principal via Sagas, mas também *reage* a eventos de outros BCs (coreografia) para atualizar seu estado.
* **Imutabilidade do Snapshot:** Garantir que os dados da compra (itens, preços, endereços) capturados no PedidoCriado não sejam alterados é crucial para a integridade da transação.
* **Máquina de Estados:** Modelar explicitamente o StatusPedido como uma máquina de estados finitos ajuda a gerenciar a complexidade das transições válidas.
* **Durabilidade:** Pedidos são registros de longo prazo e críticos para o negócio. A escolha do banco de dados (PostgreSQL) deve refletir essa necessidade de durabilidade e consistência.
* **Performance de Consultas:** As projeções de leitura (especialmente para listas de pedidos) devem ser otimizadas para performance, pois podem ser acessadas frequentemente.