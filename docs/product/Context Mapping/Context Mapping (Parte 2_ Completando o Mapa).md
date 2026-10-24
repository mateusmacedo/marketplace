**Context Mapping (Parte 2: Completando o Mapa)**

Continuando de onde paramos, vamos agora mapear as interações dos BCs restantes e como eles se conectam ao núcleo já mapeado. Relembrando nossa lista:

1. Identidade e Acesso (IAM)
2. Configuração do Tenant (Tenant Config)
3. Catálogo (Catalog)
4. Inventário (Inventory)
5. Precificação (Pricing)
6. Carrinho de Compras (Cart)
7. Pedidos (Order)
8. Pagamentos (Payment)
9. Entrega (Fulfillment)
10. Notificações (Notification)
11. Avaliações (Reviews)
12. Gerenciamento de Vendedor (Seller)

**Mapeando as Relações Adicionais:**

* **Relações do Catálogo (Catalog):**
  + **(Catalog) --- [OHS(Events), PL] ---> (Inventory):**
    - **Relação:** Catálogo (Upstream) publica eventos cruciais como SKUDefinido, ProdutoPublicado, ProdutoDesativado. Inventário (Downstream) *precisa* consumir esses eventos (**Published Language - PL**) para saber quais SKUs inicializar ou parar de rastrear. Catálogo expõe isso via seu **Open Host Service (OHS)** de eventos. Inventário pode ser **Conformist** ou ter uma **ACL** simples para esses eventos. A dependência é clara: Inventário depende do Catálogo (C/S).
  + **(Catalog) --- [OHS(Events), PL] ---> (Pricing):**
    - **Relação:** Idêntica à relação com Inventário. Pricing (Downstream) consome os mesmos eventos (**PL** via **OHS** de eventos) do Catálogo (Upstream) para saber quais SKUs precisam ter um preço base definido. Pode ser **Conformist** ou **ACL**. Pricing depende do Catálogo (C/S).
  + **(Catalog) --- [OHS(API/Query), PL] ---> (Cart / Order / Reviews / UI Layer):**
    - **Relação:** Catálogo (Upstream) fornece os dados descritivos dos produtos/SKUs. Diversos consumidores (Downstream) precisam ler essas informações (nome, imagem, atributos).
    - **Integração:** Catálogo expõe projeções de leitura otimizadas (ex: ProjecaoDetalheProduto, ProjecaoListagemProduto) através de uma API de consulta (GraphQL/REST) ou acesso direto a um datastore de leitura (ex: Elasticsearch, cache Redis). Isso constitui seu **OHS** para consultas. A estrutura dessas projeções/respostas é a **PL**. Os consumidores dependem dessa PL.
  + **(Seller) --- [OHS(Events), PL] ---> (Catalog):**
    - **Relação:** Se vendedores gerenciam seus próprios produtos, o BC Seller (Upstream) publica eventos como VendedorAtivado ou VendedorSuspenso. O Catálogo (Downstream) consome esses eventos (**PL** via **OHS** de eventos) para habilitar/desabilitar a capacidade daquele vendedor gerenciar produtos ou para marcar produtos como ativos/inativos. Catálogo depende do Seller (C/S).
* **Relações do IAM (Identidade e Acesso):**
  + **(IAM) --- [OHS(API/JWT/Context), PL] ---> (TODOS os outros BCs, praticamente):**
    - **Relação:** IAM (Upstream) é a fonte da verdade para identidade de usuário e tenant. Quase todos os outros BCs (Downstream) dependem dele para obter o contexto (UserId, TenantId, Papéis/Permissões) necessário para autorização e para filtrar dados por tenant.
    - **Integração:** Pode ocorrer de várias formas:
      1. Via JWT: O token emitido pelo Logto (gerenciado conceitualmente pelo IAM) carrega UserId, TenantId, talvez papéis básicos (**PL** no formato do token). Serviços validam o token e extraem o contexto.
      2. Consulta Direta: Serviços consultam uma API do IAM (**OHS**) para obter detalhes do usuário ou permissões.
      3. Context Propagation: O contexto é propagado entre chamadas internas (ex: via cabeçalhos gRPC/HTTP).
    - **Considerações:** Uma dependência fundamental. A forma como o contexto é compartilhado (JWT é comum) define a **PL**.
  + **(IAM) --- [OHS(Events), PL] ---> (Seller):**
    - **Relação:** IAM (Upstream) publica UsuarioRegistrado (com tipo Vendedor). Seller (Downstream) consome (**PL** via **OHS** de eventos) para iniciar o fluxo de onboarding da entidade vendedora. Seller depende do IAM (C/S).
  + **(IAM) --- [OHS(Events), PL] ---> (Notification):**
    - **Relação:** IAM (Upstream) publica eventos como UsuarioRegistrado (para boas-vindas), SenhaRedefinidaSolicitada. Notification (Downstream) consome (**PL** via **OHS** de eventos). Notification depende do IAM (C/S).
* **Relações da Configuração do Tenant (Tenant Config):**
  + **(Tenant Config) --- [OHS(API/Cache), PL] ---> (Muitos BCs: Payment, Pricing, Fulfillment, UI Layer, etc.):**
    - **Relação:** Tenant Config (Upstream) fornece configurações específicas do tenant. Vários BCs (Downstream) dependem desses dados para adaptar seu comportamento.
    - **Integração:** Geralmente via leitura de uma projeção de leitura cacheada (ProjecaoConfiguracaoAtivaTenant) exposta como **OHS** (API ou lookup direto no cache Redis). A estrutura dessa projeção é a **PL**. Os BCs consumidores são tipicamente **Conformist** ou usam uma **ACL** muito simples. A dependência é C/S.
  + **(IAM) --- [OHS(Events), PL] ---> (Tenant Config):**
    - **Relação:** IAM (Upstream) publica TenantCriado. Tenant Config (Downstream) consome (**PL** via **OHS** de eventos) para inicializar um registro de configuração padrão para o novo tenant. Tenant Config depende do IAM (C/S).
* **Relações do Gerenciamento de Vendedor (Seller):** (Já vimos Seller -> Catalog)
  + **(Seller) --- [OHS(API/Event?), PL] ---> (IAM):**
    - **Relação:** Seller (Upstream) precisa associar um VendedorId a um ou mais UserIds.
    - **Integração:** Pode enviar um comando para o IAM (AssociarUsuarioAoVendedor) ou o IAM pode ler uma projeção do Seller. O fluxo exato depende da implementação, mas Seller "informa" o IAM. Define uma **PL** para essa associação.
  + **(Seller) --- [OHS(API/Event), PL] ---> (BC Financeiro/Payout - *Não detalhado, mas implícito*):**
    - **Relação:** Seller (Upstream) fornece a identidade do vendedor e a referência segura dos dados bancários. O BC Financeiro (Downstream) consome esses dados (**PL** via **OHS**) para realizar os repasses. Financeiro depende do Seller (C/S).
  + **(Seller) --- [OHS(Projections), PL] ---> (Reviews):**
    - **Relação:** Seller (Upstream) fornece o contexto do vendedor. Reviews (Downstream) lê projeções públicas do Seller (via **OHS/PL**) para exibir informações no contexto da avaliação do vendedor e associa a avaliação ao VendedorId. Reviews depende do Seller (C/S).
* **Relações das Avaliações (Reviews):**
  + **(Reviews) --- [OHS(API/Query/Events), PL] ---> (Catalog / Seller / UI Layer):**
    - **Relação:** Reviews (Upstream) publica métricas agregadas (MediaEntidadeAtualizada) e expõe projeções de avaliações aprovadas (ProjecaoAvaliacoesPublicadas). Outros BCs/UI (Downstream) consomem esses dados (**PL** via **OHS** de eventos ou API de consulta).
  + **(Order) --- [OHS(API/Query), PL] ---> (Reviews):**
    - **Relação:** Reviews (Downstream) precisa validar a elegibilidade do avaliador consultando o histórico de pedidos. Reviews depende do Order (C/S) para essa validação. Order expõe seu histórico via **OHS/PL** (API de consulta).
  + **(Catalog / Seller) --- [OHS(API/Query), PL] ---> (Reviews):**
    - **Relação:** Reviews (Downstream) precisa identificar o Produto ou Vendedor ao qual associar a avaliação. Reviews depende do Catálogo/Seller (Upstream) para obter esses IDs e talvez nomes. Catálogo/Seller expõem via **OHS/PL**.
* **Relações das Notificações (Notification):** (Já vimos Order/IAM -> Notification)
  + **(Vários BCs Upstream) --- [OHS(Events), PL] ---> (Notification):**
    - **Relação:** Notification (Downstream) atua como um consumidor central de eventos de negócio (**PL**) publicados por muitos outros BCs (Upstream) através de seus **OHS** de eventos.
    - **Integração:** Geralmente **Conformist** ou com **ACLs** simples para extrair os dados necessários dos eventos e passá-los aos templates.
* **Relações da Precificação (Pricing):** (Já vimos Cart -> Pricing, Catalog -> Pricing)
  + **(Pricing) --- [OHS(API/Query), PL] ---> (Order / UI Layer):**
    - **Relação:** Pricing (Upstream) fornece os preços calculados quando solicitado. Order (para "congelar" o preço no pedido) e a UI (para exibição) são Clientes/Downstream.
    - **Integração:** Via chamada ao **OHS** (API de cálculo) do Pricing, consumindo sua **PL**. Dependência C/S.
* **Relações do Inventário (Inventory):** (Já vimos Order -> Inventory, Catalog -> Inventory)
  + **(Inventory) --- [OHS(API/Cache/Query), PL] ---> (UI Layer / Cart / Catalog):**
    - **Relação:** Inventory (Upstream) fornece informações sobre disponibilidade de estoque. A UI, Carrinho e Catálogo (Downstream) consomem essa informação (**PL**) via **OHS** (leitura da projeção cacheada ou API). Dependência C/S.
* **Relações da Entrega (Fulfillment):** (Já vimos Order -> Fulfillment)
  + **(Fulfillment) --- [OHS(Events), PL] ---> (Notification):**
    - **Relação:** Fulfillment (Upstream) publica eventos de status (**PL**) via seu **OHS** de eventos. Notification (Downstream) consome esses eventos. C/S fraco.
  + **(Tenant Config) --- [OHS(API/Cache), PL] ---> (Fulfillment):**
    - **Relação:** Fulfillment (Downstream) precisa ler a referência das credenciais das transportadoras configuradas pelo tenant. Fulfillment depende do Tenant Config (Upstream) via seu **OHS/PL** (cache/API). C/S.

**5. Resumo dos Padrões e Visão Geral**

* **Predominância de Customer-Supplier:** Muitas relações são C/S, o que é natural e esperado.
* **Comunicação via Eventos (OHS/PL):** A comunicação assíncrona via eventos publicados pelos BCs (sua PL exposta via OHS) é amplamente utilizada, promovendo baixo acoplamento (ex: Order -> Notification, Catalog -> Inventory/Pricing).
* **Comunicação via APIs (OHS/PL):** APIs síncronas (REST/gRPC) ou acesso a projeções/caches (OHS) são usados onde a informação é necessária mais imediatamente (ex: Cart -> Pricing, UI -> Catalog/Inventory/Reviews). A estrutura da resposta/projeção é a PL.
* **Anticorruption Layers (ACL):** Recomendadas em pontos críticos onde um BC Downstream precisa se proteger de mudanças no Upstream ou traduzir modelos (ex: Sagas no Order consumindo eventos de Payment/Inventory/Fulfillment, Cart consumindo resposta do Pricing).
* **Conformist:** Utilizado em casos mais simples onde o Downstream apenas reage diretamente aos dados/eventos do Upstream (ex: Notification consumindo eventos, Inventory/Pricing consumindo eventos básicos do Catalog).
* **Ausência (Intencional) de Shared Kernel / Partnership:** Para maximizar a autonomia dos BCs (visando implantações independentes como microsserviços), evitamos compartilhar código de domínio central (Shared Kernel) ou criar dependências mútuas fortes que exigiriam coordenação constante (Partnership).

**6. Representação Visual (Conceitual - Atualizada)**

graph LR
 %% Core BCs
 subgraph Identity & Config
 IAM -- "TenantCriado (PL)" --> TenantConfig;
 TenantConfig -- "Config Ativa (OHS/PL)" --> Payment;
 TenantConfig -- "Config Ativa (OHS/PL)" --> Pricing;
 TenantConfig -- "Config Ativa (OHS/PL)" --> Fulfillment;
 TenantConfig -- "Config Ativa (OHS/PL)" --> UILayer[UI Layer];

 IAM -- "UsrReg (Vendedor) (PL)" --> Seller;
 Seller -- "Assoc User (OHS/PL)" --> IAM;
 end

 subgraph Catalog & Seller
 Catalog -- "SKU/Prod Events (PL)" --> Inventory;
 Catalog -- "SKU/Prod Events (PL)" --> Pricing;
 Catalog -- "Projeções (OHS/PL)" --> UILayer;
 Catalog -- "Projeções (OHS/PL)" --> Cart;
 Catalog -- "Projeções (OHS/PL)" --> Reviews;

 Seller -- "Status Events (PL)" --> Catalog;
 Seller -- "Projeções (OHS/PL)" --> Reviews;
 Seller -- "Dados Repasse (PL)" --> FinanceBC[(Finance/Payout BC)];
 end

 subgraph Order Flow
 Cart -- "CheckoutIniciado (PL)" --> Order;
 Order -- "C/S, ACL?" --> Payment;
 Order -- "C/S, ACL?" --> Inventory;
 Order -- "C/S, ACL?" --> Fulfillment;
 Cart -- "C/S, API" --> Pricing;

 Order -- "Histórico (OHS/PL)" --> Reviews;
 Order -- "Valida Compra (C/S)" --> Reviews;
 end

 subgraph Feedback & Comms
 Order -- "Status Events (PL)" --> Notification;
 Payment -- "Status Events (PL)" --> Notification;
 Fulfillment -- "Status Env Events (PL)" --> Notification;
 IAM -- "Auth Events (PL)" --> Notification;
 Inventory -- "Estoque Baixo (PL)" --> Notification;
 Reviews -- "Nova Avaliação (PL)" --> Notification;

 Reviews -- "Métricas/Avaliações (OHS/PL)" --> UILayer;
 end

 %% Cross-BC Dependencies (General)
 IAM -- "Contexto (JWT/API - PL)" --> Order;
 IAM -- "Contexto (JWT/API - PL)" --> Cart;
 IAM -- "Contexto (JWT/API - PL)" --> Reviews;
 IAM -- "Contexto (JWT/API - PL)" --> Seller;
 %% ... (Implicit dependency on IAM from most others)

 %% Style for key BCs
 style Order fill:#ccf,stroke:#333,stroke-width:2px;
 style Catalog fill:#cfc,stroke:#333,stroke-width:2px;
 style IAM fill:#fec,stroke:#333,stroke-width:2px;
 style TenantConfig fill:#fec,stroke:#333,stroke-width:2px;

*(Nota: Este diagrama Mermaid é uma representação ainda mais complexa e conceitual das interconexões. Um diagrama real seria mais elaborado).*