**Bounded Context - Carrinho de Compras (Shopping Cart / Basket)**

**1. Definição Descritiva Refinada**

* **Propósito Central:** Este Bounded Context (BC) gerencia o estado **transitório e volátil** que representa a intenção de compra de um usuário *antes* da formalização de um pedido. Funciona como uma área de rascunho pessoal onde os clientes podem coletar, visualizar, modificar quantidades e remover itens (SKUs) que consideram adquirir, antes de se comprometerem com a compra através do checkout.
* **Responsabilidades Principais:**
  + **Adição/Remoção/Atualização de Itens:** Permitir aos usuários manipular o conteúdo do carrinho: adicionar um SKU específico com uma quantidade desejada, remover um item completamente, ou alterar a quantidade de um item já presente.
  + **Persistência Temporária:** Manter o estado do carrinho associado a um identificador único, que pode ser um UserId para clientes logados ou um SessionId (geralmente via cookie) para visitantes anônimos. Essa persistência tem um tempo de vida limitado.
  + **Recuperação e Associação:** Lidar com a transição de um carrinho anônimo para um carrinho de usuário logado (ex: quando um visitante com itens no carrinho faz login). Isso pode envolver mesclar carrinhos, substituir um pelo outro ou manter ambos, dependendo da regra de negócio definida.
  + **Gerenciamento do Ciclo de Vida:** Controlar o status do carrinho (ex: Aberto, Em Checkout, Finalizado, Abandonado) e implementar a limpeza automática (exclusão) de carrinhos considerados abandonados (inativos por um período configurável).
  + **Orquestração de Cálculos Externos (Delegação):**
    - **Preço:** *Solicitar* ao BC de Precificação o cálculo atualizado dos preços dos itens e do total do carrinho sempre que o conteúdo for modificado. Armazena o *resultado* desse cálculo (snapshot) para exibição.
    - **Disponibilidade (Informativa):** *Pode* consultar (ler projeções) o BC de Inventário para exibir informações básicas de disponibilidade (ex: "em estoque", "últimas unidades") ao lado dos itens no carrinho, mas **não** realiza reservas de estoque.
  + **Fornecimento para Checkout:** Disponibilizar o conteúdo final e validado do carrinho (itens, quantidades, preços calculados) para o BC de Pedidos no momento em que o usuário decide iniciar o processo de checkout.
* **Limites e Exclusões:**
  + **Não é Pedido:** O carrinho é um precursor do pedido. A ação de "Finalizar Compra" ou "Iniciar Checkout" a partir do carrinho dispara a criação de um Pedido no BC correspondente, usando os dados do carrinho como base.
  + **Não Garante Estoque:** A presença de um item no carrinho não significa que o estoque está reservado. A reserva efetiva ocorre tipicamente durante o processo de checkout, gerenciada pelo BC de Pedidos em coordenação com o Inventário.
  + **Não Garante Preço Final:** Os preços exibidos são um snapshot do cálculo mais recente feito pelo BC de Precificação. Embora desejável manter a estabilidade, os preços podem teoricamente mudar até a confirmação final do pedido (promoções podem expirar, etc.). O "congelamento" ocorre geralmente no início do checkout.
  + **Não Lida com Pagamento/Entrega/Faturamento:** Estas são responsabilidades de outros BCs que atuam sobre o Pedido formalizado.
* **Importância:** Componente essencial da experiência de compra online. Facilita a exploração e seleção de produtos. Um carrinho que funciona bem, persiste entre sessões (dentro do razoável) e reflete preços atualizados (mesmo que via delegação) contribui significativamente para a taxa de conversão. É a manifestação da intenção de compra do cliente.

**2. Event Storming (Focado em Shopping Cart)**

* **Eventos Externos Chave (Informativos / Gatilhos para Consulta):**
  + ProjecaoDetalheProduto 🟩 (Leitura do Catálogo): Para buscar nome, imagem do SKU adicionado.
  + ProjecaoDisponibilidadeSKU 🟩 (Leitura do Inventário): Para exibir status de estoque (opcional).
  + PrecoCarrinhoCalculado 🟧 (Consumido da Precificação): Resultado do cálculo de preços solicitado.
  + UsuarioLogado 🟧 (Do IAM): Pode disparar a lógica de associação de carrinho anônimo.
* **Fluxo Interno Principal (Cliente Adiciona Item):**
  + AdicionarItem 🟦 (Cliente 🧍 via UI, envia skuId, qtd, sessionId ou userId) -> Comando para Carrinho Aggregate 🟨.
  + *(Política Interna: "Enriquecer Item")* -> Consulta Projeções 🟩 (Catálogo para nome/imagem).
  + ItemAdicionado 🟧 (Publicado) -> Contém dados do item, carrinhoId, tenantId. Persistido no Aggregate.
  + *(Política Interna: "Manter Preços Atualizados")* -> Dispara SolicitarCalculoPrecoCarrinho 🟦 (Comando/Requisição para BC Precificação).
* **Fluxo Interno (Preços Recalculados Chegam):**
  + *(Listener consome PrecoCarrinhoCalculado)* 🟧 -> Dados: resultado detalhado do cálculo.
  + AplicarPrecosCalculados 🟦 (Comando interno para Carrinho Aggregate 🟨).
  + PrecosAtualizadosNoCarrinho 🟧 (Publicado) -> Contém os preços snapshotados. Persistido no Aggregate.
* **Fluxo Interno (Cliente Inicia Checkout):**
  + IniciarCheckout 🟦 (Cliente 🧍 clica em "Finalizar Compra") -> Comando para Carrinho Aggregate 🟨.
  + *(Política Interna: "Validar e Preparar para Pedido")* -> Verifica se carrinho não está vazio, se todos os itens são válidos, etc. Muda status do carrinho para "EmCheckout".
  + CheckoutIniciado 🟧 (Publicado) -> Contém o **payload completo e final** do carrinho (itens, quantidades, preços finais calculados, userId, tenantId). **Este é o evento crucial que o BC de Pedidos ouvirá para criar um novo Pedido.**
  + *(Opcional: Política Interna)* -> Pode disparar LimparCarrinho 🟦 após um tempo ou após confirmação do pedido, mudando o status para "Finalizado".
* **Fluxo Interno (Limpeza de Carrinho Abandonado):**
  + *(Scheduler)* -> IdentificarCarrinhosAbandonados 🟦 (Consulta carrinhos com timestampUltimaModificacao antiga).
  + Para cada carrinho identificado -> MarcarComoAbandonado / ExcluirCarrinho 🟦 (Comando para Carrinho Aggregate 🟨).
  + CarrinhoExcluido 🟧 (Publicado, motivo "ABANDONO").

**3. Event Modelling (Exemplos de Eventos Chave)**

* CarrinhoCriado (v1): { ..., "aggregateId": "cart-uuid-789", "payload": { "carrinhoId": "...", "tenantId": "...", "userId": null, "sessionId": "sess-abc-123", ... } }
* ItemAdicionado (v1): { ..., "aggregateId": "cart-uuid-789", "payload": { "carrinhoId": "...", "tenantId": "...", "item": { "skuId": "sku-book-123", "produtoId": "prod-book-abc", "quantidade": 1, "nomeSnapshot": "Livro DDD Essencial", "imagemUrlSnapshot": "/img/ddd.jpg" } } }
* QuantidadeItemAtualizada (v1): { ..., "aggregateId": "cart-uuid-789", "payload": { "carrinhoId": "...", "tenantId": "...", "skuId": "sku-book-123", "novaQuantidade": 2 } }
* ItemRemovido (v1): { ..., "aggregateId": "cart-uuid-789", "payload": { "carrinhoId": "...", "tenantId": "...", "skuId": "sku-book-123" } }
* PrecosAtualizadosNoCarrinho (v1): { ..., "aggregateId": "cart-uuid-789", "payload": { "carrinhoId": "...", "tenantId": "...", "snapshotCalculo": { "subtotalItens": 179.80, "descontoPromocional": 17.98, "descontoCupom": 10.00, "valorFinalCalculado": 151.82, "moeda": "BRL" }, "timestampCalculo": "..." } }
* CheckoutIniciado (v1): { ..., "aggregateId": "cart-uuid-789", "payload": { "carrinhoId": "...", "tenantId": "...", "userId": "user-456", "itensSnapshotFinal": [ { "skuId": "...", "quantidade": 2, "precoUnitarioFinal": 71.91, ... } ], "valorTotalFinal": 151.82, "moeda": "BRL", ... } }
* CarrinhoExcluido (v1): { ..., "aggregateId": "cart-uuid-789", "payload": { "carrinhoId": "...", "tenantId": "...", "motivoExclusao": "ABANDONO" | "CHECKOUT\_CONCLUIDO" } }

**4. Modelagem dos Componentes (DDD Tático)**

* **Aggregates:**
  + **Carrinho Aggregate:** (O único aggregate principal neste BC)
    - **Root:** Carrinho (Entity)
    - **ID:** CarrinhoId (VO, UUID).
    - **Atributos:** TenantId (VO), UserId (VO, pode ser nulo inicialmente), SessionId (VO, para anônimos), Status (Enum VO: Aberto, EmCheckout, Finalizado, Abandonado), TimestampCriacao, TimestampUltimaModificacao.
    - **Coleções:** Map<SkuId, ItemCarrinho> onde ItemCarrinho é um **Value Object** contendo: SkuId (VO), ProdutoId (VO), Quantidade (int), NomeSnapshot (String), ImagemUrlSnapshot (String), PrecoUnitarioCalculadoSnapshot (Money VO, opcional, atualizado pelo evento de preço), TimestampAdicao. (Usar VOs aqui simplifica, já que o item não tem ciclo de vida próprio *dentro* do carrinho).
    - **Atributos (Snapshots do Cálculo):** SnapshotPrecoCalculado (VO contendo subtotais, descontos, total, moeda, timestamp do cálculo).
    - **Invariantes:** Quantidade de item > 0. Status transita corretamente. Regras de associação UserId/SessionId. Carrinho não pode ir para "EmCheckout" se estiver vazio.
    - **Comandos:** AdicionarItem, RemoverItem, AtualizarQuantidade, AplicarSnapshotPrecos (interno), IniciarCheckout, MarcarComoAbandonado, ExcluirCarrinho, AssociarUsuario.
    - **Eventos:** CarrinhoCriado, ItemAdicionado, ItemRemovido, QuantidadeItemAtualizada, PrecosAtualizadosNoCarrinho, CheckoutIniciado, CarrinhoExcluido, UsuarioAssociadoAoCarrinho.
* **Value Objects:** CarrinhoId, TenantId, UserId, SessionId, SkuId, ProdutoId, Quantidade (int), Money, StatusCarrinho, ItemCarrinho, SnapshotPrecoCalculado.
* **Entities:** Carrinho.
* **CQRS:**
  + **Write Side:** O Aggregate Carrinho processa comandos. **Redis** é altamente recomendado como banco de dados primário para o write-side do Carrinho. Sua performance em memória e estruturas de dados (Hashes) são ideais para a natureza volátil e de acesso frequente dos carrinhos. A durabilidade pode ser configurada (ex: AOF) se alguma persistência for desejada, mas não precisa ser tão rigorosa quanto a de Pedidos. Cada carrinho seria um Hash cart:tenantId:carrinhoId. Eventos são publicados no RabbitMQ. **MongoDB** é a segunda melhor opção.
  + **Read Side:** Dado que o write-side já está provavelmente em Redis/MongoDB, a necessidade de projeções de leitura *separadas* é mínima. A API que serve a UI pode ler diretamente a estrutura do carrinho do Redis/MongoDB.
    - **ProjecaoCarrinhoUsuario (Leitura Direta):** A API busca o Hash/Documento do carrinho diretamente do Redis/MongoDB.
    - **ProjecaoMonitoramentoCarrinhos (Opcional - Redis/DB):** Um Sorted Set no Redis (carrinhos\_ativos\_por\_ts) com CarrinhoId e TimestampUltimaModificacao como score, ou uma tabela em DB, para facilitar a identificação de carrinhos abandonados pelo processo de limpeza. Atualizado pelos eventos de modificação do carrinho.

**5. Sagas / Process Managers**

* **ProcessManagerAtualizacaoPrecoCarrinho:**
  + **Trigger:** Eventos ItemAdicionado, ItemRemovido, QuantidadeItemAtualizada.
  + **Lógica:** Envia requisição/comando para BC Precificação (SolicitarCalculoPrecoCarrinho). Ouve evento/resposta PrecoCarrinhoCalculado. Envia comando AplicarSnapshotPrecos para o Aggregate Carrinho.
* **SagaLimpezaCarrinhoAbandonado:**
  + **Trigger:** Scheduler.
  + **Lógica:** Consulta ProjecaoMonitoramentoCarrinhos (ou faz query direta) por carrinhos inativos. Envia comando MarcarComoAbandonado ou ExcluirCarrinho para os aggregates identificados.
* **SagaAssociacaoCarrinhoUsuario:**
  + **Trigger:** Evento UsuarioLogado + presença de SessionId de carrinho anônimo.
  + **Lógica:** Busca ambos os carrinhos (anônimo e do usuário, se existir). Aplica regra de negócio (mesclar? substituir?). Envia comandos AssociarUsuario, ExcluirCarrinho (o anônimo, talvez), AdicionarItem (se mesclando) para os aggregates.

**6. Considerações Adicionais**

* **Persistência e Volatilidade:** Escolher a tecnologia de armazenamento (Redis preferível) que equilibre performance com a necessidade de persistência (carrinhos não são tão críticos quanto pedidos).
* **Performance:** Operações no carrinho devem ser extremamente rápidas.
* **Comunicação com Precificação:** Definir se a chamada para calcular preços é síncrona (bloqueia a UI até ter o preço?) ou assíncrona (UI atualiza quando o preço chega?). Assíncrono com feedback visual (spinner) e atualização via WebSocket/SSE é geralmente melhor UX.
* **Evento CheckoutIniciado:** É a "passagem de bastão" para o BC de Pedidos. Deve conter toda a informação necessária para o pedido ser criado, de forma imutável a partir daquele ponto.