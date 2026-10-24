**1. Definição Descritiva Refinada**

* **Propósito Central:** Este Bounded Context (BC) é a autoridade para determinar o **preço final** que um cliente pagará por um item (SKU) ou um conjunto de itens (carrinho/pedido) em um dado momento e contexto. Ele encapsula toda a lógica de negócio relacionada a preços base, aplicação de descontos, gerenciamento de promoções complexas e validação/aplicação de cupons.
* **Responsabilidades Principais:**
  + **Gerenciamento de Preços Base:** Definir e manter o preço de venda padrão para cada SKU. Pode incluir suporte a múltiplas listas de preços (ex: preço de varejo padrão, preço para membros VIP, preço B2B, preços regionais). A definição de qual lista aplicar pode depender do contexto do usuário ou do tenant.
  + **Gerenciamento de Regras de Promoção:** Criar, configurar, ativar/desativar e agendar uma variedade de regras promocionais que modificam os preços base ou oferecem outros benefícios. Exemplos incluem:
    - Descontos percentuais ou de valor fixo (ex: 10% OFF em categoria X, R$ 20 de desconto em produto Y).
    - Promoções do tipo "Leve X, Pague Y" (ex: Leve 3, Pague 2).
    - Descontos por quantidade/volume (ex: 5% acima de 10 unidades, 10% acima de 20).
    - Descontos condicionais ou em camadas (ex: 50% de desconto na segunda unidade de menor valor).
    - Promoções de frete (ex: Frete Grátis acima de R$ 199).
    - Combinações e priorização de múltiplas promoções aplicáveis.
  + **Gerenciamento de Cupons:** Ciclo de vida completo dos cupons de desconto: criação (códigos únicos ou genéricos), definição de regras (valor/percentual, aplicabilidade a produtos/categorias/total, valor mínimo, limite de usos, data de validade, uso único por cliente), validação (verificar se o cupom é válido para o carrinho/cliente atual) e controle de utilização (marcar como usado).
  + **Motor de Cálculo de Preços:** O componente central que recebe um contexto (identificação do tenant, usuário, itens do carrinho/pedido, código de cupom) e aplica toda a lógica relevante (preços base, regras de promoção ativas, validação e aplicação de cupom) para determinar os preços finais de cada item e o valor total.
  + **Fornecimento de Informações de Preço:** Expor os preços calculados (ou preços base) de forma eficiente para outros BCs que precisam dessa informação, como Carrinho de Compras, Pedidos e Catálogo/Busca (para exibição na vitrine e páginas de produto).
* **Limites e Exclusões:**
  + **Não Define Produtos/SKUs:** Consome informações do Catálogo para saber sobre quais itens aplicar preços e promoções.
  + **Não Gerencia Estoque:** Não sabe a quantidade disponível (Inventário).
  + **Não Processa Pagamentos:** Não lida com a transação financeira em si (Pagamento).
  + **Não Calcula Frete (Custo):** Apenas pode aplicar promoções relacionadas a frete (ex: Frete Grátis), mas o cálculo do custo do frete em si pertence ao BC de Entrega.
  + **Não Gerencia Carrinho/Pedido:** Apenas realiza cálculos *para* eles quando solicitado.
* **Importância:** Estratégico para o negócio. Permite implementar táticas de venda, promoções sazonais, programas de fidelidade e competir no mercado. A flexibilidade para criar promoções e a performance do cálculo são cruciais para a conversão e a experiência do usuário. Erros aqui podem impactar diretamente o lucro ou a satisfação do cliente.

**2. Event Storming (Focado em Pricing)**

* **Eventos Externos Chave (Consumidos):**
  + SKUDefinido / ProdutoPublicado 🟧 (Do Catálogo): Indica a necessidade de definir um preço base.
  + UsuarioSegmentado 🟧 (Do IAM/Perfil): Pode indicar a necessidade de usar uma lista de preços específica ou aplicar promoções exclusivas.
  + CarrinhoModificado 🟧 (Do Carrinho): Contém os itens atuais, quantidades, tenantId, userId, dispara a necessidade de recalcular os preços do carrinho.
  + CupomInformadoNoCarrinho 🟧 (Do Carrinho): Sinaliza que um cupom precisa ser validado e potencialmente aplicado no próximo cálculo.
  + PedidoConfirmado 🟧 (Do Pedido): Pode ser o gatilho para marcar um cupom de uso único como utilizado.
* **Fluxo Interno Principal (Cálculo Preço Carrinho):**
  + SolicitarCalculoPrecoCarrinho 🟦 (Comando/Requisição vindo do Carrinho/UI Checkout) -> Dados: tenantId, userId, itens: [{skuId, qtd}], codigoCupom?
  + **Política/Motor:** "Calcular Preços" ⚙️
    1. Obter Preços Base 🟩 (Consulta ProjecaoPrecoBaseAtivo para os SKUs e contexto)
    2. Obter Regras de Promoção Ativas 🟩 (Consulta ProjecaoRegrasPromocaoAtivas para o contexto)
    3. Obter Dados Cupom (se houver) 🟩 (Consulta ProjecaoCupomValido)
    4. Aplicar Preços Base aos Itens.
    5. Aplicar Regras de Promoção de Item (ex: % off em SKU X). Calcular subtotal por item.
    6. Aplicar Regras de Promoção de Carrinho/Pedido (ex: Leve 3 Pague 2, Frete Grátis acima de Y). Calcular subtotal geral.
    7. Se houver Cupom: ValidarAplicacaoCupom 🟦 (Comando interno para Cupom Aggregate 🟨 ou verificação direta na Projeção).
    8. Se Cupom Válido: CupomValidadoParaAplicacao 🟧 (Interno) -> Aplicar desconto do cupom ao subtotal.
    9. Calcular Valor Final.
  + PrecoCarrinhoCalculado 🟧 (Evento publicado OU Resposta síncrona) -> Payload detalhado com preços originais, descontos, cupom, valor final.
* **Fluxo Interno (Admin Cria Promoção "Leve 3 Pague 2"):**
  + CriarRegraPromocao 🟦 (Admin Tenant 🧍 via UI) -> Dados: tenantId, nome, tipo="LEVE\_X\_PAGUE\_Y", config={qtdLeve:3, qtdPague:2, skusAplicaveis:[...]}, datas, etc. -> Comando para RegraPromocao Aggregate 🟨.
  + RegraPromocaoCriada 🟧 (Publicado) -> Persistido, status "Agendada" ou "Ativa".
  + *(Consumo por Read Side):* Event handler atualiza a ProjecaoRegrasPromocaoAtivas se a regra estiver ativa.
* **Fluxo Interno (Cliente usa Cupom "BEMVINDO10"):**
  + *(Durante Cálculo Preço Carrinho com codigoCupom="BEMVINDO10")*
  + **Política/Motor:** ⚙️ ... (Passos 1-6) ...
  + **Política/Motor:** ⚙️ ValidarAplicacaoCupom 🟦 (para Cupom Aggregate 🟨 ou consulta ProjecaoCupomValido)
  + CupomValidadoParaAplicacao 🟧 (Interno)
  + **Política/Motor:** ⚙️ Aplica 10% de desconto.
  + PrecoCarrinhoCalculado 🟧 (Retorna resultado com desconto aplicado)
  + *(Após PedidoConfirmado)* -> MarcarCupomComoUtilizado 🟦 (Comando para Cupom Aggregate 🟨) -> Dados: cupomId ou codigo, pedidoId.
  + CupomUtilizado 🟧 (Publicado) -> Status do cupom atualizado no write-side e na projeção.

**3. Event Modelling (Exemplos de Eventos Chave)**

* PrecoBaseSKUDefinido (v1): { ..., "aggregateId": "sku-uuid-xyz#varejo", "payload": { "tenantId": "...", "skuId": "...", "listaPrecoId": "varejo", "moeda": "BRL", "valor": 199.99, ... } }
* RegraPromocaoCriada (v1): { ..., "aggregateId": "promo-uuid-789", "payload": { "tenantId": "...", "promocaoId": "...", "nome": "Leve 3 Pague 2 - Camisetas", "tipoPromocao": "LEVE\_X\_PAGUE\_Y", "status": "ATIVA", ..., "configuracao": { "quantidadeLevar": 3, "quantidadePagar": 2 }, "condicoesAplicacao": { "skusAplicaveis": ["sku-tshirt-red", "sku-tshirt-blue"], ...} } }
* CupomCriado (v1): { ..., "aggregateId": "cupom-uuid-abc", "payload": { "tenantId": "...", "cupomId": "...", "codigo": "CLIENTEVIP50", "tipoDesconto": "VALOR\_FIXO", "valorDesconto": 50.00, "moeda": "BRL", "status": "DISPONIVEL", ..., "regrasAplicacao": { "valorMinimoPedido": 200.00, "usoUnicoPorCliente": true } } }
* CupomUtilizado (v1): { ..., "aggregateId": "cupom-uuid-abc", "payload": { "tenantId": "...", "cupomId": "...", "codigo": "CLIENTEVIP50", "pedidoId": "pedido-uuid-xyz", "userId": "user-uuid-123", ... } }
* PrecoCarrinhoCalculado (v1): (Como descrito no Event Storming - estrutura detalhada com totais e descontos)

**4. Modelagem dos Componentes (DDD Tático)**

* **Aggregates:**
  + **PrecoSKU Aggregate:** (Simples, focado no preço base por SKU/Lista)
    - **Root:** PrecoSKU
    - **ID:** Composto TenantId + SkuId + ListaPrecoId (Value Object)
    - **Atributos:** Moeda (VO), Valor (Money VO), Timestamp.
    - **Invariantes:** Valor >= 0.
    - **Comandos:** DefinirOuAtualizarPrecoBase.
    - **Eventos:** PrecoBaseSKUDefinido.
  + **RegraPromocao Aggregate:** (Complexo, define uma regra promocional)
    - **Root:** RegraPromocao
    - **ID:** PromocaoId (VO, UUID)
    - **Atributos:** TenantId, Nome, Descricao, TipoPromocao (Enum VO), Status (Enum VO), DataInicio, DataFim, Configuracao (VO polimórfico), CondicoesAplicacao (VO), Prioridade.
    - **Invariantes:** Datas válidas, Configuração/Condições consistentes com Tipo, Status transita corretamente.
    - **Comandos:** Criar, Atualizar, Agendar, Ativar, Desativar.
    - **Eventos:** RegraPromocaoCriada, Atualizada, Ativada, Desativada.
  + **Cupom Aggregate:** (Gerencia o ciclo de vida de um cupom)
    - **Root:** Cupom
    - **ID:** CupomId (VO, UUID)
    - **Atributos:** TenantId, Codigo (VO), Descricao, TipoDesconto (Enum VO), ValorDesconto (Money VO ou Percentual), Status (Enum VO), UsosMaximos, UsosAtuais, DataExpiracao, RegrasAplicacao (VO).
    - **Invariantes:** Unicidade de código (por tenant?), UsosAtuais <= UsosMaximos, Status transita corretamente, Regras válidas.
    - **Comandos:** Criar, Invalidar, RegistrarUso (lógica interna para incrementar UsosAtuais e mudar Status se necessário).
    - **Eventos:** CupomCriado, CupomInvalidado, CupomUsoRegistrado.
* **Value Objects:** SkuId, TenantId, ListaPrecoId, Moeda, Money, PromocaoId, TipoPromocao, StatusPromocao, ConfiguracaoPromocao (vários tipos), CondicoesAplicacao, CupomId, CodigoCupom, TipoDescontoCupom, StatusCupom, RegrasAplicacaoCupom.
* **Entities:** PrecoSKU, RegraPromocao, Cupom.
* **CQRS:**
  + **Write Side:** Aggregates persistem seus estados em **PostgreSQL**. JSONB é útil para a flexibilidade das configurações/regras. Transações garantem consistência ao criar/atualizar regras e cupons. Eventos são publicados no RabbitMQ.
  + **Read Side (Projections):**
    - **ProjecaoPrecoBaseAtivo (Redis):** Cache K/V (preco:tenantId:listaId:skuId -> valor) para acesso ultra-rápido pelo motor de cálculo. Populado por PrecoBaseSKUDefinido.
    - **ProjecaoRegrasPromocaoAtivas (Redis/Elasticsearch):** Cache/Índice das regras *atualmente* válidas. Redis (Sets, Hashes) pode funcionar para buscas simples. Elasticsearch se a consulta por regras aplicáveis for complexa (baseada em múltiplos atributos/condições). Populado/Atualizado por RegraPromocao\* eventos.
    - **ProjecaoCupomValido (Redis):** Cache K/V (cupom:tenantId:codigo -> {cupomId, status, regras...}) para validação rápida do código e regras básicas. Populado/Atualizado por Cupom\* eventos.
    - **ProjecaoDetalhesCalculo (Opcional - MongoDB/PostgreSQL):** Se for necessário armazenar o resultado detalhado de cada cálculo de preço para auditoria ou exibição posterior. Populado por PrecoCarrinhoCalculado.
  + **Motor de Precificação (Serviço):** Componente chave, stateless, que orquestra a leitura das projeções (Redis/ES) e aplica a lógica de cálculo. Pode ser invocado via gRPC (para baixa latência interna) ou REST.

**5. Sagas / Process Managers**

* **SagaAgendamentoPromocao:**
  + **Trigger:** Scheduler periódico.
  + **Lógica:** Busca RegraPromocao com status 'Agendada'/'Ativa' e datas correspondentes. Envia comandos AtivarPromocao ou DesativarPromocao aos aggregates.
* **SagaExpiracaoCupom:**
  + **Trigger:** Scheduler periódico.
  + **Lógica:** Busca Cupom com status 'Disponivel' e data de expiração passada. Envia comando InvalidarCupom aos aggregates.

**6. Considerações Adicionais**

* **Performance Crítica:** O cálculo de preço do carrinho/pedido é um hot path. O design das projeções de leitura e o uso de caches (Redis) são absolutamente essenciais. O motor de cálculo precisa ser eficiente.
* **Complexidade das Regras:** O maior desafio é modelar e implementar a miríade de regras de promoção de forma flexível e performática. O uso de VOs polimórficos para ConfiguracaoPromocao e CondicoesAplicacao ajuda. Testes extensivos são obrigatórios.
* **Interface de Administração:** Tenants precisarão de uma UI poderosa para criar e gerenciar suas promoções e cupons.
* **Consistência:** A consistência eventual entre a definição da regra/cupom (write) e sua aplicação (read via projeção) é geralmente aceitável, mas a *utilização* do cupom (write no aggregate Cupom) deve ser consistente para evitar uso múltiplo indevido.

**Próxima Tarefa:**

Finalizamos a análise do Bounded Context de Precificação e Promoções. O próximo na nossa lista é o **Contexto de Carrinho de Compras (Shopping Cart/Basket)**.

Este nível de detalhe está adequado? Podemos prosseguir para o Carrinho de Compras?