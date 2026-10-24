**Bounded Context - Configuração do Tenant (Tenant Configuration)**

**1. Definição Descritiva Refinada**

* **Propósito Central:** Este Bounded Context (BC) é o centro nevrálgico para a personalização e adaptação da plataforma por cada tenant individual. Ele encapsula todas as configurações que distinguem uma instância "white label" de outra, permitindo que os tenants moldem a aparência, o comportamento e as funcionalidades disponíveis em seu próprio marketplace. É o motor por trás da promessa "White Label - Multi Tenant".
* **Responsabilidades Principais:**
  + **Gestão da Aparência (Theming):** Controla os elementos visuais que definem a marca do tenant, como logotipos, esquemas de cores (primária, secundária, etc.), tipografia e a capacidade de injetar CSS customizado para ajustes finos.
  + **Gestão de Domínios:** Permite que tenants associem e gerenciem seus próprios nomes de domínio (ex: loja.minhamarca.com) à sua instância do marketplace, incluindo o processo de validação de propriedade do domínio (geralmente via DNS).
  + **Gestão de Funcionalidades (Feature Flags/Toggles):** Atua como um painel de controle para habilitar ou desabilitar módulos ou capacidades específicas da plataforma para um determinado tenant. Exemplos: Módulo de Cupons, Sistema de Avaliação de Produtos, Integração com Marketplaces Externos, Programa de Fidelidade, Tipos específicos de Produtos (ex: digitais, serviços).
  + **Gestão de Regras de Negócio Parametrizáveis:** Permite aos tenants ajustar parâmetros que influenciam o comportamento do sistema para sua loja, dentro de limites predefinidos pela plataforma. Exemplos: Moeda padrão e moedas aceitas, percentual de comissão base para vendedores (em marketplaces multi-vendedor), valor mínimo para pedidos, configurações de impostos padrão, políticas de frete básicas.
  + **Gestão Segura de Credenciais Externas:** Facilita a configuração segura de integrações com serviços de terceiros, armazenando (de forma segura, preferencialmente referenciando um Vault) as chaves de API, segredos ou outras credenciais necessárias (ex: credenciais de gateways de pagamento como Stripe/PagSeguro, chaves de API de serviços de entrega como Correios/MelhorEnvio, chaves de API de serviços de email marketing).
* **Limites e Exclusões:**
  + **Não é IAM:** Não lida com a criação do tenant em si, nem com usuários ou papéis (isso é do BC de Identidade e Acesso). Ele assume que o TenantId fornecido é válido.
  + **Não é Catálogo/Pedido/Pagamento:** Não gerencia os dados operacionais do marketplace (produtos, pedidos, estoque, transações).
  + **Não Define Features Globais:** A existência de uma feature (ex: "Cupons") é definida pela plataforma; este BC apenas controla se ela está *ligada* ou *desligada* para um tenant.
* **Importância na Multi-Tenancy/White Label:** Essencial. É o que permite que a *mesma* base de código e infraestrutura multi-tenant se apresente e se comporte de maneiras distintas para cada cliente (tenant). Outros BCs consultarão este contexto (ou suas projeções cacheadas) constantemente para adaptar sua lógica e UI.

**2. Event Storming Focado (Exemplos de Fluxos Internos)**

* **Fluxo: Tenant Configura Gateway de Pagamento Padrão**
  + SelecionarGatewayPagamento 🟦 (Admin Tenant 🧍, escolhe "Gateway X" na UI)
  + SalvarCredenciaisGateway 🟦 (Admin Tenant 🧍, insere API Key e Secret Key na UI)
    - *Política Interna:* Antes de salvar, interagir com ServicoDeSegredos ⬜ (ex: Hashicorp Vault) para armazenar as credenciais.
  + CredenciaisArmazenadasNoVault 🟧 (Evento interno do adaptador do Vault)
  + RegistrarReferenciaCredencial 🟦 (Comando para Aggregate ConfiguracaoTenant 🟨) -> contém tenantId, servicoKey="GW\_PAG\_X", referenciaVault="path/to/secret"
  + ReferenciaCredencialExternaAtualizada 🟧 (Publicado)
  + DefinirGatewayPagamentoPadrao 🟦 (Comando para Aggregate ConfiguracaoTenant 🟨) -> contém tenantId, gatewayKey="GW\_PAG\_X"
  + RegraDeNegocioTenantAtualizada 🟧 (Publicado, com regraKey="GATEWAY\_PAGAMENTO\_PADRAO", valor="GW\_PAG\_X")
* **Fluxo: Admin da Plataforma Habilita Feature "Avaliacoes" para um Tenant**
  + HabilitarFeatureParaTenant 🟦 (Admin Plataforma 🧍 via CLI/UI Admin Global) -> Comando para ConfiguracaoTenant 🟨
  + FeatureStatusAlterado 🟧 (Publicado, contém tenantId, featureKey: "AVALIACOES", ativo: true)
  + *Consumo:* BC de Avaliações pode começar a aceitar/exibir avaliações para produtos desse tenant. UI do tenant pode mostrar a seção de avaliações.
* **Fluxo: Tenant Atualiza Cor Primária**
  + AtualizarCorPrimaria 🟦 (Admin Tenant 🧍 via Color Picker na UI) -> Comando para ConfiguracaoTenant 🟨
  + TemaTenantAtualizado 🟧 (Publicado, contém tenantId e a estrutura do tema atualizada com a nova cor)
  + *Consumo:* Frontend busca a configuração atualizada (provavelmente do cache Redis) e aplica a nova cor via CSS variables ou similar.

**3. Event Modelling (Exemplos de Eventos Chave)**

* TemaTenantAtualizado (v1)
  {
   "eventId": "uuid-evento-4",
   "eventType": "TemaTenantAtualizado",
   "eventVersion": "1",
   "timestamp": "2025-04-22T12:05:00Z",
   "aggregateId": "tenant-uuid-123", // TenantId
   "correlationId": "uuid-correlacao-jkl",
   "payload": {
   "tenantId": "tenant-uuid-123",
   "configuracaoTema": {
   "cores": {
   "primaria": "#FF5733", // Nova cor
   "secundaria": "#33FF57",
   "fundo": "#FFFFFF",
   "texto": "#333333"
   },
   "logoUrl": "https://cdn.tenant.com/logo.png",
   "faviconUrl": "https://cdn.tenant.com/favicon.ico",
   "cssCustomizadoUrl": null
   }
   }
  }
* FeatureStatusAlterado (v1)
  {
   "eventId": "uuid-evento-5",
   "eventType": "FeatureStatusAlterado",
   "eventVersion": "1",
   "timestamp": "2025-04-22T12:06:00Z",
   "aggregateId": "tenant-uuid-456", // Outro TenantId
   "correlationId": "uuid-correlacao-mno",
   "payload": {
   "tenantId": "tenant-uuid-456",
   "featureKey": "PROGRAMA\_FIDELIDADE", // Chave da feature
   "ativo": true // Novo status
   }
  }
* ReferenciaCredencialExternaAtualizada (v1)
  {
   "eventId": "uuid-evento-6",
   "eventType": "ReferenciaCredencialExternaAtualizada",
   "eventVersion": "1",
   "timestamp": "2025-04-22T12:07:00Z",
   "aggregateId": "tenant-uuid-123", // TenantId
   "correlationId": "uuid-correlacao-pqr",
   "payload": {
   "tenantId": "tenant-uuid-123",
   "servicoExternoKey": "GATEWAY\_PAGAMENTO\_STRIPE", // Identificador do serviço/credencial
   "referenciaSegredo": "arn:aws:secretsmanager:us-east-1:123456789012:secret:tenant123/stripe-key-AbCdEf" // Referência ao segredo no Vault/Secrets Manager
   }
  }

**4. Modelagem dos Componentes (DDD Tático)**

* **Aggregates:**
  + **ConfiguracaoTenant Aggregate:** (Mantém a ideia de um aggregate por tenant para coesão)
    - **Root:** ConfiguracaoTenant (Entity)
    - **ID:** TenantId (Value Object, o mesmo ID do IAM BC).
    - **Estruturas Internas (Value Objects ou Entidades dentro do Aggregate):**
      * Tema (VO complexo: Cores (VO), LogoUrl (VO), FaviconUrl (VO), CssUrl (VO?)).
      * Dominio (Entity dentro do aggregate? Ou VO? Contém NomeDominio (VO), StatusVerificacao (Enum VO), DataVerificacao (VO?)). Poderia ser uma coleção Set<Dominio>.
      * FeatureToggles (VO tipo Map<FeatureKey, Boolean>, onde FeatureKey é um VO/Enum).
      * RegrasNegocio (VO tipo Map<RegraKey, RegraValor>, onde RegraKey é VO/Enum e RegraValor pode ser um VO polimórfico ou String/JSON para flexibilidade).
      * CredenciaisExternas (VO tipo Map<ServicoExternoKey, ReferenciaSegredo>, onde ServicoExternoKey é VO/Enum e ReferenciaSegredo é um VO representando o path no Vault).
    - **Invariantes:** Validação de formatos (cores hex, URLs válidas). Chaves (FeatureKey, RegraKey, ServicoExternoKey) devem pertencer a um conjunto conhecido/válido. Status de domínio segue fluxo lógico (Pendente -> Verificado/Falhou). Garantir que NUNCA armazene credenciais diretamente, apenas a referência segura.
    - **Comandos:** AtualizarTema, AdicionarDominio, ConfirmarVerificacaoDominio, FalharVerificacaoDominio, AlterarStatusFeature, DefinirRegraNegocio, SalvarReferenciaCredencialExterna.
    - **Eventos:** TemaTenantAtualizado, DominioAdicionado, DominioVerificado, VerificacaoDominioFalhou, FeatureStatusAlterado, RegraDeNegocioTenantAtualizada, ReferenciaCredencialExternaAtualizada.
* **Value Objects:** TenantId, Tema, Cores, Url, NomeDominio, StatusVerificacaoDominio, FeatureKey, RegraKey, ServicoExternoKey, ReferenciaSegredo, RegraValor (pode ser complexo).
* **Entities (Potenciais, dentro do Aggregate):** Dominio (se tiver ciclo de vida próprio ou mais atributos).
* **CQRS:**
  + **Write Side:** Aggregate ConfiguracaoTenant processa comandos, valida invariantes, interage com serviço de segredos se necessário, e publica eventos no RabbitMQ. **MongoDB** continua sendo uma escolha forte aqui devido à natureza semi-estruturada e evolutiva das configurações. Cada tenant teria seu documento ConfiguracaoTenant.
  + **Read Side (Projections):**
    - **ProjecaoConfiguracaoTenantAtiva (Cache):** Essencial. Contém o estado *atual* e *completo* da configuração do tenant (tema, features, regras, refs de credenciais). Armazenada em **Redis**, chaveada por TenantId. Atualizada pelos event handlers que consomem os eventos deste BC. É a fonte primária de consulta para outros BCs e para a UI. TTL (Time-To-Live) no cache deve ser considerado.
    - **ProjecaoMapeamentoDominioTenant:** Uma projeção específica para infraestrutura (ex: Traefik) mapeando nomes de domínio para TenantId. Pode ser uma tabela simples em **PostgreSQL** ou um K/V no **Redis/MongoDB**. Atualizada por DominioVerificado, DominioRemovido.

**5. Sagas / Process Managers**

* **SagaVerificacaoDominio:**
  + **Trigger:** Evento DominioAdicionado.
  + **Orquestração:** Coordena a validação assíncrona do DNS, interagindo com serviços externos de verificação ou agendando tarefas. Ao receber a confirmação (sucesso/falha), envia o comando correspondente (ConfirmarVerificacaoDominio / FalharVerificacaoDominio) de volta ao Aggregate. Pode também disparar a atualização da configuração do proxy (ProjecaoMapeamentoDominioTenant).
* **SagaProvisionamentoInfraTenant (Potencial):**
  + **Trigger:** Evento TenantProntoParaConfiguracao (do IAM) ou ConfiguracaoTenantInicializada.
  + **Orquestração:** Poderia coordenar tarefas de infraestrutura necessárias para um novo tenant, como criar entradas no proxy (Traefik), configurar buckets de armazenamento específicos, etc., com base nas configurações iniciais.

**6. Considerações Adicionais**

* **Performance do Cache:** A ProjecaoConfiguracaoTenantAtiva no Redis será um ponto de acesso crítico. Monitorar o hit rate e a latência é fundamental. Estratégias como cache L2 na aplicação (ex: Caffeine no Quarkus) podem complementar o Redis.
* **Gerenciamento de Segredos:** A integração com o Vault/Secrets Manager deve ser robusta e segura. O serviço deste BC precisa ter as permissões corretas para escrever/ler referências, e os serviços que *usam* as credenciais (ex: BC de Pagamento) precisam ter permissão para ler os segredos *reais* do Vault usando a referência.
* **Evolução das Configurações:** Usar MongoDB no write-side facilita adicionar novas configurações (features, regras) sem migrações de esquema complexas. A projeção de leitura precisa ser adaptável.
* **Interface de Administração:** Os Admins de Tenant precisarão de uma UI clara e segura para gerenciar essas configurações. Os Admins da Plataforma podem precisar de uma visão geral ou capacidade de sobrescrever/habilitar features.