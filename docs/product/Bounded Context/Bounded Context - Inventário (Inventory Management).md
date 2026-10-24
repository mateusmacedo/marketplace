**Bounded Context - Inventário (Inventory Management)**

**1. Definição Descritiva Refinada**

* **Propósito Central:** Este Bounded Context é o responsável por rastrear e gerenciar a **quantidade disponível** de cada item fisicamente vendável (identificado unicamente como um SKU - Stock Keeping Unit) que existe no Catálogo. Sua principal função é fornecer uma visão precisa da disponibilidade de estoque em tempo (quase) real, controlar reservas para evitar vendas de itens inexistentes (overselling) e processar todas as movimentações que alteram a quantidade física ou disponível.
* **Responsabilidades Principais:**
  + **Rastreamento de Níveis de Estoque:** Manter a contagem atualizada de unidades fisicamente disponíveis para cada SKU (TenantId + SkuId). Pode, opcionalmente, suportar o rastreamento por múltiplos locais (armazéns, lojas físicas, estoque de diferentes vendedores dentro do tenant).
  + **Gestão de Reservas:** Implementar um mecanismo para "segurar" temporariamente unidades de um SKU durante o processo de compra do cliente (tipicamente a partir do momento em que o pedido é finalizado ou até mesmo quando adicionado ao carrinho, dependendo da estratégia). Isso previne que outro cliente compre o mesmo item enquanto o primeiro finaliza o pagamento.
  + **Processamento de Ajustes de Estoque:** Registrar todas as alterações na quantidade de estoque, sejam elas incrementais (recebimento de mercadorias de fornecedores, devoluções de clientes aceitas) ou decrementais (confirmação de vendas após pagamento, baixas por perdas, danos, obsolescência, ajustes de contagem física). Cada ajuste deve ter um motivo claro.
  + **Alertas de Nível Baixo:** Monitorar a quantidade disponível em relação a um limite mínimo configurável por SKU e disparar eventos (NivelBaixoEstoqueAtingido) quando esse limite for cruzado, para sinalizar a necessidade de reposição ou outras ações administrativas.
  + **Consultas de Disponibilidade:** Fornecer informações rápidas e eficientes sobre a disponibilidade de um SKU para outros BCs (como Catálogo/Busca para exibir na loja) e para a interface do usuário.
* **Limites e Exclusões:**
  + **Não Define o SKU:** Ele consome informações do BC Catálogo para saber *quais* SKUs existem e precisam ser rastreados, mas não define seus atributos descritivos (nome, cor, tamanho, etc.).
  + **Não Define Preço:** O custo ou preço de venda do SKU é gerenciado pelo BC de Precificação.
  + **Não Gerencia Pedidos:** Ele *reage* a eventos de Pedidos (Pedido Finalizado, Pedido Cancelado) e Pagamentos (Pagamento Aprovado) para executar reservas e débitos, mas não gerencia o ciclo de vida do pedido em si.
  + **Não é Logística Física:** Não trata do processo físico de recebimento no armazém, endereçamento, picking, packing ou envio (essas podem ser responsabilidades de um BC de Fulfillment/WMS), apenas registra o resultado final dessas operações como um ajuste de quantidade.
* **Importância:** Absolutamente crítico para a operação confiável do marketplace. Evitar overselling é fundamental para a satisfação do cliente. Informações precisas de disponibilidade influenciam a decisão de compra e são necessárias para processos internos de reposição e planejamento.

**2. Event Storming (Focado em Inventory)**

* **Eventos Externos Chave (Consumidos):**
  + SKUDefinido / ProdutoPublicado 🟧 (Do Catálogo): Sinaliza que um novo SKU precisa ter seu estoque rastreado.
  + PedidoFinalizado 🟧 (Do Pedido): Principal gatilho para iniciar a reserva de estoque para os itens do pedido.
  + PagamentoAprovado 🟧 (Do Pagamento): Confirmação para converter a reserva em um débito definitivo do estoque.
  + PedidoCancelado 🟧 / ItemPedidoDevolvidoAceito 🟧 (Do Pedido/Devolução): Gatilho para liberar reservas ou reverter débitos (ajuste positivo).
* **Fluxo Interno Principal (Exemplo Venda):**
  + *(Listener consome SKUDefinido)* -> InicializarEstoqueSKU 🟦 (Comando interno para Aggregate EstoqueSKU 🟨)
  + EstoqueSKUInicializado 🟧 (Publicado, Qtd=0) -> Persistido.
  + AjustarEstoqueFisico 🟦 (Admin/Vendedor 🧍 via UI/API, +50 unidades, motivo "Entrada NF 123") -> Comando para EstoqueSKU 🟨
  + EstoqueAjustado 🟧 (Publicado, Qtd Nova=50, Ajuste=+50) -> Persistido.
  + *(Listener consome PedidoFinalizado com SKU X, Qtd 1)* -> ReservarEstoque 🟦 (Comando para EstoqueSKU 🟨 do SKU X) -> Contém pedidoId, quantidade: 1
  + EstoqueReservado 🟧 (Publicado, Qtd Disponível=49, Qtd Reservada=1) -> Persistido (Aggregate atualizado).
  + *(Listener consome PagamentoAprovado para o pedidoId)* -> ConfirmarDebitoEstoque 🟦 (Comando para EstoqueSKU 🟨 do SKU X) -> Contém pedidoId, quantidade: 1
  + DebitoEstoqueConfirmado 🟧 (Publicado) -> Estado interno da reserva é alterado/removido.
  + EstoqueAjustado 🟧 (Publicado, Qtd Nova=49, Qtd Reservada=0, Ajuste=-1, motivo "Venda Pedido Y") -> Persistido (Aggregate atualizado).
  + *(Verificação Interna: Qtd Nova < Limite Mínimo?)* Se sim -> NivelBaixoEstoqueAtingido 🟧 (Publicado).
* **Fluxo Interno (Exemplo Cancelamento Após Reserva):**
  + *(Listener consome PedidoCancelado para pedidoId que tinha reserva para SKU X)* -> LiberarReservaEstoque 🟦 (Comando para EstoqueSKU 🟨 do SKU X) -> Contém pedidoId, quantidade: 1
  + ReservaEstoqueLiberada 🟧 (Publicado)
  + EstoqueAjustado 🟧 (Publicado, Qtd Disponível=50, Qtd Reservada=0, Ajuste=0, motivo "Cancelamento Reserva Pedido Y") -> Persistido (Aggregate atualizado, disponibilidade volta ao normal).
* **Outros Fluxos:** Ajustes manuais (contagem física), definição de limite mínimo, etc., seguem padrões similares: Comando -> Processamento no Aggregate -> Evento(s).

**3. Event Modelling (Exemplos de Eventos Chave)**

* EstoqueAjustado (v1)
  {
   "eventId": "uuid-evento-inv-1",
   "eventType": "EstoqueAjustado",
   "eventVersion": "1",
   "timestamp": "2025-04-24T23:40:00Z",
   "aggregateId": "sku-uuid-xyz", // SkuId
   "correlationId": "uuid-correlacao-nf123",
   "payload": {
   "tenantId": "tenant-uuid-123",
   "skuId": "sku-uuid-xyz",
   "produtoId": "produto-uuid-abc", // Informativo
   "quantidadeAnterior": 0,
   "quantidadeNova": 50,
   "ajuste": 50,
   "motivoAjuste": "RECEBIMENTO\_FORNECEDOR", // Enum ou String
   "referenciaMotivo": "NF-12345", // ID da Nota Fiscal, Pedido, etc.
   "operadorId": "user-uuid-admin", // Quem fez o ajuste
   "timestampAjuste": "2025-04-24T23:39:55Z"
   }
  }
* EstoqueReservado (v1)
  {
   "eventId": "uuid-evento-inv-2",
   "eventType": "EstoqueReservado",
   "eventVersion": "1",
   "timestamp": "2025-04-24T23:41:00Z",
   "aggregateId": "sku-uuid-xyz", // SkuId
   "correlationId": "uuid-correlacao-pedido-abc",
   "payload": {
   "tenantId": "tenant-uuid-123",
   "skuId": "sku-uuid-xyz",
   "referenciaReserva": "pedido-uuid-abc", // ID do Pedido (ou Carrinho)
   "quantidadeReservada": 1,
   "quantidadeDisponivelAposReserva": 49,
   "timestampReserva": "2025-04-24T23:40:58Z",
   "expiracaoReservaTimestamp": "2025-04-25T00:10:58Z" // Opcional, se reservas expiram
   }
  }
* NivelBaixoEstoqueAtingido (v1)
  {
   "eventId": "uuid-evento-inv-3",
   "eventType": "NivelBaixoEstoqueAtingido",
   "eventVersion": "1",
   "timestamp": "2025-04-24T23:42:00Z",
   "aggregateId": "sku-uuid-jkl", // Outro SkuId
   "correlationId": "uuid-correlacao-venda-def",
   "payload": {
   "tenantId": "tenant-uuid-123",
   "skuId": "sku-uuid-jkl",
   "produtoId": "produto-uuid-ghi",
   "quantidadeAtual": 9,
   "limiteMinimoConfigurado": 10
   }
  }

**4. Modelagem dos Componentes (DDD Tático)**

* **Aggregates:**
  + **EstoqueSKU Aggregate:** (O coração do BC, garantindo a consistência da quantidade por SKU)
    - **Root:** EstoqueSKU (Entity)
    - **ID:** SkuId (Value Object - o mesmo ID usado no BC Catálogo, estabelecendo a ligação).
    - **Atributos:**
      * TenantId (Value Object)
      * ProdutoId (Value Object - informativo, ajuda na contextualização)
      * QuantidadeDisponivel (long/int - a quantidade que pode ser vendida *agora*)
      * QuantidadeReservadaTotal (long/int - soma de todas as reservas ativas)
      * LimiteMinimoAlerta (long/int - limite para o evento NivelBaixoEstoqueAtingido)
      * IsEnabled (boolean - indica se o estoque deste SKU deve ser rastreado, pode ser desabilitado se o produto/SKU for desativado no catálogo).
      * Version (long - para controle de concorrência otimista).
    - **Coleções (Opcional/Alternativa para Reservas):** Em vez de QuantidadeReservadaTotal, poderia ter Map<ReferenciaReserva, Quantidade> para reservas ativas. Isso permite liberar reservas específicas.
    - **Invariantes:**
      * QuantidadeDisponivel >= 0.
      * Uma reserva só pode ser feita se QuantidadeDisponivel >= quantidadeSolicitada.
      * Um débito (ajuste negativo por venda) só pode ocorrer se houver reserva correspondente ou se QuantidadeDisponivel >= quantidadeSolicitada.
      * **Atomicidade:** As operações de verificar disponibilidade, diminuir quantidade e incrementar versão DEVEM ser atômicas para prevenir overselling (garantido pela transação do DB e pelo lock otimista).
    - **Comandos:** InicializarEstoqueSKU, HabilitarRastreamentoEstoque, DesabilitarRastreamentoEstoque, AjustarEstoque (com quantidade +/- e motivo), ReservarEstoque (com ReferenciaReserva, Quantidade), ConfirmarDebitoVenda (usando ReferenciaReserva), LiberarReserva (usando ReferenciaReserva), DefinirLimiteMinimoAlerta.
    - **Eventos:** EstoqueSKUInicializado, RastreamentoEstoqueHabilitado, RastreamentoEstoqueDesabilitado, EstoqueAjustado, EstoqueReservado, DebitoVendaConfirmado, ReservaEstoqueLiberada, LimiteMinimoAlertaDefinido, NivelBaixoEstoqueAtingido.
* **Value Objects:** SkuId, TenantId, ProdutoId, Quantidade (int/long), MotivoAjuste (Enum: RECEBIMENTO, VENDA, DEVOLUCAO, PERDA, CONTAGEM\_FISICA, ...), ReferenciaMotivo (String, ex: PedidoId, NotaFiscalId), ReferenciaReserva (String, ex: PedidoId).
* **Entities:** EstoqueSKU.
* **CQRS:**
  + **Write Side:** O Aggregate EstoqueSKU processa os comandos. A persistência é crucialmente feita em **PostgreSQL** devido à necessidade de transações ACID fortes e controle de concorrência otimista (usando a Version). Uma tabela estoque\_sku (sku\_id PK, tenant\_id, produto\_id, quantidade\_disponivel, quantidade\_reservada\_total, limite\_minimo, is\_enabled, version) seria o centro. Outra tabela reservas\_estoque (reserva\_id PK, sku\_id, tenant\_id, referencia\_reserva, quantidade, timestamp\_expiracao) poderia ser usada se a gestão fina de reservas for necessária. Eventos são publicados no RabbitMQ após a transação do DB ser commitada com sucesso.
  + **Read Side (Projections):** Event handlers assíncronos consomem os eventos e atualizam as projeções otimizadas para leitura:
    - **ProjecaoDisponibilidadeSKU:** A projeção mais crítica para performance da loja. Contém { skuId, tenantId, produtoId, quantidadeDisponivel, statusDisponibilidade: ("DISPONIVEL" | "BAIXO\_ESTOQUE" | "INDISPONIVEL"), timestampAtualizacao }. Idealmente armazenada em **Redis** (cache K/V: chave disp:tenantId:skuId) para acesso de baixíssima latência pelas listagens e páginas de produto. Atualizada principalmente por EstoqueAjustado, EstoqueReservado, ReservaEstoqueLiberada.
    - **ProjecaoHistoricoEstoque:** Log de todas as movimentações (EstoqueAjustado) para fins de auditoria, relatórios de giro, etc. Pode ser uma tabela em **PostgreSQL** ou documentos em **MongoDB**.
    - **ProjecaoAlertaEstoqueBaixo:** Lista/Painel dos SKUs que estão abaixo do limite mínimo. Tabela em **PostgreSQL** ou coleção em **MongoDB**, populada pelo evento NivelBaixoEstoqueAtingido e limpa quando um EstoqueAjustado positivo resolve a situação.

**5. Sagas / Process Managers**

* **SagaReservaEstoquePedido:** (Altamente recomendada se o checkout envolve múltiplos itens ou fontes de estoque)
  + **Trigger:** Evento PedidoFinalizado.
  + **Orquestração:**
    1. Para cada item no pedido, envia Comando ReservarEstoque para o Aggregate EstoqueSKU correspondente.
    2. Aguarda pelos eventos EstoqueReservado ou FalhaReservaEstoque (se o aggregate rejeitar por falta de estoque).
    3. Se *todas* as reservas forem bem-sucedidas, publica ReservaEstoquePedidoCompleta (evento para o BC Pedido continuar o fluxo).
    4. Se *qualquer* reserva falhar, inicia a compensação: envia Comandos LiberarReserva para todos os SKUs que *foram* reservados com sucesso neste fluxo e publica FalhaReservaEstoquePedido (evento para o BC Pedido cancelar/informar o usuário).
  + **Estado:** Precisa manter o pedidoId, a lista de itens a reservar, e o status de cada reserva (pendente, sucesso, falha). Persistido em DB.
* **ProcessManagerExpiracaoReserva:** (Necessário se as reservas tiverem tempo de vida limitado)
  + **Trigger:** Scheduler (executa periodicamente, ex: a cada minuto) ou eventos agendados (usando expiracaoReservaTimestamp).
  + **Lógica:** Busca por reservas expiradas (na tabela reservas\_estoque ou no estado da saga, se aplicável). Para cada reserva expirada, envia Comando LiberarReserva para o Aggregate EstoqueSKU.

**6. Considerações Adicionais**

* **Concorrência e Atomicidade:** Este é o desafio nº 1 do inventário. O uso de versionamento no Aggregate e transações ACID no PostgreSQL são essenciais. Testes de carga e stress são fundamentais para validar o comportamento sob concorrência.
* **Precisão vs. Performance:** Leituras de disponibilidade precisam ser rápidas (Redis), mas podem ter uma pequena defasagem em relação ao write-side (consistência eventual). O write-side precisa ser consistente.
* **Estratégia de Reserva:** Decidir *quando* reservar (carrinho vs. checkout) e se reservas *expiram* tem grandes implicações. Reservar no checkout é mais comum para não prender estoque desnecessariamente.
* **Integração com Catálogo:** Consumir SKUDefinido/ProdutoPublicado para iniciar o rastreamento e ProdutoDesativado/SKURemovido para parar o rastreamento é vital.

**Próxima Tarefa:**

Cobrimos o Bounded Context de Inventário. O próximo passo lógico em nossa lista é detalhar o **Contexto de Precificação (Pricing & Promotions)**.

Podemos prosseguir com a análise detalhada do Contexto de Precificação?