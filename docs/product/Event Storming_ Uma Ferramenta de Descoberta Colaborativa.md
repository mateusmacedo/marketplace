**Event Storming: Uma Ferramenta de Descoberta Colaborativa**

* **O que é?** É uma workshop flexível e interativo que reúne pessoas com diferentes perspectivas (desenvolvedores, especialistas de domínio, product owners, etc.) para explorar um domínio de negócio complexo. O objetivo é criar um modelo visual compartilhado do domínio, geralmente usando notas adesivas (stickies) coloridas em uma grande superfície (parede ou quadro virtual).
* **Por que usar?**
  + **Mapear o Fluxo de Negócios:** Visualiza como os eventos ocorrem ao longo do tempo e como os processos de negócio realmente funcionam.
  + **Descobrir Eventos de Domínio:** Identifica os acontecimentos mais importantes no sistema.
  + **Identificar Limites (Bounded Contexts):** Agrupamentos naturais de eventos, comandos e dados começam a surgir, sugerindo os limites dos contextos.
  + **Expor Ambiguidade e Complexidade:** Revela gargalos, regras de negócio implícitas, diferentes interpretações de termos (levando à necessidade de linguagens ubíquas distintas).
  + **Fomentar a Colaboração:** Cria um entendimento compartilhado entre a equipe técnica e os stakeholders do negócio.
* **Artefatos Principais (e suas cores típicas):**
  + 🟧 **Evento de Domínio (Domain Event):** Algo significativo que aconteceu no passado. É um fato imutável. (Ex: PedidoCriado, PagamentoRecebido, ProdutoPublicado, EstoqueAtualizado, TenantConfigurado). A espinha dorsal do Event Storming.
  + 🟦 **Comando (Command):** Uma intenção de mudar o estado do sistema, geralmente iniciada por um ator ou outro evento. (Ex: CriarPedido, ProcessarPagamento, AdicionarProduto, AjustarEstoque, SalvarConfiguracaoTenant).
  + 🧍 **Ator (Actor):** Pessoa, papel ou sistema externo que dispara um comando. (Ex: Cliente, Vendedor, AdminTenant, SistemaDePagamento, OperadorLogistico).
  + 🟩 **Modelo de Leitura / Projeção (Read Model / Projection):** Informação necessária para um Ator tomar uma decisão ou para exibir dados na UI. (Ex: ListaDeProdutosDisponiveis, DetalhesDoPedidoPendente, HistoricoDeComprasCliente, PainelDeVendasVendedor).
  + 🟨 **Agregado (Aggregate):** Um cluster de entidades e objetos de valor que são tratados como uma unidade coesa, com um raiz de agregado (Aggregate Root) responsável por proteger as regras de negócio (invariantes) dentro desse cluster. Frequentemente, comandos atuam sobre agregados para gerar eventos. (Ex: Pedido, Produto, Usuario, Tenant).
  + 🟪 **Política / Process Manager / Saga (Policy / Process Manager / Saga):** Lógica de negócio que reage a eventos, frequentemente orquestrando fluxos de trabalho que abrangem múltiplos agregados ou contextos. Pode disparar comandos em resposta a eventos. (Ex: "Quando PedidoPago, então SolicitarEnvio e NotificarCliente").
  + ⬜ **Sistema Externo (External System):** Serviço ou componente fora do nosso controle direto, mas com o qual interagimos. (Ex: GatewayDePagamentoX, ServicoDeEntregaY, API de Consulta de CEP).
  + 🗣️ **Linguagem Ubíqua (Ubiquitous Language):** O vocabulário compartilhado e rigorosamente definido dentro de um Bounded Context específico. O Event Storming ajuda a descobrir onde a linguagem diverge entre diferentes partes do domínio.

**2. O Processo (Simplificado):**

1. **Comece com os Eventos:** Espalhe os Eventos de Domínio que vêm à mente, tentando colocá-los em uma linha do tempo aproximada.
2. **Identifique Comandos e Atores:** Para cada evento, pergunte: "O que causou isso?". Adicione o Comando e o Ator correspondente antes do evento.
3. **Adicione Modelos de Leitura:** Pergunte: "Que informação o Ator precisava para disparar esse Comando?". Adicione os Modelos de Leitura necessários.
4. **Descubra Políticas e Fluxos:** Pergunte: "O que acontece *depois* que este Evento ocorre?". Isso pode revelar Políticas (regras "Sempre que X acontecer, faça Y") ou Sagas (processos mais longos).
5. **Identifique Agregados:** Procure por clusters de Comandos e Eventos que operam sobre os mesmos conceitos de negócio. Esses clusters sugerem os Agregados.
6. **Procure por Limites:** Observe onde a linguagem muda, onde há interações menos frequentes entre clusters, ou onde há diferentes especialistas de domínio. Essas "fraturas" no modelo são fortes indicadores de limites de Bounded Contexts.
7. **Refine e Itere:** Revise o quadro, discuta as ambiguidades, reorganize e refine até que um modelo coeso e compartilhado emerja.

**3. Simulação Rápida para o Marketplace (Exemplo: Fluxo de Compra)**

Imagine um quadro:

* ... ProdutoAdicionadoAoCarrinho 🟧 (Causado por AdicionarAoCarrinho 🟦 pelo Cliente 🧍, que viu DetalhesDoProduto 🟩)
* ... EnderecoDeEntregaSelecionado 🟧 (SelecionarEndereco 🟦 pelo Cliente 🧍, que viu ListaDeEnderecosSalvos 🟩)
* ... MetodoDePagamentoEscolhido 🟧 (EscolherPagamento 🟦 pelo Cliente 🧍, que viu OpcoesDePagamentoDisponiveis 🟩)
* PedidoFinalizado 🟧 (Causado por FinalizarCompra 🟦 pelo Cliente 🧍, que viu ResumoDoPedido 🟩)
  + **Política:** Quando PedidoFinalizado ➡️ Disparar ReservarEstoque 🟦 e IniciarPagamento 🟦.
* EstoqueReservado 🟧 (Resultado de ReservarEstoque 🟦 no Agregado InventarioProduto 🟨)
* PagamentoIniciado 🟧 (Resultado de IniciarPagamento 🟦, que interage com GatewayDePagamento ⬜)
* PagamentoAprovado 🟧 (Evento vindo do GatewayDePagamento ⬜)
  + **Política/Saga:** Quando PagamentoAprovado ➡️ Disparar ConfirmarPedido 🟦, DebitarEstoqueReservado 🟦, SolicitarNotaFiscal 🟦, AgendarEnvio 🟦.
* PedidoConfirmado 🟧 (Resultado de ConfirmarPedido 🟦 no Agregado Pedido 🟨)
* EstoqueDebitado 🟧 (Resultado de DebitarEstoqueReservado 🟦 no Agregado InventarioProduto 🟨)
* EnvioAgendado 🟧 (Resultado de AgendarEnvio 🟦, pode interagir com ServicoDeEntrega ⬜)
* NotaFiscalEmitida 🟧 (Vindo de um sistema fiscal ou processo interno)
* PedidoEnviado 🟧 (Após o ServicoDeEntrega ⬜ confirmar a coleta)
  + **Política:** Quando PedidoEnviado ➡️ Disparar NotificarClienteSobreEnvio 🟦.
* NotificacaoClienteEnviada 🟧 (Resultado de NotificarClienteSobreEnvio 🟦)
* PedidoEntregue 🟧 (Vindo do ServicoDeEntrega ⬜)

**4. Bounded Contexts: Definindo os Limites**

Observando o fluxo acima e pensando no sistema como um todo, começamos a ver separações lógicas. Um Bounded Context é um limite explícito onde um modelo de domínio particular se aplica. Dentro desse limite, a linguagem (Ubiquitous Language) é consistente.

**5. Proposta Inicial de Bounded Contexts para o Marketplace:**

Esta é uma lista inicial, sujeita a refinamento contínuo através de mais Event Storming e modelagem tática:

1. **Contexto de Identidade e Acesso (Identity & Access Management):**
   * **Responsabilidades:** Gerenciar usuários (clientes, vendedores, admins), tenants, autenticação (integração com Logto SSO), autorização (RBAC - Role-Based Access Control), perfis de usuário.
   * **Linguagem Ubíqua:** Usuário, Tenant, Papel (Role), Permissão, Sessão, Token JWT.
2. **Contexto de Configuração do Tenant (Tenant Configuration):**
   * **Responsabilidades:** Gerenciar configurações específicas de cada tenant: tema (cores, logo), features ativas/inativas (ex: permitir cupons?), regras de comissão específicas, domínios, chaves de API para serviços externos (pagamento, entrega) configurados pelo tenant.
   * **Linguagem Ubíqua:** Configuração de Tenant, Tema, Feature Flag, Regra de Negócio Customizada.
3. **Contexto de Catálogo (Catalog):**
   * **Responsabilidades:** Definição de produtos e suas variações (SKUs), categorias, atributos (cor, tamanho), marcas, especificações técnicas. Gerenciamento de conteúdo descritivo e imagens. Pode incluir aspectos de múltiplos vendedores (quem cadastra o quê). Foco na *representação* do que pode ser vendido.
   * **Linguagem Ubíqua:** Produto, Categoria, Atributo, Variante (SKU), Marca, Especificação.
4. **Contexto de Inventário (Inventory):**
   * **Responsabilidades:** Rastrear a quantidade disponível de cada SKU, possivelmente por localização ou vendedor. Gerenciar reservas de estoque (durante o checkout) e débitos (após confirmação da venda). Lida com a *disponibilidade física ou lógica*.
   * **Linguagem Ubíqua:** Estoque, SKU, Quantidade Disponível, Reserva de Estoque, Nível de Reposição.
5. **Contexto de Precificação (Pricing):**
   * **Responsabilidades:** Determinar o preço de um item para um determinado contexto (pode depender do cliente, de promoções ativas). Gerenciar listas de preços, regras de desconto, cupons de desconto.
   * **Linguagem Ubíqua:** Preço Base, Preço Final, Desconto, Cupom, Promoção, Regra de Precificação.
6. **Contexto de Carrinho de Compras (Shopping Cart):**
   * **Responsabilidades:** Gerenciar o estado transitório dos itens que um usuário pretende comprar antes de finalizar o pedido. Cálculos preliminares de subtotal.
   * **Linguagem Ubíqua:** Carrinho, Item do Carrinho, Subtotal.
7. **Contexto de Pedidos (Order Management):**
   * **Responsabilidades:** Orquestrar o ciclo de vida do pedido desde a criação até a conclusão (ou cancelamento/devolução). Armazena um "instantâneo" dos dados relevantes no momento da compra (itens, preços, endereço). Gerencia status do pedido.
   * **Linguagem Ubíqua:** Pedido, Item do Pedido, Status do Pedido (Pendente, Pago, Enviado, Entregue, Cancelado), Endereço de Entrega (no momento do pedido), Valor Total.
8. **Contexto de Pagamentos (Payment):**
   * **Responsabilidades:** Interagir com gateways de pagamento externos, processar transações financeiras (cobranças, autorizações), gerenciar reembolsos.
   * **Linguagem Ubíqua:** Pagamento, Transação, Status de Pagamento (Autorizado, Capturado, Falhou, Reembolsado), Gateway de Pagamento.
9. **Contexto de Entrega (Fulfillment/Shipment):**
   * **Responsabilidades:** Gerenciar o processo logístico após a confirmação do pedido. Calcular frete (pode interagir com serviços externos), agendar coletas, rastrear o envio, atualizar status de entrega.
   * **Linguagem Ubíqua:** Envio, Pacote, Código de Rastreio, Status de Entrega (Agendado, Em Trânsito, Entregue), Transportadora.
10. **Contexto de Notificações (Notification):**
    * **Responsabilidades:** Enviar comunicações aos usuários (email, SMS, push) baseadas em eventos de outros contextos (ex: confirmação de pedido, envio, redefinição de senha). Gerenciar templates de mensagens.
    * **Linguagem Ubíqua:** Notificação, Template, Canal (Email, SMS), Destinatário.
11. **Contexto de Avaliações (Reviews & Ratings):**
    * **Responsabilidades:** Permitir que clientes avaliem produtos e/ou vendedores. Gerenciar o conteúdo das avaliações.
    * **Linguagem Ubíqua:** Avaliação, Nota, Comentário.

**6. Context Mapping (Introdução)**

Após identificar os contextos, o próximo passo no DDD estratégico é mapear explicitamente como eles se relacionam (Context Mapping). Alguns exemplos de relações que podem surgir:

* **Pedido ➡️ Pagamento (Customer-Supplier):** O Contexto de Pedidos (Cliente) depende do Contexto de Pagamentos (Fornecedor) para processar o pagamento.
* **Pedido ➡️ Inventário (Customer-Supplier):** Pedidos precisam interagir com Inventário para reservar/debitar estoque.
* **Pedido ⬅️ Identidade (Conformist/ACL):** O Pedido precisa de informações do usuário (ID, talvez nível) do contexto de Identidade. Pode simplesmente consumir os eventos/dados de Identidade (Conformist) ou usar uma Camada Anticorrupção (ACL) para traduzir o modelo de Usuário de Identidade para o que o Pedido precisa.
* **Catálogo e Inventário (Partnership/Shared Kernel?):** Podem ser desenvolvidos em conjunto (Partnership) ou compartilhar um pequeno núcleo de conceitos (Shared Kernel, como a identificação do SKU), mas isso geralmente é evitado se possível para manter a autonomia. Uma relação Customer-Supplier é mais comum (Inventário depende do Catálogo para saber *o que* estocar).

**7. Iteração e Refinamento**

É vital entender que esta lista de Bounded Contexts e seus limites não são definitivos na primeira tentativa. Conforme aprofundamos na modelagem tática (Agregados, Value Objects) dentro de cada contexto e implementamos as interações, podemos descobrir que um contexto está fazendo coisas demais (precisa ser dividido) ou que dois contextos são tão acoplados que deveriam ser fundidos (menos comum, mas possível). O Event Storming pode ser revisitado periodicamente.