**Bounded Context - Entrega (Fulfillment / Shipment)**

**1. Definição Descritiva Refinada**

* **Propósito Central:** Este Bounded Context (BC) é o especialista em logística da plataforma. Sua responsabilidade é gerenciar todo o processo físico e informacional que transforma um pedido confirmado e pago em um pacote entregue nas mãos do cliente. Ele orquestra desde a solicitação de preparação do envio até o rastreamento final, interagindo com transportadoras e, potencialmente, com sistemas internos de gerenciamento de armazém (WMS).
* **Responsabilidades Principais:**
  + **Recepção e Processamento da Ordem de Envio:** Receber a instrução para enviar um pedido (originada pelo BC de Pedidos), contendo os detalhes dos itens, o endereço de entrega validado e, possivelmente, o método de envio preferido pelo cliente.
  + **Cálculo e Cotação de Frete:** Interagir com serviços externos (APIs de transportadoras como Correios, Jadlog, Melhor Envio, Kangu, etc.) ou utilizar tabelas de frete internas/configuradas para determinar as opções de envio disponíveis (serviços, prazos estimados, custos) para um determinado pacote (peso, dimensões) e destino. Esta função pode ser exposta para ser consultada durante o checkout também.
  + **Geração de Etiquetas de Envio:** Integrar-se com as APIs das transportadoras selecionadas para gerar as etiquetas de postagem/envio necessárias, incluindo códigos de barras e informações de rastreamento iniciais.
  + **Orquestração da Preparação Física (Picking & Packing):** Sinalizar e coordenar o processo de separação dos itens corretos no estoque, embalagem adequada e afixação da etiqueta de envio. *Nota: Em operações complexas com múltiplos armazéns ou processos sofisticados, esta parte pode pertencer a um BC de Warehouse Management (WMS) dedicado, com o qual este BC de Fulfillment interagiria.* Para este escopo, assumimos que ele orquestra ou registra a conclusão desta etapa.
  + **Gestão da Expedição (Coleta/Postagem):** Registrar o momento em que o pacote está pronto para ser coletado pela transportadora ou quando foi efetivamente postado em uma agência.
  + **Rastreamento do Envio (Tracking):** Acompanhar ativamente o progresso do pacote através das atualizações de status fornecidas pela transportadora. Isso pode ocorrer via recebimento de webhooks da transportadora ou por consultas periódicas (polling) às APIs de rastreamento. Os status típicos incluem: Objeto Postado, Em Trânsito, Saiu para Entrega, Tentativa de Entrega Falhou, Entregue, Devolvido ao Remetente.
  + **Comunicação de Status:** Publicar eventos significativos sobre o progresso do envio (ex: PedidoEnviado, PedidoEntregue, FalhaNaEntrega) para que outros BCs (principalmente Pedidos e Notificações) possam reagir e manter o cliente informado.
* **Limites e Exclusões:**
  + **Não Gerencia o Pedido:** Opera com base em um pedido já existente e confirmado, recebendo a ordem de envio do BC de Pedidos.
  + **Não Processa Pagamentos:** Assume que o pedido já está pago.
  + **Não Gerencia Quantidade de Estoque:** Assume que o BC de Inventário já realizou a baixa dos itens. Pode precisar saber a *localização* do estoque se houver WMS.
  + **Não Define Produtos ou Preços.**
  + **Não é o SAC (Serviço de Atendimento ao Cliente):** Fornece os dados de rastreio, mas não trata diretamente reclamações sobre atrasos, avarias ou extravios (embora os eventos que publica possam *acionar* processos de suporte).
* **Importância:** É a "última milha" (literal e figurativamente) da experiência de compra para produtos físicos. A qualidade percebida do serviço (velocidade, custo do frete, comunicação sobre o rastreio, integridade do pacote na chegada) tem um impacto imenso na satisfação e fidelização do cliente. A eficiência logística também afeta diretamente os custos operacionais do marketplace/vendedor. A capacidade de integrar e oferecer múltiplas opções de transporte é um fator competitivo chave.

**2. Event Storming (Focado em Fulfillment/Shipment)**

* **Eventos Externos Chave (Consumidos - Comandos/Gatilhos):**
  + SolicitarEnvioPedido 🟦 (Comando da Saga/BC Pedidos): **GATILHO PRIMÁRIO**. Contém pedidoId, lista de itens (SKUs, qtds), enderecoEntrega, metodoEnvioEscolhido.
  + SolicitarCalculoFrete 🟦 (Comando do Carrinho/Checkout/Pedidos): Gatilho para cotação. Contém dados do pacote (peso/dimensões estimadas) e CEPs.
  + WebhookAtualizacaoRastreioTransportadora 🟧 (Evento interno via Controller/Webhook Handler): Notificação externa sobre mudança no status do envio.
* **Fluxo Interno Principal (Do Pedido à Entrega):**
  + *(Handler consome SolicitarEnvioPedido)* 🟦
  + IniciarProcessamentoEnvio 🟦 (Comando interno para Envio Aggregate 🟨). -> Cria Envio, status 'PENDENTE\_PREPARACAO'.
  + EnvioIniciado 🟧 (Publicado). Persistido.
  + *(Política: "Preparar Envio")* ⚙️
    - (Opcional) CalcularFreteDefinitivo 🟦 (Chama Serviço de Cálculo/Adapters Transportadora) ⬜.
    - FreteDefinitivoCalculado 🟩.
    - GerarEtiqueta 🟦 (Chama Adapter Transportadora X) ⬜.
  + *(API Transportadora retorna Etiqueta e Rastreio)*
  + RegistrarDadosEtiqueta 🟦 (Comando interno para Envio Aggregate 🟨). -> Atualiza status para 'AGUARDANDO\_PREPARACAO' ou 'AGUARDANDO\_COLETA', armazena rastreio, URL etiqueta.
  + EtiquetaEnvioGerada 🟧 (Publicado). Persistido. (Pode notificar WMS/Operação).
  + *(Processo Externo/WMS: Picking, Packing)* -> Dispara ConfirmarPreparacaoCompleta 🟦 (Comando para Envio Aggregate 🟨).
  + EnvioProntoParaColeta 🟧 (Publicado). Status "PRONTO\_PARA\_COLETA". Persistido.
  + *(Processo Externo: Coleta/Postagem)* -> Transportadora atualiza status.
  + *(Webhook Handler recebe atualização "Objeto Coletado")* -> Valida -> Publica evento interno EventoRastreioRecebido 🟧.
  + *(Listener consome EventoRastreioRecebido)*
  + AtualizarStatusRastreio 🟦 (Comando interno para Envio Aggregate 🟨). -> Atualiza último status, adiciona ao histórico de rastreio.
  + StatusEnvioAtualizado 🟧 (Publicado). Status "COLETADO". Persistido.
  + PedidoEnviado 🟧 (**Publicado para BC Pedidos**). -> Contém pedidoId, dados de rastreio.
  + *(...Webhook Handler recebe mais atualizações: "Em Trânsito", "Saiu para Entrega"...) -> AtualizarStatusRastreio 🟦 -> StatusEnvioAtualizado 🟧.*
  + *(Webhook Handler recebe atualização "Objeto Entregue")* -> Publica EventoRastreioRecebido 🟧 (Entregue).
  + *(Listener consome EventoRastreioRecebido)*
  + AtualizarStatusRastreio 🟦 (Comando interno).
  + StatusEnvioAtualizado 🟧 (Publicado). Status "ENTREGUE". Persistido.
  + PedidoEntregue 🟧 (**Publicado para BC Pedidos**). -> Contém pedidoId, timestampEntrega.
* **Fluxo Interno (Cotação de Frete):**
  + *(Handler consome SolicitarCalculoFrete)* 🟦 -> Dados: CEPs, peso, dimensões, valor.
  + *(Política: "Cotar Fretes")* ⚙️ -> Chama múltiplos Adapters (Correios, MelhorEnvio, etc.) ⬜.
  + *(Respostas das APIs agregadas)*
  + CompilarResultadosFrete 🟦 (Lógica interna).
  + CalculoFreteRealizado 🟧 (Evento publicado OU Resposta síncrona) -> Contém lista de { transportadora, servico, prazo, custo }.

**3. Event Modelling (Exemplos de Eventos Chave)**

* EnvioIniciado (v1): { ..., "aggregateId": "shp-uuid-123", "payload": { "envioId": "...", "pedidoId": "...", "tenantId": "...", "statusEnvio": "PENDENTE\_PREPARACAO", ... } }
* EtiquetaEnvioGerada (v1): { ..., "aggregateId": "shp-uuid-123", "payload": { "envioId": "...", "statusEnvio": "AGUARDANDO\_COLETA", "dadosEtiqueta": { "transportadora": "Jadlog", "servico": ".Package", "codigoRastreio": "123...", "urlPdfEtiqueta": "...", "custoFrete": 19.80, "moeda": "BRL" }, ... } }
* StatusEnvioAtualizado (v1): { ..., "aggregateId": "shp-uuid-123", "payload": { "envioId": "...", "statusEnvioAnterior": "EM\_TRANSITO", "statusEnvioNovo": "ENTREGUE", "eventoRastreio": { "timestamp": "...", "statusOriginal": "Objeto entregue ao destinatário", "localizacao": "Cidade Destino", "mensagem": "..." }, ... } }
* CalculoFreteRealizado (v1): { ..., "correlationId": "...", "payload": { "parametrosConsulta": { ... }, "opcoes": [ {"transportadora": "Correios", "servico": "SEDEX", "prazoEstimadoDias": 2, "custoTotal": 35.00, ...}, ... ] } }
* PedidoEnviado (v1) (Para BC Pedidos): { "pedidoId": "...", "tenantId": "...", "envioId": "shp-uuid-123", "dadosRastreio": { "codigo": "...", "transportadora": "...", "url": "..." }, "timestampEnvio": "..." }
* PedidoEntregue (v1) (Para BC Pedidos): { "pedidoId": "...", "tenantId": "...", "envioId": "shp-uuid-123", "timestampEntrega": "..." }

**4. Modelagem dos Componentes (DDD Tático)**

* **Aggregates:**
  + **Envio Aggregate:** (Representa a unidade logística para um pedido ou parte dele)
    - **Root:** Envio (Entity)
    - **ID:** EnvioId (VO, UUID). Fundamental ter a referência ao PedidoId.
    - **Atributos:** PedidoId (VO), TenantId (VO), StatusEnvio (Enum VO: PendentePreparacao, AguardandoColeta, Coletado, EmTransito, Falha, Entregue, Devolvido, Cancelado), EnderecoEntregaSnapshot (Endereco VO), DadosEtiqueta (VO: Transportadora, Servico, CodigoRastreio, UrlEtiqueta, Custo), PesoEstimadoKg (Decimal), DimensoesEstimadasCm (VO), TimestampUltimoStatus, Version.
    - **Coleções:** List<ItemEnvio> (VOs: skuId, qtd), List<EventoRastreioHistorico> (VOs: timestamp, status, local, msg).
    - **Invariantes:** Status progride de forma lógica. Dados da etiqueta são consistentes. Histórico de rastreio é append-only.
    - **Comandos (internos):** RegistrarDadosEtiquetaGerada, ConfirmarProntoColeta, RegistrarAtualizacaoRastreio, MarcarComoEntregue, MarcarComoFalhaEntrega, CancelarLogisticaEnvio.
    - **Eventos:** EnvioIniciado, EtiquetaEnvioGerada, EnvioProntoParaColeta, StatusEnvioAtualizado, EnvioCancelado. (+ Eventos para BC Pedidos).
* **Value Objects:** EnvioId, PedidoId, TenantId, StatusEnvio, Endereco, Money, ItemEnvio, SkuId, Quantidade, Dimensoes, Peso, DadosEtiqueta, EventoRastreioHistorico.
* **Entities:** Envio.
* **CQRS:**
  + **Write Side:** Aggregate Envio gerencia o ciclo de vida do envio. **PostgreSQL** é adequado para persistir o estado do envio e seu histórico de rastreio (que pode crescer, talvez em tabela separada ou JSONB). Transações ACID são importantes para registrar o estado. Eventos publicados no RabbitMQ.
  + **Read Side (Projections):**
    - **ProjecaoRastreioEnvio (PostgreSQL/MongoDB/Redis):** Usada pela UI do cliente/admin para mostrar o status atual e o histórico de rastreio. Pode ser uma cópia do aggregate em MongoDB ou uma tabela denormalizada em PostgreSQL. O status *atual* pode ser cacheado em Redis (K/V: envio:tenantId:pedidoId ou envio:tenantId:envioId) para acesso rápido. Atualizada por StatusEnvioAtualizado.
    - **ProjecaoEnviosEmAberto (PostgreSQL):** Para painéis operacionais, mostrando envios aguardando etiqueta, coleta, ou que estão em trânsito há muito tempo. Atualizada pelos eventos de status.
    - **ProjecaoCustosFrete (PostgreSQL):** Para relatórios financeiros, registrando o custo estimado/real do frete por envio. Atualizada por EtiquetaEnvioGerada.
* **Componentes Chave:**
  + **Serviço de Cálculo de Frete:** Orquestra chamadas aos adapters das transportadoras para obter cotações. Pode implementar regras de negócio (ex: qual transportadora oferecer com base no contrato, markup no frete). Pode ter cache.
  + **Adapters de Transportadora:** Componentes específicos para cada integração (CorreiosAdapter, JadlogAdapter, MelhorEnvioAdapter, etc.). Lidam com autenticação, formatação de requests/responses para cálculo de frete, geração de etiquetas e consulta de rastreio.
  + **Webhook Controller (Transportadoras):** Endpoint seguro para receber e validar notificações de rastreio, publicando eventos internos.

**5. Sagas / Process Managers:**

* **Participante:** Principalmente um participante em Sagas maiores.
  + Na SagaProcessamentoPedido: Recebe SolicitarEnvioPedido.
  + Na SagaCancelamentoPedido: Pode receber CancelarEnvioPendente.
* **(Potencial) SagaMonitoramentoAtivoRastreio:**
  + **Trigger:** Scheduler ou evento EtiquetaEnvioGerada.
  + **Lógica:** Para transportadoras que não oferecem webhooks confiáveis, esta saga poderia periodicamente consultar a API de rastreio e disparar comandos RegistrarAtualizacaoRastreio se houver novidades. Também poderia detectar envios parados/atrasados e gerar alertas.

**6. Considerações Adicionais**

* **Integrações Externas:** O ponto mais complexo e frágil. APIs de transportadoras são heterogêneas, podem mudar, ficar indisponíveis. Design defensivo (retentativas, circuit breakers, logging detalhado) e bons Adapters são cruciais.
* **Dados Físicos:** A precisão dos dados de peso e dimensões dos produtos (vindos do Catálogo) impacta diretamente o cálculo do frete e a validade da etiqueta.
* **Webhooks vs. Polling:** Webhooks são mais eficientes para atualizações de rastreio, mas exigem um endpoint público seguro e validação rigorosa. Polling é alternativa, mas consome mais recursos e tem maior latência.
* **Complexidade Logística:** A solução aqui descrita assume uma logística relativamente simples. Cenários com múltiplos centros de distribuição, dropshipping, cross-docking, agendamento de entrega, ou logística reversa complexa podem exigir BCs mais especializados (WMS, Logistics).
* **Comunicação com Cliente:** Os eventos PedidoEnviado e PedidoEntregue são gatilhos importantes para o BC de Notificações manter o cliente informado.