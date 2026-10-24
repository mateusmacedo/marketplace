**Context Mapping (Parte 1: Introdução e Relações Centrais)**

**1. Introdução ao Context Mapping**

O Context Mapping é uma técnica crucial do Domain-Driven Design (DDD) Estratégico. Seu objetivo é **visualizar e documentar explicitamente as relações e os padrões de interação entre diferentes Bounded Contexts** dentro de um sistema complexo.

**Por que é importante?**

* **Clareza nas Dependências:** Torna óbvio quais BCs dependem de outros, ajudando a entender o fluxo de informações e o impacto de mudanças.
* **Estilo de Comunicação:** Define como os BCs se comunicam (via eventos, APIs síncronas, etc.) e qual o nível de acoplamento entre eles.
* **Coordenação entre Times:** Em sistemas maiores onde diferentes times podem ser donos de diferentes BCs, o mapa de contexto é fundamental para alinhar a comunicação e a colaboração.
* **Identificação de Problemas:** Ajuda a identificar anti-patterns, como um "Big Ball of Mud" (onde tudo depende de tudo) ou dependências circulares problemáticas.
* **Definição de Limites:** Reforça e, às vezes, ajuda a refinar os limites dos próprios Bounded Contexts.

**Padrões Comuns de Relacionamento (Introdução Rápida):**

Vamos introduzir alguns dos padrões que usaremos para descrever as relações. Cada relação será detalhada conforme mapeamos:

* **Customer-Supplier (Cliente-Fornecedor):** A relação mais comum. Um BC (Cliente, Downstream) consome funcionalidades ou dados de outro BC (Fornecedor, Upstream). O sucesso do Cliente depende do Fornecedor. O Fornecedor tem poder sobre a relação.
* **Conformist (Conformista):** Um BC (Downstream) adota completamente o modelo de domínio de outro BC (Upstream), sem tentar traduzir ou proteger-se. Isso simplifica a integração, mas torna o Downstream vulnerável a mudanças no Upstream.
* **Anticorruption Layer (ACL - Camada Anticorrupção):** Um BC (Downstream) cria uma camada de tradução explícita para isolar seu modelo interno das mudanças ou complexidades de um BC (Upstream) do qual depende. Protege o Downstream.
* **Open Host Service (OHS - Serviço de Host Aberto):** Um BC define um protocolo ou API bem documentada para que outros BCs (Clientes) possam acessá-lo. É a forma como um Fornecedor expõe seus serviços.
* **Published Language (PL - Linguagem Publicada):** Um formato de dados bem definido e compartilhado (geralmente eventos ou modelos de API) usado para comunicação entre BCs. É frequentemente associado a um OHS.
* **Shared Kernel (Núcleo Compartilhado):** Uma pequena parte do modelo de domínio (código, estruturas de dados) que é compartilhada e mantida conjuntamente por dois ou mais BCs. *Geralmente evitado em arquiteturas de microsserviços* devido ao alto acoplamento que introduz.
* **Partnership (Parceria):** Dois BCs (e seus times) dependem mutuamente um do outro e precisam coordenar suas mudanças de forma colaborativa. Requer sincronização constante.

**2. Bounded Contexts Identificados (Recap)**

Para referência, aqui estão os BCs que detalhamos:

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

**3. Mapeamento Inicial - Foco nas Relações do Pedido (Order)**

Vamos começar mapeando as interações do BC de Pedidos, que é um orquestrador central:

* **(Order) --- [C/S, OHS(Events), PL, ACL?] ---> (Payment)**
  + **Relação:** **Customer-Supplier (C/S)**. Order (Cliente/Downstream) depende criticamente do Payment (Fornecedor/Upstream) para processar o pagamento.
  + **Integração:** Order (via Saga) envia um comando SolicitarProcessamentoPagamento. Payment expõe esse comando como parte de seu **Open Host Service (OHS)**. Payment publica eventos (PagamentoAprovado, PagamentoFalhou) que formam sua **Published Language (PL)**. Order consome esses eventos.
  + **Considerações:** A Saga dentro de Order pode implementar uma **Anticorruption Layer (ACL)** para traduzir os eventos do Payment para o contexto interno do Pedido, protegendo-o de mudanças detalhadas na PL do Payment. A confiabilidade do Payment é crucial para o Order.
* **(Order) --- [C/S, OHS(Events), PL, ACL?] ---> (Inventory)**
  + **Relação:** **Customer-Supplier (C/S)**. Order (Downstream) depende do Inventory (Upstream) para verificar/reservar/debitar o estoque.
  + **Integração:** Order (via Saga) envia comandos como ReservarEstoqueParaPedido ou ConfirmarDebitoEstoquePedido (parte do **OHS** do Inventory). Inventory publica eventos como EstoqueReservado, DebitoConfirmado, FalhaReservaEstoquePedido (sua **PL**). Order consome esses eventos.
  + **Considerações:** Similar ao Payment. Uma **ACL** na Saga do Order pode ser útil. Inventory precisa ser confiável.
* **(Order) --- [C/S, OHS(Events), PL, ACL?] ---> (Fulfillment)**
  + **Relação:** **Customer-Supplier (C/S)**. Order (Downstream) depende do Fulfillment (Upstream) para realizar o envio físico.
  + **Integração:** Order (via Saga, após pagamento/estoque OK) envia comando SolicitarEnvioPedido (parte do **OHS** do Fulfillment). Fulfillment publica eventos de status como PedidoEnviado, PedidoEntregue (sua **PL**) que são consumidos pelo Order para atualizar seu próprio status.
  + **Considerações:** Order precisa das atualizações de status. Uma **ACL** pode simplificar os eventos de rastreio para o Order.
* **(Cart) --- [OHS(Event), PL] ---> (Order)**
  + **Relação:** O Carrinho (Cart) atua como "fornecedor" do input inicial para o Pedido (Order). A relação é primariamente através de um evento.
  + **Integração:** Cart publica o evento CheckoutIniciado, que é a **Published Language (PL)** relevante aqui e parte de seu **Open Host Service (OHS)** via eventos. Order é um consumidor (Downstream) desse evento específico para iniciar seu processo de criação.
  + **Considerações:** Order tem uma dependência na estrutura e semântica do evento CheckoutIniciado. Mudanças nesse evento exigem coordenação ou uma ACL no lado do Order. Não é uma relação C/S típica contínua, mas sim um gatilho inicial.
* **(Order) --- [OHS(Events), PL] ---> (Notification)**
  + **Relação:** Order (Upstream, neste contexto) publica eventos de seu ciclo de vida que Notification (Downstream) consome para enviar mensagens. Pode ser vista como uma relação **Customer-Supplier** fraca, onde Notification é o cliente dos eventos do Order.
  + **Integração:** Order publica eventos como PedidoCriado, PedidoPago, PedidoEnviado, PedidoCancelado (sua **PL** e **OHS** via eventos). Notification é um **Conformist** (ou tem uma ACL simples) em relação a esses eventos, usando os dados deles para acionar o envio de mensagens.
  + **Considerações:** Acoplamento baixo. Notification depende da PL do Order.
* **(Cart) --- [C/S, OHS(API), ACL?] ---> (Pricing)**
  + **Relação:** **Customer-Supplier (C/S)**. Cart (Cliente/Downstream) depende do Pricing (Fornecedor/Upstream) para obter cálculos de preço em tempo real.
  + **Integração:** Cart envia uma requisição (Comando/Query, provavelmente síncrona via API REST/gRPC) SolicitarCalculoPrecoCarrinho para o Pricing. Pricing expõe esse serviço como seu **Open Host Service (OHS)**. Pricing retorna a estrutura de preços calculada (**PL** via API).
  + **Considerações:** Geralmente um acoplamento mais forte devido à necessidade de resposta rápida para a UI. Cart pode ter uma **ACL** para mapear a resposta complexa do Pricing para o seu modelo interno (ex: SnapshotPrecoCalculado).

**4. Representação Visual (Conceitual)**

Imagine um diagrama com caixas para cada BC. As setas indicam a direção da dependência principal ou do fluxo de comando/evento principal.

graph LR
 subgraph Core Flow
 Cart -- "CheckoutIniciado (PL)" --> Order;
 Order -- "Solicitar... -> Pgto/Estoque/Envio OK? (C/S, ACL?)" --> Payment((Payment));
 Order -- "Solicitar... -> Estoque OK? (C/S, ACL?)" --> Inventory((Inventory));
 Order -- "Solicitar Envio -> Status? (C/S, ACL?)" --> Fulfillment((Fulfillment));
 Cart -- "Solicitar Preço -> Preço (C/S, API)" --> Pricing((Pricing));
 end
 subgraph Notifications
 Order -- "Eventos de Status (PL)" --> Notification((Notification));
 end

 %% Annotations (Conceptual)
 %% Order depends on Payment (U/D notation: O(D) <- P(U))
 %% Order depends on Inventory (U/D notation: O(D) <- I(U))
 %% Order depends on Fulfillment (U/D notation: O(D) <- F(U))
 %% Order depends on Cart event (U/D notation: O(D) <- C(U - for event))
 %% Cart depends on Pricing (U/D notation: C(D) <- Pr(U))
 %% Notification depends on Order events (U/D notation: N(D) <- O(U))

 %% Style for external BCs (conceptual)
 style Payment fill:#f9d,stroke:#333,stroke-width:2px;
 style Inventory fill:#f9d,stroke:#333,stroke-width:2px;
 style Fulfillment fill:#f9d,stroke:#333,stroke-width:2px;
 style Pricing fill:#f9d,stroke:#333,stroke-width:2px;
 style Notification fill:#f9d,stroke:#333,stroke-width:2px;

*(Nota: O diagrama Mermaid acima é uma representação simplificada para ilustrar a ideia).*

**Próximos Passos no Mapeamento:**

Nesta primeira parte, introduzimos o Context Mapping e mapeamos as relações críticas envolvendo os BCs de Pedido e Carrinho. Na próxima resposta (Parte 2), continuaremos o mapeamento, focando nas interações envolvendo:

* Catálogo (Catalog)
* Identidade e Acesso (IAM)
* Configuração do Tenant (Tenant Config)
* Gerenciamento de Vendedor (Seller)
* Avaliações (Reviews)
* E como eles se conectam aos BCs já mapeados e entre si.

Este início do mapeamento de contexto está claro e satisfatório? Podemos prosseguir para a Parte 2 do Context Mapping, detalhando as demais relações?