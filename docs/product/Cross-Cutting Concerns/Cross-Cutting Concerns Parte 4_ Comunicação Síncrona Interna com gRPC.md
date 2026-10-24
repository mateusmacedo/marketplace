**Cross-Cutting Concerns Parte 4: Comunicação Síncrona Interna com gRPC**

**1. Introdução: A Necessidade de Comunicação Síncrona**

Nossa arquitetura é primariamente **Orientada a Eventos (EDA)**, favorecendo a comunicação **assíncrona** via RabbitMQ. Isso promove baixo acoplamento, resiliência e escalabilidade. No entanto, existem cenários específicos dentro de um sistema distribuído onde uma interação **síncrona** – onde o serviço chamador espera por uma resposta imediata do serviço chamado para poder continuar seu próprio fluxo – é necessária ou mais apropriada.

Exemplos em nossa arquitetura:

* O BC Carrinho precisa do preço calculado *agora* para exibir ao usuário.
* Um serviço precisa de uma verificação de permissão *imediata* do IAM BC antes de prosseguir com uma operação crítica.

Embora pudéssemos usar REST para essas chamadas internas, optamos (conforme sua solicitação inicial) por usar **gRPC** quando a comunicação síncrona interna for pertinente.

**2. Por Que gRPC para Comunicação Interna (vs. REST)?**

gRPC (gRPC Remote Procedure Calls) oferece várias vantagens sobre REST/JSON para comunicação síncrona *serviço-a-serviço*:

* **Performance:**
  + **HTTP/2:** Utiliza HTTP/2 por padrão, que permite multiplexação (múltiplas chamadas sobre uma única conexão TCP), compressão de cabeçalhos (HPACK) e comunicação bidirecional, resultando em menor latência e melhor utilização da rede.
  + **Protocol Buffers (Protobuf):** Usa Protobuf como formato padrão de serialização. Mensagens Protobuf são binárias, mais compactas e significativamente mais rápidas para serializar e desserializar em comparação com JSON baseado em texto.
* **Contrato Forte e Tipagem (IDL):**
  + **.proto Files:** As interfaces de serviço e as estruturas de mensagem são definidas rigorosamente em arquivos .proto (Interface Definition Language). Isso cria um contrato claro entre cliente e servidor.
  + **Geração de Código:** Ferramentas geram automaticamente código para clientes (stubs) e servidores (interfaces base) em diversas linguagens (incluindo Java via quarkus-grpc). Isso garante type safety em tempo de compilação e simplifica o desenvolvimento.
  + **Evolução de Schema:** Protobuf tem regras bem definidas para evolução de schema de forma retrocompatível e, em alguns casos, prospectiva, facilitando a gestão de versões.
* **Streaming:** Suporte nativo para diferentes modelos de streaming (Unary - request/response simples, Server-streaming, Client-streaming, Bidirectional-streaming), que podem ser úteis para certos casos de uso (embora nosso foco aqui seja síncrono unary).
* **Deadlines e Cancelamento:** Mecanismos integrados para que clientes especifiquem um prazo máximo para receber a resposta (deadline) e para propagar cancelamentos.

**3. Onde Usar gRPC em Nossa Arquitetura (Casos de Uso Identificados)**

Aplicaremos gRPC nos seguintes pontos de interação síncrona interna:

* **Consulta de Autorização (Serviços -> IAM BC):**
  + **Necessidade:** Serviços como Pedidos, Catálogo, etc., precisam verificar permissões *imediatamente* antes de executar ações sensíveis. Uma chamada assíncrona não funcionaria aqui.
  + **Interface (Exemplo .proto):**
    service IamAuthorizationService {
     rpc CheckPermission (CheckPermissionRequest) returns (CheckPermissionResponse);
    }
    message CheckPermissionRequest {
     string user\_id = 1;
     string tenant\_id = 2;
     string permission\_key = 3;
    }
    message CheckPermissionResponse {
     bool granted = 1;
    }
  + **Justificativa gRPC:** Performance é crítica para não adicionar latência significativa à requisição original do usuário. A tipagem forte garante clareza na requisição/resposta.
* **Cálculo de Preço (Cart/Checkout -> Pricing BC):**
  + **Necessidade:** O Carrinho ou o início do Checkout precisam exibir o preço calculado atualizado para o usuário *agora*.
  + **Interface (Exemplo .proto):**
    service PricingCalculationService {
     rpc CalculateCartPrice (CalculateCartPriceRequest) returns (CalculateCartPriceResponse);
    }
    message CalculateCartPriceRequest {
     string tenant\_id = 1;
     string user\_id = 2; // Opcional, para preços/promoções específicas
     repeated CartItem items = 3;
     string coupon\_code = 4; // Opcional
    }
    message CartItem {
     string sku\_id = 1;
     int32 quantity = 2;
    }
    message CalculateCartPriceResponse {
     // Estrutura detalhada do preço calculado, similar ao evento PrecoCarrinhoCalculado
     double subtotal\_original = 1;
     double desconto\_total = 2;
     double valor\_final = 3;
     string moeda = 4;
     // ... outros detalhes ...
    }
  + **Justificativa gRPC:** Performance é importante para a experiência do usuário no carrinho/checkout. Protobuf pode ser eficiente para serializar a lista de itens.
* **Consulta de Disponibilidade de Estoque (Tempo Real - Se Necessário):**
  + **Necessidade:** Em cenários onde uma verificação de estoque *altamente consistente* e imediata é crucial *antes* de uma ação (talvez complementar ao lock otimista). *Nota: Frequentemente, a abordagem EDA com eventos e compensação é preferível, mas se a consistência síncrona for um requisito absoluto para um caso específico.*
  + **Interface (Exemplo .proto):**
    service InventoryQueryService {
     rpc CheckSkuAvailability (CheckSkuAvailabilityRequest) returns (CheckSkuAvailabilityResponse);
    }
    message CheckSkuAvailabilityRequest {
     string tenant\_id = 1;
     string sku\_id = 2;
     int32 required\_quantity = 3;
    }
    message CheckSkuAvailabilityResponse {
     bool available = 1;
     int32 quantity\_available = 2; // Informativo
    }
  + **Justificativa gRPC:** Baixa latência para uma verificação rápida.
* **Consulta de Configuração Específica (Fallback do Cache):**
  + **Necessidade:** Se um serviço precisa de uma configuração *absolutamente atualizada* do Tenant Config e não pode confiar apenas no cache da projeção (caso raro).
  + **Interface (Exemplo .proto):**
    service TenantConfigQueryService {
     rpc GetTenantFeatureFlag (GetTenantFeatureFlagRequest) returns (GetTenantFeatureFlagResponse);
    }
    message GetTenantFeatureFlagRequest {
     string tenant\_id = 1;
     string feature\_key = 2;
    }
    message GetTenantFeatureFlagResponse {
     bool enabled = 1;
    }
  + **Justificativa gRPC:** Simplicidade e performance para uma consulta interna direta.

**4. Onde NÃO Usar gRPC (Reforçar o Padrão EDA/Assíncrono)**

É crucial limitar o uso de gRPC (ou qualquer comunicação síncrona) para evitar reintroduzir o acoplamento forte que estamos tentando minimizar com EDA. **NÃO usar gRPC para:**

* **Notificar outros BCs sobre mudanças de estado:** Use eventos via RabbitMQ (ex: Pedido -> Notificação, Catálogo -> Inventário).
* **Disparar processos em outros BCs que não exigem resposta imediata:** Use comandos assíncronos via RabbitMQ (ex: Pedido -> Pagamento, Pedido -> Entrega). A Saga orquestradora lida com as respostas (eventos) quando chegarem.
* **Propagar dados para atualização de projeções de leitura:** Use consumo de eventos via RabbitMQ. Ler diretamente do serviço de escrita via gRPC para popular uma leitura é um anti-pattern CQRS.

**5. Detalhes de Implementação (Com Quarkus gRPC)**

* **Definição dos .proto:** Criar arquivos .proto versionados definindo os services e messages para cada interação gRPC. Armazená-los em um local acessível (ex: repositório Git dedicado, ou dentro do projeto do serviço provedor).
* **Geração de Código:** A extensão quarkus-grpc no Maven/Gradle cuidará de gerar as classes Java (stubs de cliente, interfaces de serviço) automaticamente durante o build.
* **Implementação do Servidor:** No serviço que *provê* a funcionalidade (ex: IAM BC), criar uma classe que implementa a interface base gerada (ex: IamAuthorizationServiceImpl extends IAMServiceGrpc.IAMServiceImplBase). Anotar com @GrpcService para que o Quarkus a exponha. Usar injeção de dependência CDI normalmente.
* **Implementação do Cliente:** No serviço que *consome* a funcionalidade (ex: Order Service), injetar o stub do cliente gRPC gerado usando a anotação @GrpcClient("nome-do-servico"). O nome-do-servico será mapeado para a localização do servidor gRPC via application.properties (usando descoberta de serviço via Kubernetes, Consul ou URL fixa). Ex: @Inject @GrpcClient("iam-service") IAMServiceGrpc.IAMServiceBlockingStub iamClient; (para chamadas bloqueantes) ou MutinyIAMServiceGrpc.MutinyIAMServiceStub (para reativas).
* **Tratamento de Erros:** Mapear exceções de negócio no servidor para códigos de status gRPC apropriados (ex: Status.NOT\_FOUND, Status.PERMISSION\_DENIED, Status.INVALID\_ARGUMENT, Status.INTERNAL). O cliente deve tratar esses códigos de status.
* **Deadlines:** O cliente DEVE configurar deadlines razoáveis em suas chamadas gRPC para evitar ficar esperando indefinidamente se o servidor demorar ou não responder. Ex: iamClient.withDeadlineAfter(500, TimeUnit.MILLISECONDS).checkPermission(...). O servidor também pode ter timeouts configurados.
* **Segurança (mTLS):** Configurar mTLS (Mutual TLS) entre os serviços para garantir que apenas clientes autorizados possam chamar os endpoints gRPC internos e que a comunicação seja criptografada. Quarkus suporta configuração de TLS para gRPC.
* **Load Balancing:** Se houver múltiplas instâncias do serviço provedor (ex: IAM BC), o cliente gRPC injetado pelo Quarkus pode ser configurado para fazer balanceamento de carga client-side (ex: round-robin) sobre os endereços descobertos.

**6. Trade-offs e Considerações**

* **Acoplamento Temporal:** A principal desvantagem. O chamador fica bloqueado esperando a resposta. Se o serviço chamado estiver lento ou indisponível, isso afeta diretamente o chamador. Padrões de resiliência como **Circuit Breaker** (disponível no Quarkus via MicroProfile Fault Tolerance ou Resilience4j) são essenciais em torno das chamadas gRPC críticas.
* **Complexidade:** Adiciona a necessidade de gerenciar os arquivos .proto e o código gerado. O ferramental é bom, mas é uma camada adicional.
* **Observabilidade:** Embora gRPC seja suportado pelo OpenTelemetry para tracing, a depuração de payloads binários Protobuf na rede pode ser menos direta que JSON (ferramentas como grpcurl, Wireshark com dissecadores Protobuf ajudam).
* **Evolução de Schema:** Requer disciplina para manter a compatibilidade ao alterar os .proto files, especialmente em um ambiente com múltiplos consumidores.

**Conclusão:** gRPC será nossa escolha padrão para comunicação síncrona interna necessária, devido às suas vantagens de performance e tipagem forte, mas seu uso será criterioso e limitado aos casos onde a natureza assíncrona da EDA não é adequada.