# PRDs Consolidado — Plataforma Multivendor de Marketplace

---

<!-- markdownlint-disable MD025 MD024 -->

# PRD — Plataforma Multivendor de Marketplace

## Controle do documento

- Versão: 1.0
- Status: proposta para refinamento
- Abrangência: Brasil
- Entidades iniciais: Sellers (Vendedores), Buyers (Compradores), Produtos Físicos
- Papel da empresa: Plataforma intermediadora, processadora de pagamentos (split) e orquestradora de pedidos.

## Visão do produto

Construir uma plataforma B2B2C que permita a vendedores cadastrar e gerenciar seus catálogos de produtos, e a compradores descobrir, comparar e adquirir itens de múltiplos vendedores em uma experiência de compra unificada. A plataforma não é dona do estoque, não atua como transportadora primária, mas garante a segurança da transação retendo valores (escrow) até a entrega e orquestrando o repasse (split) e comissionamento.

## Objetivos

1. Reduzir o tempo e esforço para vendedores iniciarem vendas online (Time-to-Market).
2. Prover uma experiência de checkout unificado para itens de múltiplos vendedores.
3. Garantir a divisão correta e transparente de recebíveis (Split de Pagamento) no momento da transação.
4. Proteger compradores e vendedores através de regras de custódia (Escrow) e resolução de disputas.
5. Produzir rastreabilidade ponta a ponta: do anúncio ao repasse financeiro.

## Não objetivos

- Comprar, armazenar ou deter a propriedade do estoque dos vendedores.
- Operar a logística física de entrega (apenas orquestração com parceiros).
- Conceder crédito direto ao consumidor (embora possa plugar parceiros financeiros).
- Substituir sistemas de ERP dos grandes vendedores (foco em integração).

## Bounded contexts

| Contexto | Responsabilidade | PRD |
| --- | --- | --- |
| Identity & Organization | Identidade de compradores, vendedores e usuários | `01-prd-identity-organization.md` |
| KYB/KYC | Verificação cadastral, risco e prevenção a fraudes de Sellers | `02-prd-kyb-kyc.md` |
| Catalog & Inventory | Gestão de anúncios, categorias, SKUs, preços e estoque | `03-prd-catalog-inventory.md` |
| Cart & Checkout | Carrinho unificado, cálculo de frete, impostos e captura de intenção | `04-prd-cart-checkout.md` |
| Order Management | Orquestração do ciclo de vida do pedido, split lógico por Seller | `05-prd-order-management.md` |
| Payment & Split | Processamento, regras de comissão (take rate) e divisão financeira | `06-prd-payment-split.md` |
| Fulfillment & Logistics | Acompanhamento de envio, rastreio e confirmação de entrega | `07-prd-fulfillment-logistics.md` |
| Settlement & Payout | Liberação de escrow, repasse financeiro e conciliação | `08-prd-settlement-payout.md` |

## Jornada principal

`ONBOARDING (Sellers/Buyers) → DISCOVERY (Search/Catalog) → CHECKOUT → PAYMENT SPLIT → ORDER ROUTING → FULFILLMENT → DELIVERY CONFIRMATION → PAYOUT`

Estados de exceção do pedido principal: `PAYMENT_FAILED`, `CANCELLED_BY_SELLER`, `OUT_OF_STOCK`, `DELIVERY_FAILED`, `RETURNED`, `DISPUTED`.

## Regras transversais

1. O vendedor é a fonte da verdade para disponibilidade de estoque e preço até o momento do checkout.
2. Um pedido só é repassado ao vendedor após autorização e reserva de saldo (captura) do pagamento.
3. O carrinho de compras pode conter itens de *N* vendedores, gerando *N* sub-pedidos operacionais e 1 pedido global financeiro.
4. O dinheiro pago pelo comprador fica em conta gráfica/escrow protegida até o gatilho de liberação (entrega).
5. O repasse (Payout) só ocorre após abatimento de comissões (Take Rate), taxas logísticas e chargebacks.
6. Falhas na integração externa (Gateways, Transportadoras) são tratadas por sagas e retentativas assíncronas.

## Indicadores de produto

- GMV (Gross Merchandise Volume) transacionado.
- Conversão de Checkout (Visitas no carrinho vs. Pedidos Pagos).
- Tempo médio de onboarding aprovado de Sellers.
- Taxa de cancelamento pós-pagamento (ruptura de estoque).
- Tempo médio entre pagamento e entrega confirmada.
- Índice de disputas e chargebacks por milhão transacionado.

## Estratégia de entrega

1. MVP: Vendedores convidados, catálogo simples, 1 meio de pagamento (Pix/Cartão s/ split no gateway), logística própria do seller.
2. V2: Self-service onboarding, Split de Pagamento automático via Gateway parceiro.
3. V3: Integração logística centralizada (geração de etiquetas pela plataforma), regras complexas de comissionamento por categoria.
4. V4: Multi-armazém, produtos digitais e serviços.

## Critério de conclusão do produto inicial

Um comprador consegue encontrar produtos de dois vendedores diferentes, colocar no mesmo carrinho, fazer um único pagamento. O sistema deve dividir os pedidos, rotear para cada vendedor preparar o envio e, após a confirmação manual de entrega, dividir o dinheiro calculando a comissão da plataforma, sem exposição a fraude básica.

---

# PRD — Identity & Organization

## Propósito

Gerenciar a identidade e acessos de todos os atores da plataforma (Compradores, Vendedores e Administradores), garantindo isolamento de dados por Tenant e perfis de permissão adequados.

## Escopo

- Cadastro de Pessoa Jurídica ou Física como Vendedor (Seller).
- Cadastro de Pessoa Física como Comprador (Buyer).
- Gestão de usuários, senhas e autenticação (SSO, Social Login para Buyers).
- Representantes legais, operadores de loja e administradores.
- Papéis e permissões no Seller Center.
- Isolamento de catálogo, pedidos e dados bancários por loja (Tenant).

Fica fora do contexto a verificação de idoneidade, PLD (Prevenção a Lavagem de Dinheiro) e score de risco, tratados pelo contexto de KYB/KYC.

## Modelo de domínio

- `Tenant` / `Store`: A loja virtual do Vendedor no marketplace.
- `SellerOrganization`: Dados fiscais do vendedor.
- `BuyerProfile`: Perfil do consumidor, endereços de entrega e métodos de pagamento salvos.
- `UserAccount`: Identidade autenticável (credenciais).
- `Membership`: Vínculo do Usuário com a Loja ou com a Plataforma.
- `Role` e `Permission`: Regras de autorização funcional (ex: "Ver Pedidos", "Editar Preços").

## Requisitos funcionais

1. Cadastrar Vendedores garantindo unicidade por CNPJ/CPF.
2. Suportar que uma mesma `UserAccount` tenha acesso a múltiplas `Stores` (ex: agências gerenciando lojas).
3. Cadastrar Compradores com fricção mínima (Social Login: Google, Apple).
4. Permitir que o Vendedor convide operadores (atendimento, expedição) com papéis restritos.
5. Manter múltiplos endereços por Comprador e por Vendedor (Origem de expedição).
6. Registrar trilha de auditoria para ações sensíveis (alteração de domicílio bancário da loja).
7. Suportar bloqueio de contas (suspensão de loja ou banimento de comprador).

## Invariantes

- Todo recurso operacional (Produto, Pedido) pertence a um `Tenant` (Loja) ou a um `BuyerProfile`.
- Uma ação administrativa no catálogo exige um `Membership` válido com a `Store`.
- Os dados sensíveis (PII) do Comprador só são visíveis ao Vendedor se houver um pedido ativo em andamento.
- Remoção de um `UserAccount` ofusca dados pessoais, mas preserva o histórico de pedidos ligados ao `BuyerProfile`.

## Comandos e eventos

Comandos: `RegisterBuyer`, `RegisterSeller`, `CreateStore`, `InviteOperator`, `GrantRole`, `UpdateBankDetails`, `SuspendStore`.

Eventos: `BuyerRegistered`, `SellerRegistered`, `StoreCreated`, `OperatorInvited`, `RoleGranted`, `StoreSuspended`, `StoreBankDetailsChanged`.

## APIs principais

- `POST /buyers`
- `POST /sellers`
- `POST /stores/{id}/memberships`
- `GET /users/{id}/effective-permissions`
- `PUT /stores/{id}/status`

## Segurança e privacidade

- MFA sugerido para Sellers, obrigatório para alteração financeira.
- Autorização baseada em RBAC (Role-Based Access Control) dentro da loja.
- Mascaramento de dados de compradores para administradores da plataforma.
- Sessões isoladas (um token de Buyer não serve nas rotas de Seller).

## Critérios de aceitação

- Um operador convidado consegue gerenciar produtos, mas não consegue alterar a conta bancária da loja.
- Ao suspender uma `Store`, automaticamente seus produtos saem do catálogo público.
- Compradores podem ter contas vinculadas ao mesmo email que um operador de Seller, mas as jornadas não colidem.

## Métricas

- Taxa de conversão do cadastro (abandono de funil).
- Lojas ativas vs inativas (sem login nos últimos 30 dias).
- Resoluções de recuperação de senha.
- Acessos bloqueados por falha de permissão.

## Dependências

KYB/KYC consome `SellerOrganization` para verificação. Catalog & Inventory depende do `Tenant` ativo para expor produtos. Order Management consulta o `BuyerProfile` para endereçamento.

---

# PRD — KYB/KYC (Marketplace)

## Propósito

Verificar a identidade e idoneidade dos Vendedores (Sellers) antes de liberá-los para transacionar na plataforma, mitigando riscos de fraude, venda de produtos ilícitos e lavagem de dinheiro.

## Escopo

- Validação de CNPJ/CPF junto à Receita Federal.
- Identificação do Quadro Societário e Beneficiário Final.
- Verificação de listas restritivas (OFAC, PEP, Mídia Negativa) dos sócios.
- Homologação do domicílio bancário (titularidade da conta vs. titularidade da loja).
- Renovação periódica ou acionada por limites de faturamento (Tiers).

## Fora do escopo

- Aprovação manual de produtos individuais (tratado em catálogo/moderação).
- Score de crédito de risco de inadimplência (pois o marketplace não empresta dinheiro).
- Verificação profunda de Buyers (focamos a fricção apenas em quem recebe o repasse financeiro).

## Modelo de domínio

- `VerificationCase`: Processo de auditoria para um Seller.
- `BusinessProfile`: Dados enriquecidos da empresa e atividade comercial (CNAE).
- `OwnerIdentity`: Documentos e verificações dos sócios.
- `BankAccountCheck`: Validação de correspondência entre loja e conta de destino.
- `RiskSignal`: Alertas emitidos por motores antifraude externos.
- `OnboardingDecision`: Veredito automático ou manual (Aprovado, Rejeitado, Pendente).

## Requisitos funcionais

1. Iniciar caso de verificação automaticamente após o preenchimento dos dados fiscais em Identity.
2. Bloquear ativação da `Store` até aprovação do KYC/KYB.
3. Consultar bureaus externos para enriquecer CNPJ e cruzar com dados fornecidos.
4. Exigir e validar titularidade da conta bancária cadastrada (Conta deve ser no mesmo CNPJ/CPF do vendedor).
5. Implementar esteira de aprovação automática por regras configuráveis.
6. Reter casos inconclusivos para revisão manual (Backoffice de Prevenção a Fraude).
7. Solicitar reenvio de documentos ilegíveis diretamente pelo Seller Center.
8. Reavaliar automaticamente o Seller caso mude seu domicílio bancário.

## Estados

`PENDING_DOCS → COLLECTING_DATA → AUTO_EVALUATING → MANUAL_REVIEW → APPROVED | REJECTED | SUSPENDED`

## Invariantes

- Nenhuma loja pode publicar produtos sem um caso `APPROVED`.
- Nenhuma conta bancária de terceiros pode ser aprovada para repasse (Split).
- A alteração do quadro societário principal invalida o status de `APPROVED` exigindo revisão.

## Comandos e eventos

Comandos: `OpenVerification`, `SubmitDocuments`, `RunExternalChecks`, `ApproveSeller`, `RejectSeller`, `RequestMoreInfo`.

Eventos: `VerificationOpened`, `DocumentsSubmitted`, `RiskSignalDetected`, `SellerApproved`, `SellerRejected`, `ReviewRequired`.

## APIs principais

- `POST /verifications/sellers/{id}/start`
- `POST /verifications/sellers/{id}/documents`
- `GET /verifications/cases?status=MANUAL_REVIEW`
- `POST /verifications/cases/{id}/decision`

## Critérios de aceitação

- Um vendedor que cadastra CNPJ e conta bancária em CPF diferente é bloqueado automaticamente.
- A plataforma consolida retornos de 3 bureaus de dados diferentes sem corromper o processo caso um falhe.
- O analista de backoffice visualiza exatamente quais regras reprovaram no motor automático.

## Métricas

- Tempo médio de aprovação automática.
- Tempo médio de aprovação manual (SLA de backoffice).
- % de Sellers aprovados vs reprovados (Funil de Risk).
- Casos de fraude identificados pós-aprovação (Falso Positivo de Risco).

## Dependências

Identity & Organization informa a criação do Seller. Settlement & Payout só processa ordens de pagamento para Sellers `APPROVED`.

---

# PRD — Catalog & Inventory

## Propósito

Centralizar, padronizar e distribuir a disponibilidade de produtos e SKUs de múltiplos vendedores, atuando como a fonte primária de verdade para o descobrimento (vitrine) e a base para a cotação no carrinho de compras.

## Escopo

- Gestão da árvore de categorias global do marketplace.
- Cadastro e normalização de Produtos (PIM) e seus SKUs (Variações).
- Controle de preço (de/por) e estoque em tempo real (ou near-real-time).
- Aprovação e moderação de anúncios (Trust & Safety).
- Mapeamento de atributos obrigatórios por categoria.

## Fora do escopo

- Exibição (Search Engine e Vitrines); este contexto foca no *dado* não na *busca* indexada.
- Cálculo final de descontos de carrinho (cupons promocionais do checkout).

## Modelo de domínio

- `CategoryTree`: Estrutura taxonômica do marketplace.
- `GlobalProduct` (opcional): Catálogo mestre (BuyBox) se produtos iguais forem agrupados.
- `SellerListing`: O anúncio específico de um vendedor.
- `SKU`: Item mantido em estoque (Stock Keeping Unit).
- `InventoryLevel`: Quantidade disponível, reservada e vendida.
- `PriceSchedule`: Política de precificação temporal (promoções).
- `ModerationStatus`: Estado de conformidade do anúncio.

## Requisitos funcionais

1. Permitir que o Seller cadastre produtos um-a-um via painel ou via upload de planilha (Lote).
2. Forçar preenchimento de dimensões (A x L x C) e Peso para garantir cálculo de frete futuro.
3. Atualizar estoque via chamadas de API de alta concorrência.
4. Decrementar estoque logicamente (Reserva) no momento da criação do pedido e fisicamente na confirmação de pagamento.
5. Devolver estoque reservado se o pagamento falhar ou o carrinho expirar.
6. Sinalizar produtos que infringem políticas (Moderação Ativa) e ocultá-los da vitrine pública.
7. Disparar webhooks ou mensagens de atualização de catálogo para indexação nos motores de busca (Elasticsearch/Algolia).

## Estados do Listing (Anúncio)

`DRAFT → PENDING_MODERATION → ACTIVE → PAUSED_BY_SELLER → OUT_OF_STOCK → BLOCKED`

## Invariantes

- Um SKU não pode ter quantidade de estoque negativa.
- A exclusão de um produto inativa o anúncio, mas não o apaga do histórico de pedidos passados.
- Dimensões e peso não podem ser nulos para SKUs físicos.
- Apenas Sellers com status KYC `APPROVED` podem ter anúncios em `ACTIVE`.

## Comandos e eventos

Comandos: `CreateListing`, `UpdatePrice`, `AdjustInventory`, `ReserveInventory`, `ReleaseInventory`, `ModerateListing`.

Eventos: `ListingCreated`, `PriceChanged`, `InventoryUpdated`, `InventoryReserved`, `InventoryDepleted`, `ListingBlocked`.

## APIs principais

- `POST /listings`
- `PUT /listings/{skuId}/inventory`
- `PUT /listings/{skuId}/price`
- `POST /catalog/reservations`
- `POST /catalog/reservations/{id}/release`

## Critérios de aceitação

- Atualizações de preço ou estoque refletem nos motores de busca em menos de 10 segundos.
- Uma compra concorrente por dois usuários no último item em estoque garante a compra ao primeiro que concluir o checkout, falhando a reserva do segundo graciosamente.
- Produtos marcados como `BLOCKED` por moderadores perdem sua URL pública instantaneamente.

## Métricas

- Taxa de rejeição de anúncios na moderação.
- Rupturas de estoque (vendeu sem ter).
- Latência nas atualizações de inventário (P99).
- Quantidade de SKUs ativos no marketplace.

## Dependências

KYB/KYC valida o status da loja. Search & Discovery consome os eventos para atualizar o índice. Cart & Checkout consulta preço e reserva estoque.

---

# PRD — Cart & Checkout

## Propósito

Agrupar intenções de compra de múltiplos vendedores em uma única experiência contínua, calcular custos logísticos reais, aplicar regras de desconto e preparar o payload final (Order) para processamento financeiro.

## Escopo

- Gerenciamento de sessão de carrinho (persistência e recuperação).
- Separação visual e lógica dos itens por Vendedor (Seller).
- Cotação de frete multi-origem em tempo real (Integration via Gateways Logísticos).
- Aplicação de cupons (Plataforma vs. Seller).
- Captura de dados de entrega, faturamento e método de pagamento.
- Conversão de Carrinho em Intenção de Pagamento.

## Fora do escopo

- Processamento de cartões de crédito (Feito pelo Payment & Split).
- Decremento final de estoque (Apenas solicita a Reserva ao Catalog).
- Roteamento logístico (Apenas consulta as opções e custos de frete).

## Modelo de domínio

- `Cart`: Recipiente efêmero de itens, vinculado a um BuyerProfile ou sessão anônima.
- `CartItem`: Referência ao SKU, quantidade e preço congelado na adição.
- `SellerPackage`: Agrupamento de CartItems por Vendedor/Origem para cotação de frete.
- `ShippingQuote`: Opções de entrega (Normal, Expresso) retornadas para um pacote.
- `CheckoutSession`: O processo irreversível de finalização (Checkout transparente).

## Requisitos funcionais

1. Adicionar, alterar quantidade e remover itens preservando o histórico da sessão.
2. Agrupar logicamente os itens por Vendedor, calculando sub-totais separados.
3. Consultar provedores de frete com base no CEP do Comprador e os respectivos CEPs de origem dos Vendedores.
4. Somar o custo de fretes distintos em um único total transparente para o comprador.
5. Revalidar preço e disponibilidade de estoque imediatamente antes do botão de pagamento.
6. Aplicar regras matemáticas de cupom: desconto restrito a produtos da plataforma ou produtos do vendedor X.
7. Bloquear checkout se qualquer item se tornar indisponível, oferecendo ao usuário a opção de "remover itens esgotados".
8. Gerar a carga estruturada para Ordem e Pagamento garantindo os centavos cravados (prevenção a erros de arredondamento).

## Estados do Carrinho / Checkout

`ACTIVE_CART → ABANDONED`
`CHECKOUT_INITIATED → QUOTING_SHIPPING → PAYMENT_PENDING → CONVERTED_TO_ORDER`

## Invariantes

- O valor total do Checkout é *estritamente* a soma dos itens + frete - descontos.
- Um carrinho convertido em ordem se torna imutável e sua sessão morre.
- A reserva de estoque ocorre no momento em que a `CheckoutSession` entra em `PAYMENT_PENDING`, com TTL (Time-To-Live) curto (ex: 15 minutos para cartão, 2 horas para boleto/Pix).

## Comandos e eventos

Comandos: `AddItem`, `RemoveItem`, `CalculateShipping`, `ApplyCoupon`, `InitiateCheckout`, `FinalizeCheckout`.

Eventos: `ItemAdded`, `CartAbandoned`, `CheckoutInitiated`, `ShippingCalculated`, `CheckoutFailed`, `OrderIntentCreated`.

## APIs principais

- `POST /carts/{id}/items`
- `POST /carts/{id}/shipping-quotes`
- `POST /checkout/sessions`
- `POST /checkout/sessions/{id}/complete`

## Critérios de aceitação

- Um carrinho contendo itens do Vendedor A e Vendedor B apresenta 2 opções de frete (uma para cada), e o usuário precisa selecionar ambas.
- Mudança de preço no catálogo enquanto o item está no carrinho atualiza o valor apenas ao entrar na página de Checkout, avisando o usuário.
- Se a API dos Correios falhar, o sistema deve usar uma tabela de contingência pre-carregada ou avisar o comprador sem quebrar o site todo.

## Métricas

- Taxa de abandono de carrinho (Cart Abandonment Rate).
- Tempo gasto na página de checkout.
- Erros de cotação de frete (Timeout).
- Carrinhos perdidos por mudança de preço/estoque.

## Dependências

Catalog & Inventory fornece produtos e reservas. Order Management recebe a intenção de ordem. Identity & Organization fornece os endereços de Buyers e Sellers.

---

# PRD — Order Management (OMS)

## Propósito

Gerenciar o ciclo de vida completo do pedido, desmembrando a compra única do cliente em pedidos operacionais isolados para cada Vendedor, e orquestrando as trocas de estado com Pagamento, Estoque e Logística.

## Escopo

- Criação do Pedido Global (Visão do Comprador).
- Criação dos Sub-pedidos (Visão do Vendedor).
- Máquina de estados (Aguardando Pagamento, Aprovado, Faturado, Enviado, Entregue, Cancelado).
- Orquestração de estornos parciais e devoluções (RMA).
- Centro de comunicação transacional (notificações para Buyer e Seller).

## Fora do escopo

- Interação direta com gateways adquirentes (Delegado ao Payment & Split).
- Geração física de etiquetas (Delegado ao Fulfillment).
- Chat direto entre comprador e vendedor (Pode ser um produto apartado).

## Modelo de domínio

- `GlobalOrder`: A compra inteira. Agrupa o pagamento único do comprador.
- `SellerOrder`: A fração do pedido direcionada a um Vendedor.
- `OrderItem`: Referência persistida do produto vendido (snapshot de preço e nome).
- `OrderStatusTimeline`: Histórico imutável de transições de status.
- `CancellationRequest`: Solicitação de cancelamento (pelo comprador ou plataforma).
- `ReturnRequest`: Processo de logística reversa.

## Requisitos funcionais

1. Receber o `OrderIntent` do Checkout e persistir o documento imutável do pedido.
2. Aguardar a confirmação assíncrona de pagamento (Webhook) para mudar o status de `PAYMENT_PENDING` para `APPROVED`.
3. Notificar o(s) Vendedor(es) apenas quando seus `SellerOrders` estiverem `APPROVED`.
4. Permitir que o vendedor atualize os estados operacionais (`INVOICED`, `SHIPPED`).
5. Processar cancelamentos pré-envio estornando o pagamento total ou parcial (se cancelado um SellerOrder apenas).
6. Coordenar a logística reversa se o pedido for devolvido (RMA).
7. Proteger o comprador contra atualização indevida (ex: Seller marca como Entregue mas transportadora não confirma).

## Estados

*Global Order*: `PENDING_PAYMENT → PAYMENT_FAILED | APPROVED → PARTIALLY_FULFILLED | FULFILLED → CLOSED`
*Seller Order*: `APPROVED → INVOICED → PREPARING → SHIPPED → DELIVERED | RETURNED | CANCELLED`

## Invariantes

- Um `SellerOrder` não pode avançar de estado sem que o `GlobalOrder` esteja `APPROVED`.
- Cancelamentos antes da nota fiscal (Invoiced) não requerem logística reversa.
- Os preços persistidos no `OrderItem` nunca mudam, mesmo que o catálogo mude o preço no futuro.
- A soma financeira dos `SellerOrders` deve ser idêntica ao valor do `GlobalOrder`.

## Comandos e eventos

Comandos: `PlaceOrder`, `ConfirmPayment`, `FailPayment`, `InvoiceOrder`, `ShipOrder`, `ConfirmDelivery`, `CancelOrder`.

Eventos: `OrderPlaced`, `PaymentConfirmed`, `SellerOrderApproved`, `OrderInvoiced`, `OrderShipped`, `OrderDelivered`, `OrderCancelled`, `RefundRequested`.

## APIs principais

- `GET /orders/global/{id}` (Visão Buyer/Admin)
- `GET /orders/seller?status=APPROVED` (Visão Seller)
- `POST /orders/seller/{id}/invoice`
- `POST /orders/seller/{id}/cancel`

## Critérios de aceitação

- Um pedido pago contendo Produto A (Vendedor X) e Produto B (Vendedor Y) gera alertas paralelos para X e Y.
- Se o Vendedor X cancelar seu pedido, a plataforma orquestra um estorno financeiro *parcial* apenas referente à parte do Vendedor X, mantendo a operação do Vendedor Y intacta.
- Comprador pode ver o status individual do pacote X e do pacote Y no painel.

## Métricas

- SLA de postagem do Vendedor (Tempo entre Approved e Shipped).
- Taxa de cancelamento por ruptura de estoque (Seller cancela pós pago).
- Tempo médio de resolução de Devoluções.

## Dependências

Cart & Checkout inicia o fluxo. Payment & Split é chamado para processar/estornar valores. Fulfillment alimenta eventos de trânsito (SHIPPED/DELIVERED).

---

# PRD — Payment & Split

## Propósito

Processar as capturas de fundos (Cartão, Pix, Boleto), garantir que os valores pagos não circulem indevidamente nas contas da plataforma (Compliance Regulatório/Bacen), e calcular com precisão milimétrica a divisão dos recebíveis entre Vendedores, Transportadoras e Plataforma (Take Rate).

## Escopo

- Integração com Gateways Sub-adquirentes (Stripe, Pagar.me, Adyen).
- Cálculo das Regras de Split (Comissões).
- Gerenciamento de Escrow (Custódia/Conta Escrow).
- Processamento de Captura, Estorno (Refund) Parcial ou Total, e Chargebacks.
- Antecipação de Recebíveis (se o modelo permitir oferecer adiantamento).

## Fora do escopo

- Liberação do dinheiro na conta bancária do vendedor (Isso é feito pelo Settlement & Payout após a entrega).
- Avaliação de crédito (Empréstimo).

## Modelo de domínio

- `PaymentIntent`: A intenção originada do checkout.
- `Transaction`: Registro do processamento no provedor.
- `SplitRule`: Parametrização percentual ou fixa de repasse (Ex: 10% Plataforma, 90% Seller).
- `EscrowLedger`: Saldo bloqueado aguardando eventos operacionais.
- `ChargebackDispute`: Contestação do titular do cartão no banco.

## Requisitos funcionais

1. Isolar dados PCI (não trafegar número de cartão em claro pelos nossos serviços, usar tokens/Vault do provedor).
2. Definir os recebedores primários do fluxo financeiro no momento da autorização (Split At-Authorization) garantindo aderência regulatória.
3. Reter valores bloqueados (Escrow) até que a plataforma receba o evento `OrderDelivered`.
4. Calcular taxa da plataforma abatendo cupons corretamente. Exemplo: se plataforma deu cupom subsidiado, ela deve abater do seu *take rate*, e não do repasse do seller.
5. Lidar com falhas transitórias e re-tentativas de autorização com o gateway.
6. Gerenciar cancelamentos parciais estornando o valor exato no cartão do cliente e estornando a comissão proporcional da plataforma.
7. Informar ao Order Management sobre sucesso ou falha da operação de captura financeira.

## Estados

`PENDING → AUTHORIZED → CAPTURED → REFUNDED_PARTIALLY | REFUNDED | CHARGEBACK`

## Invariantes

- A soma das partições de um Split Rule não pode ultrapassar 100% ou o valor exato da Transação (Prevenção contra criação ou destruição de dinheiro).
- Operação em `AUTHORIZED` não gera pedido aprovado se não transitar para `CAPTURED`.
- O saldo vai para a Conta Escrow do Recebedor; ele ainda não pode sacar.

## Comandos e eventos

Comandos: `ProcessPayment`, `CapturePayment`, `CalculateSplit`, `IssueRefund`, `HandleChargeback`.

Eventos: `PaymentAuthorized`, `PaymentCaptured`, `SplitCalculated`, `RefundProcessed`, `ChargebackReceived`.

## APIs principais

- `POST /payments/intents`
- `POST /payments/webhooks/gateway` (Recepção de notificações externas)
- `POST /payments/{id}/refunds`

## Critérios de aceitação

- Uma compra de R$100 com take rate de 15% gera uma partição intocável de R$85 para o Seller e R$15 para a Plataforma.
- Um estorno parcial repassa o prejuízo mantendo as proporções ou aplicando regras de prioridade (ex: plataforma estorna seu take rate primeiro).
- A integração absorve delays do arranjo Pix, atualizando o sistema de OMS minutos após o cliente pagar, se houver atraso bancário.

## Métricas

- Taxa de conversão de autorizações de cartão (Autorizados / Tentativas).
- Rejeições por Antifraude (Gateway).
- Volume em Escrow.
- Disputas de Chargeback abertas.

## Dependências

Checkout fornece o Intent. Identity & Organization (ou KYB) fornece os IDs dos recebedores no Gateway. Order Management aguarda e emite os comandos baseados em transições operacionais.

---

# PRD — Fulfillment & Logistics

## Propósito

Simplificar a orquestração do despacho, integrando e normalizando as diversas transportadoras em uma camada única, provendo geração de etiquetas e monitoramento de rastreio de pacotes de forma padronizada.

## Escopo

- Integração com sistemas de frete (Ex: MelhorEnvio, Kangu, Loggi, Correios).
- Compra e geração de Etiquetas de Postagem (Labels).
- Ingestão de atualizações de status de transporte (Tracking).
- Roteamento de eventos lógicos para liberar valores em custódia (Escrow).
- Gestão da Declaração de Conteúdo (quando NFe não for aplicável).

## Fora do escopo

- Operação física de frota (motoristas, caminhões).
- Estoque físico em WMS (Warehouse Management System), a menos que evolua para *Fulfillment By Plataform*.

## Modelo de domínio

- `Shipment`: A unidade física a ser despachada.
- `ShippingLabel`: O documento (PDF, ZPL) contendo o código de rastreio e dados de roteamento.
- `TrackingEvent`: Ponto no tempo registrado pela transportadora (ex: "Saiu para entrega").
- `Carrier`: Transportador e suas capacidades.

## Requisitos funcionais

1. Disponibilizar botão/API para o Vendedor gerar a etiqueta assim que o pedido for faturado.
2. Descontar o custo real da etiqueta do saldo futuro do vendedor (se a plataforma pagou) ou validar que o vendedor assumiu o envio (FOB).
3. Poll/Webhooks diários nas transportadoras para sincronizar o status de cada `Shipment` pendente.
4. Traduzir o dicionário de status logísticos de diferentes transportadoras para uma taxonomia padrão da plataforma (`IN_TRANSIT`, `OUT_FOR_DELIVERY`, `DELIVERED`, `EXCEPTION`).
5. Acionar o Order Management comunicando quando um pacote foi entregue.
6. Gerenciar exceções logísticas: endereço não encontrado, extravio e roubo de carga, facilitando as regras de seguro do pacote.

## Estados do Transporte

`LABEL_PENDING → LABEL_CREATED → DISPATCHED → IN_TRANSIT → OUT_FOR_DELIVERY → DELIVERED`

Exceções: `LOST_IN_TRANSIT`, `STOLEN`, `RETURN_TO_SENDER`, `DELIVERY_FAILED`.

## Invariantes

- Etiquetas só podem ser geradas para pedidos com status financeiro `CAPTURED`.
- O status oficial `DELIVERED` exige evidência rastreável externa ou confirmação formal pelo comprador (quando frota própria do vendedor).
- Perdas (`LOST_IN_TRANSIT`) não punem o Vendedor se o frete era de responsabilidade da plataforma, acionando seguro interno.

## Comandos e eventos

Comandos: `PurchaseLabel`, `PrintLabel`, `SyncTracking`, `MarkAsException`.

Eventos: `LabelPurchased`, `ShipmentDispatched`, `ShipmentDelivered`, `ShipmentExceptionDetected`.

## APIs principais

- `POST /shipments/{sellerOrderId}/labels`
- `GET /shipments/{trackingCode}/events`
- `POST /webhooks/carriers` (Endpoints para receber atualizações logísticas)

## Critérios de aceitação

- O vendedor não precisa sair do painel da plataforma para gerar e imprimir o PDF da etiqueta dos Correios.
- Ao receber o webhook da Jadlog de que o pacote foi entregue, o painel do comprador é atualizado, e o ciclo do pedido é encerrado via OMS.
- Eventos da transportadora com nomes esotéricos ("Encaminhado para CTE CEE") são mapeados para `IN_TRANSIT` com o texto original armazenado.

## Métricas

- Prazo Logístico Médio Prometido vs. Realizado (OTIF - On Time In Full).
- Taxa de ocorrências (Extravio, Roubo).
- Custo logístico como % do GMV.
- SLA de geração de etiqueta pelo Seller.

## Dependências

Order Management fornece a ordem para gerar envios. Payment & Split requer os eventos logísticos de sucesso para repassar dinheiro.

---

# PRD — Settlement & Payout

## Propósito

Liquiditar as operações financeiras em custódia (Escrow), processar transferências efetivas (saques/payouts) para os domicílios bancários dos vendedores e conciliar divergências financeiras garantindo saúde do fluxo de caixa.

## Escopo

- Movimentação do saldo `Escrow` para `AvailableBalance` após confirmação de recebimento.
- Criação de Ordens de Pagamento (Transferência bancária, TED/Pix).
- Extrato financeiro da loja e relatórios operacionais.
- Ciclos de liquidação automáticos (ex: saques semanais) ou sob demanda (manual via painel).
- Prevenção de saldo negativo (cobranças retroativas ou abatimento em recebíveis futuros por chargeback).

## Fora do escopo

- Divisão primária da comissão na autorização (Split at Auth, tratado no Payment & Split).
- Tratativa de emissão de NF da venda do produto (Tratado no OMS / Seller). A plataforma emite apenas a NF de prestação de serviços sobre o Take Rate.

## Modelo de domínio

- `LedgerAccount`: Conta gráfica do vendedor.
- `LedgerEntry`: Partida dobrada financeira (Crédito, Débito).
- `PayoutRequest`: Intenção de transferir saldo disponível para o banco.
- `PayoutTransaction`: A efetivação da transferência no BACEN/Arranjo Pix.
- `ConciliationBatch`: Processo de batimento entre sistema e Gateway/Bancos.

## Requisitos funcionais

1. Escutar o evento `OrderDelivered` (ou liberação antecipada programada) e creditar o montante descontado do vendedor em sua conta gráfica de `Available`.
2. Prover histórico imutável (Ledger) explicando toda mudança de saldo (Venda A, Estorno B, Tarifa Logística C).
3. Reter reservas financeiras baseadas em risco (Rolling Reserve) se o KYC assim parametrizar.
4. Processar `PayoutRequest` automaticamente descontando taxas de TED/Pix.
5. Suspender repasses se o Seller estiver sob investigação por anomalia (Fraude detectada tardiamente).
6. Executar rotinas de conciliação batendo lançamentos internos vs. arquivo de retorno CNAB/API de recebimentos dos Gateways.
7. Aplicar o débito de Chargebacks em vendas futuras se o saldo atual for zero (Saldo devedor).

## Estados do Payout

`REQUESTED → PROCESSING → COMPLETED | FAILED_BANK_REJECTION`

## Invariantes

- A soma de todos os saldos da plataforma deve bater com a conta centralizadora de garantia no banco físico.
- Nenhuma modificação em `LedgerEntry` é permitida. Correções exigem lançamentos contrapostos (Estornos compensatórios).
- Repasses só podem ocorrer para conta bancária previamente homologada no KYC, pertencente ao mesmo CNPJ do vendedor.

## Comandos e eventos

Comandos: `ReleaseEscrow`, `RequestPayout`, `ExecutePayout`, `ApplyAdjustment`, `ConciliateTransactions`.

Eventos: `BalanceAvailable`, `PayoutRequested`, `PayoutCompleted`, `PayoutFailed`, `LedgerAdjusted`.

## APIs principais

- `GET /finances/sellers/{id}/balance`
- `GET /finances/sellers/{id}/statement`
- `POST /finances/sellers/{id}/payouts`

## Critérios de aceitação

- Vendedor vê o valor de "Pedidos em Rota" no saldo bloqueado, e, magicamente no dia em que a transportadora confirma a entrega, o valor despenca para Saldo Disponível.
- Solicitar um saque envia o Pix para a conta do lojista e adiciona o lançamento no extrato com ID rastreável da liquidação.
- Se a conta bancária do vendedor estiver errada e o banco rejeitar, o sistema devolve o saldo para a conta disponível da plataforma cobrando taxa administrativa configurável, gerando notificação.

## Métricas

- Latência do Repasse (Tempo entre PayoutRequest e dinheiro na conta).
- Erros de conciliação diários (diferença entre sistema e banco).
- Volume de capital retido por estornos/saldo devedor de Sellers.

## Dependências

Payment & Split processou e proveu as métricas exatas, originando o crédito. Fulfillment informou que o risco se extinguiu, destrancando o dinheiro. KYB homologou a conta de destino.
