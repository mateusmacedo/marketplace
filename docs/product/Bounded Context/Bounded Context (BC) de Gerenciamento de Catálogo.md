Bounded Context (BC) de **Gerenciamento de Catálogo**

**Entender o Pedido:** O usuário solicitou uma exploração profunda do BC "Gerenciamento de Catálogo", seguindo o padrão já estabelecido: Definição Descritiva, Event Storming focado, Event Modelling, Modelagem Tática (Agregados, VOs, Entidades, CQRS) e Sagas. O contexto é sempre a plataforma Marketplace White Label Multi-Tenant.

1. **Desconstruir o Conceito de "Catálogo":** O que significa "catálogo" neste contexto?
   * É a fonte da verdade sobre *o que* pode ser vendido. Foca na *descrição*, *classificação* e *identificação* dos itens.
   * Inclui: Produtos (a ideia geral), Variações/SKUs (as coisas específicas que se compra e estoca, ex: camiseta M vermelha), Categorias (organização), Marcas (fabricantes), Atributos (cor, tamanho, material, voltagem), Mídia (fotos, vídeos).
   * Crucialmente, *não* inclui: Quantidade disponível (isso é Inventário), Preço (isso é Precificação), Pedidos de clientes, Avaliações de clientes.
   * No cenário Multi-Tenant/Multi-Vendedor: Quem gerencia o quê? Um vendedor cadastra seus próprios produtos, ou existe um catálogo mestre? (Precisa definir uma premissa ou abordar as opções).
2. **Passo 1: Refinar a Definição Descritiva:**
   * Começar com o propósito central: Definir e enriquecer os itens vendáveis.
   * Listar as responsabilidades detalhadas: Gerenciar Produtos, Variações/SKUs, Categorias, Atributos, Marcas, Mídia. Mencionar Coleções como opcional.
   * Abordar explicitamente o aspecto Multi-Vendedor: Definir a premissa adotada (ex: vendedor gerencia seus produtos dentro do tenant) e mencionar alternativas para clareza de escopo.
   * Definir os limites claros: Separar explicitamente de Inventário, Precificação, Pedidos, Avaliações. Enfatizar o que o BC *não* faz.
   * Explicar a importância do BC: Base para a loja, UX, e outros processos vitais.
3. **Passo 2: Simular Event Storming Focado:**
   * Selecionar fluxos de trabalho *dentro* do Catálogo: Criação de produto (simples vs. com variações), adição de mídia, categorização, publicação/ativação, atualização.
   * Detalhar um fluxo chave (ex: Produto com Variações):
     + Mapear Comandos (intenções do usuário/sistema) e Eventos (fatos ocorridos). Ex: DefinirProdutoBase (Cmd) -> ProdutoBaseCriado (Evt); GerarCombinacoesDeSKU (Cmd) -> SKUDefinido (Evt, *um por SKU*); PublicarProduto (Cmd) -> ProdutoPublicado (Evt - evento crítico!).
     + Identificar Atores (Vendedor, Admin Tenant).
     + Identificar os Agregados principais envolvidos (claramente o Produto).
     + Mostrar como eventos se encadeiam e a granularidade necessária (ex: evento por SKU).
4. **Passo 3: Modelar os Eventos Chave:**
   * Selecionar os eventos mais significativos do Event Storming (ProdutoCriado, SKUDefinido, ProdutoPublicado, CategoriaCriada, etc.).
   * Propor uma estrutura (ex: JSON) para cada evento:
     + Incluir metadados padrão (eventId, eventType, eventVersion, timestamp, aggregateId, correlationId).
     + Definir o payload com os dados relevantes para aquele evento específico. O que mudou? Qual o estado resultante? (ex: SKUDefinido precisa dos atributos, códigos, dimensões do SKU).
     + Incluir versionamento (v1).
5. **Passo 4: Modelar Componentes Táticos (DDD):**
   * **Agregados:**
     + Identificar os limites de consistência: Produto é o agregado central óbvio. Categoria e Marca parecem independentes e coesos o suficiente para serem seus próprios agregados. Talvez AtributoDefinicao se for dinâmico.
     + Detalhar o Agregado Produto:
       - Definir a Raiz (Produto Entity), ID (ProdutoId).
       - Listar atributos básicos (Nome, Descricao, MarcaId, Status).
       - Modelar as coleções/entidades internas: Como representar variações? Map<SkuId, SKU> onde SKU é uma Entidade interna parece adequado. Definir a estrutura da entidade SKU (ID, atributos, códigos, dimensões, status, imagens). Incluir outras coleções (Set<CategoriaId>, Set<ImagemProduto>, etc.).
       - Definir Invariantes: Regras que o agregado *deve* garantir (unicidade de atributos de SKU dentro do produto, status válidos, produto ativo requer SKU ativo?, etc.).
       - Listar Comandos: Operações que podem ser realizadas no agregado.
       - Listar Eventos: O que o agregado publica após sucesso.
     + Detalhar brevemente os agregados Categoria e Marca (estrutura mais simples, ID, atributos, relacionamentos de hierarquia para categoria, invariantes como nome/slug único, comandos, eventos).
   * **Value Objects:** Listar os VOs identificados (IDs, campos descritivos, estruturas como Dimensoes, Peso, Enums de Status).
   * **Entities:** Listar as entidades identificadas (raízes de agregado e entidades internas como SKU).
   * **CQRS:**
     + **Write Side:** Escolher DB (PostgreSQL com JSONB é bom para relações e flexibilidade). Enfatizar ACID dentro do agregado. Publicar eventos no Broker (RabbitMQ).
     + **Read Side:** Identificar projeções necessárias (Detalhe Produto, Listagem/Busca, Navegação Categoria, Feeds para outros BCs). Escolher DBs adequados para cada projeção (Elasticsearch para busca, MongoDB/PostgreSQL para views, Redis para cache). Explicar como são atualizadas (Handlers de Eventos).
6. **Passo 5: Identificar Sagas Potenciais:**
   * Pensar em processos que envolvem múltiplos passos ou sistemas externos, iniciados por eventos do Catálogo.
   * Exemplos: Publicação complexa (validação, indexação), exclusão (notificações em cascata). Definir gatilho e passos orquestrados.
7. **Passo 6: Adicionar Considerações Finais:**
   * Resumir decisões chave/desafios: Escolhas de BD (persistência poliglota), implicações Multi-Vendedor, extensibilidade do modelo, pontos de integração cruciais (eventos para Inventário/Preço).
8. **Passo 7: Concluir com Chamada para Ação:**
   * Verificar se o nível de detalhe atende à expectativa.
   * Propor explicitamente o próximo BC (Inventário) para dar sequência lógica.
9. **Auto-Correção/Refinamento Durante o Processo:**
   * A separação de Inventário/Preço está clara? Reforçar os limites.
   * O modelo de SKU está detalhado? Garantir que atributos, códigos, status estejam presentes.
   * O aspecto multi-vendedor foi tratado? Adicionar notas sobre as escolhas de design.
   * As projeções CQRS são lógicas e servem a um propósito claro? As escolhas de BD são justificadas?
   * Os eventos são granulares o suficiente, mas não excessivos? SKUDefinido parece necessário para consumidores downstream. ProdutoPublicado é um marco chave.