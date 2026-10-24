**Cross-Cutting Concerns (Parte 3: Design das APIs - REST, GraphQL, CLI)**

**1. Introdução à Estratégia de APIs**

Uma arquitetura bem definida precisa de interfaces bem definidas para interação, tanto externa quanto interna (embora o foco aqui seja mais externo). Adotar uma abordagem única para todas as interações pode levar a ineficiências. Por isso, conforme sua solicitação inicial, vamos utilizar uma estratégia **híbrida e orientada a propósito**, alinhada com nosso padrão CQRS:

* **RESTful API para Comandos:** Utilizaremos REST (Representational State Transfer) para operações que visam **mudar o estado** do sistema (operações de escrita: criar, atualizar, excluir).
* **GraphQL API para Consultas:** Utilizaremos GraphQL para operações que visam **buscar dados** do sistema (operações de leitura).
* **CLI (Command Line Interface) para Administração:** Forneceremos uma ferramenta de linha de comando para tarefas administrativas, operacionais e de manutenção.

**Objetivos desta estratégia:**

* **Clareza de Intenção:** Separar explicitamente as operações de escrita (comandos) das de leitura (consultas).
* **Interfaces Otimizadas:** Oferecer a melhor interface para cada caso de uso (flexibilidade do GraphQL para buscar dados complexos, clareza do REST para ações de mudança de estado).
* **Performance:** Permitir otimizações específicas para leitura e escrita.
* **Developer Experience (DX):** Fornecer interfaces bem documentadas e adequadas para os diferentes tipos de consumidores (frontend, apps móveis, administradores).

**2. Papel do API Gateway (Traefik + Middleware)**

Reiterando, o API Gateway (Traefik, possivelmente com um serviço de middleware adicional ou usando plugins/middleware do Traefik Enterprise/Mesh) é o **ponto de entrada único** para todas as requisições externas. Suas responsabilidades incluem:

* **Roteamento:** Direcionar as requisições para o serviço backend apropriado (seja um handler de comando REST, o servidor GraphQL, etc.) com base no path, método HTTP, host, etc.
* **Terminação SSL/TLS:** Garantir comunicação segura (HTTPS).
* **Load Balancing:** Distribuir a carga entre as instâncias dos serviços backend.
* **Middleware Essencial:** Aplicar lógicas transversais como:
  + **Validação de Autenticação (JWT):** Validar o access\_token (conforme fluxo AuthN descrito anteriormente).
  + **Rate Limiting:** Proteger os serviços contra abuso.
  + **CORS (Cross-Origin Resource Sharing):** Configurar políticas para permitir requisições de domínios diferentes (ex: SPA frontend).
  + **Compressão (Gzip).**
  + **Logging/Tracing Básico:** Registrar requisições de entrada/saída e participar do tracing distribuído.

**3. API de Comandos (RESTful)**

* **Propósito:** Interface para todas as operações que modificam o estado do sistema. Alinhada com o lado "Command" do CQRS.
* **Por que REST?**
  + Maturidade e Ampla Adoção: Bem compreendido, vasto ferramental.
  + Verbos HTTP Semânticos: Métodos como POST, PUT, PATCH, DELETE mapeiam bem as intenções de mudança de estado.
  + Stateless: Cada requisição contém toda a informação necessária.
  + Cacheabilidade (embora menos relevante para comandos).
* **Princípios de Design:**
  + **Orientação a Recursos (quando aplicável):** Modelar endpoints em torno dos recursos de negócio (ex: /v1/produtos, /v1/pedidos, /v1/vendedores/{vendedorId}/perfil-loja).
  + **Uso Semântico dos Métodos HTTP:**
    - POST: Geralmente para criar um novo recurso (POST /v1/produtos) ou para disparar uma ação/processo que não se encaixa perfeitamente em CRUD (POST /v1/pedidos/{id}/cancelar-solicitacao).
    - PUT: Para substituir completamente um recurso (menos comum para comandos complexos, talvez para atualizar um perfil inteiro: PUT /v1/carrinho/itens/{itemId}).
    - PATCH: Ideal para atualizações parciais e específicas (ex: PATCH /v1/pedidos/{id} com corpo {"status": "ENVIADO", "dadosRastreio": {...}}).
    - DELETE: Para solicitar a remoção de um recurso (DELETE /v1/carrinho/itens/{itemId}).
  + **Corpo da Requisição (Payload):** Contém os dados necessários para executar o comando (o Command DTO - Data Transfer Object). Usar JSON claro e bem definido. Validar rigorosamente o payload.
  + **Respostas:**
    - **Assíncrona (Preferencial para EDA):** Para comandos que iniciam processos potencialmente demorados (ex: criar pedido, processar pagamento):
      1. Validar input e autorização.
      2. Aceitar o comando (ex: publicar em uma fila RabbitMQ ou chamar uma Saga).
      3. Retornar **HTTP 202 Accepted** imediatamente.
      4. (Opcional) Incluir header Location apontando para um recurso onde o status do processamento pode ser consultado (ex: Location: /v1/pedidos/{novoPedidoId}).
    - **Síncrona (Casos Simples):** Para comandos muito rápidos e atômicos (ex: atualizar nome de usuário), pode retornar HTTP 200 OK (com corpo, se relevante) ou HTTP 204 No Content. **Evitar** para operações que envolvam múltiplos passos ou serviços.
  + **Idempotência:** Para operações críticas que podem ser retentadas (ex: SolicitarProcessamentoPagamento via API), exigir um header Idempotency-Key (com um valor único por tentativa) para que o backend possa detectar e ignorar requisições duplicadas.
  + **Tratamento de Erros:** Utilizar códigos de status HTTP padrão de forma consistente:
    - 400 Bad Request: Erros de validação do payload do comando.
    - 401 Unauthorized: Token JWT ausente ou inválido (problema de autenticação).
    - 403 Forbidden: Usuário autenticado, mas sem permissão para a ação (problema de autorização).
    - 404 Not Found: Recurso alvo do comando não encontrado (ex: PATCH /v1/pedidos/id\_inexistente).
    - 409 Conflict: Ação não pode ser realizada devido ao estado atual do recurso (ex: tentar cancelar pedido já enviado).
    - 422 Unprocessable Entity: Validação semântica falhou (diferente do 400 que é sintático).
    - 500 Internal Server Error: Erro inesperado no servidor.
    - 503 Service Unavailable: Serviço temporariamente indisponível ou sobrecarregado.
    - Incluir um corpo de resposta JSON padronizado para erros, com código de erro interno, mensagem clara e detalhes (se aplicável).
  + **Versionamento:** Implementar versionamento explícito na URL (ex: /v1/..., /v2/...) para gerenciar evoluções da API sem quebrar clientes existentes.
* **Roteamento:** O API Gateway mapeia os paths e métodos HTTP para os serviços de backend que hospedam os Command Handlers (endpoints REST específicos).

**4. API de Consultas (GraphQL)**

* **Propósito:** Interface otimizada para buscar dados do sistema, lendo das projeções de leitura (Read Models) do CQRS.
* **Por que GraphQL?**
  + **Busca de Dados Flexível:** Clientes (frontend/mobile) pedem *exatamente* os campos que precisam, evitando over/under-fetching.
  + **Menos Requisições:** Capacidade de buscar dados relacionados de múltiplos "recursos" ou projeções em uma única requisição de rede.
  + **Endpoint Único:** Geralmente expõe um único endpoint (ex: /graphql) para todas as consultas (e mutações, se usadas, mas estamos focando em queries aqui).
  + **Schema Fortemente Tipado:** O Schema Definition Language (SDL) define todos os tipos, queries e suas relações, servindo como contrato e auto-documentação. Facilita o desenvolvimento frontend com ferramentas de geração de código e validação.
* **Implementação:**
  + **Servidor GraphQL:** Um serviço dedicado (ou funcionalidade no BFF/Gateway) que expõe o endpoint /graphql. Quarkus oferece excelente suporte com a extensão SmallRye GraphQL.
  + **Schema Definition Language (SDL):** O coração do GraphQL. Definir:
    - **type Query { ... }**: Pontos de entrada para buscar dados. Ex: pedido(id: ID!): Pedido, produtos(filtro: FiltroProduto, paginacao: PaginacaoInput): ConexaoProduto, usuarioAtual: Usuario, lojaVendedor(id: ID!): PerfilLoja.
    - **Object Types:** Definir os tipos de dados que podem ser retornados (Pedido, ItemPedido, Produto, Avaliacao, Usuario, etc.), espelhando as Projeções de Leitura.
    - **Input Types:** Definir tipos para argumentos complexos (filtros, paginação, ordenação).
    - Enums, Interfaces, Unions: Usar conforme necessário para modelar o domínio de leitura.
  + **Resolvers:** Funções Java (em Quarkus, métodos anotados com @Query, @Source, etc.) responsáveis por buscar os dados para cada campo do schema. A lógica do resolver tipicamente envolve:
    - Consultar a projeção de leitura apropriada (cache Redis, tabela PostgreSQL, documento MongoDB, índice Elasticsearch).
    - **Evitar** fazer lógica de negócio complexa ou múltiplas chamadas síncronas a outros serviços *dentro* de um resolver (isso deve ser feito na construção da projeção). O resolver deve ser uma camada fina de busca de dados pré-computados/otimizados.
  + **DataLoader Pattern:** Essencial para otimizar a busca de dados relacionados e evitar o problema "N+1". O DataLoader agrupa e despacha requisições para buscar dados relacionados em lote (ex: buscar detalhes de múltiplos produtos referenciados em uma lista de pedidos com uma única query otimizada). SmallRye GraphQL tem suporte integrado.
  + **Paginação:** Implementar um padrão consistente, preferencialmente a especificação **GraphQL Cursor Connections** (com edges, node, pageInfo, hasNextPage, endCursor) para paginação eficiente e estável.
  + **Filtragem e Ordenação:** Permitir que as queries aceitem argumentos (Input Types) para filtrar e ordenar os resultados. A lógica de filtragem/ordenação é aplicada na consulta à projeção de leitura.
  + **Autorização:** Integrar verificações de autorização nos resolvers. Antes de retornar dados, verificar (consultando o IAM BC ou usando contexto injetado) se o usuário atual tem permissão para ver aqueles dados específicos (ex: só pode ver pedido(id:...) se for o dono ou um admin). Filtrar listas com base nas permissões.
* **Roteamento:** O API Gateway direciona todas as requisições para /graphql (geralmente POST) para o Servidor GraphQL.

**5. CLI Administrativa (Linha de Comando)**

* **Propósito:** Ferramenta para administradores da plataforma e/ou administradores de tenant realizarem tarefas operacionais, de configuração ou manutenção que não justificam uma interface gráfica completa ou são mais eficientes via linha de comando.
* **Por que CLI?**
  + **Automação/Scripting:** Fácil de integrar em scripts para tarefas repetitivas ou em lote.
  + **Acesso Controlado:** Pode ter mecanismos de autenticação/autorização robustos específicos para administradores.
  + **Tarefas de Baixo Nível:** Permite realizar operações que podem ser perigosas ou complexas demais para uma UI (ex: correções de dados pontuais - com muito cuidado!, disparo de jobs de manutenção).
  + **Custo-Benefício:** Evita o desenvolvimento de interfaces gráficas complexas para funcionalidades usadas infrequentemente.
* **Implementação:**
  + **Aplicação Java/Quarkus:** Usar bibliotecas como quarkus-picocli para construir facilmente aplicações de linha de comando robustas em Java.
  + **Comandos:** Definir subcomandos claros e bem documentados para cada tarefa. Ex:
    - marketplace tenant create --nome "Nova Loja" --plano "premium"
    - marketplace tenant activate <tenantId>
    - marketplace seller approve <vendedorId>
    - marketplace user list --tenant <tenantId> --role "admin"
    - marketplace system job trigger --jobName "recalcularMetricasAvaliacoes"
  + **Interação com Backend:** A CLI pode interagir com os serviços backend de algumas formas:
    1. **Via API REST de Comandos:** A CLI atua como um cliente da API REST, enviando comandos HTTP. É uma abordagem segura e reutiliza a API existente. Requer que a CLI se autentique como um cliente privilegiado (ex: usando Client Credentials Flow do OAuth2).
    2. **Chamadas Diretas a Serviços (gRPC/REST Interno):** Se a CLI rodar em um ambiente com acesso seguro à rede interna, pode fazer chamadas diretas às APIs internas dos serviços. Requer autenticação/autorização próprias para a CLI nesses endpoints.
    3. **Interação Direta com Infra (DB/Queues - Usar com Extremo Cuidado):** Para tarefas de manutenção muito específicas, pode interagir diretamente com bancos de dados ou message brokers. **Altamente arriscado**, requer conhecimento profundo, permissões restritas e procedimentos operacionais rigorosos. Evitar sempre que possível.
  + **Autenticação/Autorização da CLI:** A própria ferramenta CLI precisa ser autenticada (ex: login interativo para admin, chaves de API para scripts) e suas ações autorizadas pelo IAM BC (a CLI precisa ter um "ator" com permissões administrativas).

**6. Consistência e Padrões**

* Manter consistência nos formatos de data, convenções de nomenclatura, estrutura de erros e padrões de paginação entre as APIs REST e GraphQL.
* Utilizar **OpenAPI Specification (OAS)** para documentar a API REST e o **Schema GraphQL (SDL)** para a API GraphQL. Essas definições podem ser usadas para gerar documentação interativa (Swagger UI, GraphiQL/GraphQL Playground) e SDKs/clientes.

**Próxima Tarefa:**

Cobrimos o design das interfaces API (REST, GraphQL, CLI). Um ponto mencionado foi a comunicação interna serviço-a-serviço. A sua solicitação inicial mencionou o uso de **gRPC para comunicação síncrona quando pertinente**. Portanto, a próxima etapa natural seria detalhar o uso do gRPC dentro da arquitetura.

O detalhamento sobre o Design das APIs foi satisfatório? Podemos prosseguir para a discussão sobre o uso de gRPC para comunicação síncrona interna?