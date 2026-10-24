**Bounded Context - Gerenciamento de Vendedor (Seller Management)**

**1. Definição Descritiva Refinada**

* **Propósito Central:** Este Bounded Context (BC) é responsável por gerenciar o **ciclo de vida completo dos vendedores** (também chamados de sellers, lojistas ou parceiros) que disponibilizam seus produtos ou serviços através da plataforma marketplace. Ele abrange desde o processo inicial de cadastro e submissão de informações (onboarding), passando pela verificação de identidade e legitimidade (KYC/KYB), configuração do perfil público da loja, gerenciamento seguro de informações financeiras para repasses, até o controle do status operacional do vendedor na plataforma.
* **Responsabilidades Principais:**
  + **Onboarding e Cadastro:** Coletar os dados cadastrais essenciais do vendedor, que podem variar se for Pessoa Física (PF) ou Pessoa Jurídica (PJ). Isso inclui informações de identificação (CPF/CNPJ), dados de contato, endereço comercial, e a submissão de documentos comprobatórios necessários para verificação.
  + **Verificação e Aprovação (KYC/KYB):** Orquestrar e executar o processo de validação da identidade e da legitimidade do vendedor (Know Your Customer / Know Your Business). Isso pode envolver:
    - Análise (manual ou automatizada) dos documentos enviados.
    - Consultas a serviços externos (Receita Federal, Serasa, bureaus de crédito, listas de sanções).
    - Um fluxo de aprovação/reprovação pela equipe de administração da plataforma ou do tenant responsável.
  + **Gerenciamento do Perfil da Loja:** Permitir que o vendedor (através de seus usuários administradores) configure as informações que serão publicamente visíveis para os clientes, como nome da loja, descrição, logotipo, imagens de banner, informações de contato público e políticas específicas da loja (ex: política de devolução, prazos de envio específicos). *Nota: A granularidade aqui depende se cada Tenant é um Vendedor único ou se um Tenant hospeda múltiplos Vendedores. No segundo caso, este BC é ainda mais crítico.*
  + **Configuração Segura de Dados para Repasse:** Coletar e armazenar de forma **extremamente segura** as informações bancárias (banco, agência, conta) ou da conta de pagamento (ex: chave Pix, conta em gateway de pagamento) para onde os valores das vendas (líquidos de comissão) serão repassados. A segurança aqui é primordial, frequentemente envolvendo integração com Vaults ou tokenização.
  + **Gerenciamento de Status Operacional:** Controlar o estado do vendedor dentro da plataforma (ex: Pendente Verificação, Aprovado, Ativo, Suspenso Temporariamente, Inativo Permanentemente, Reprovado). Mudanças de status aqui devem gerar eventos para que outros BCs possam reagir (ex: desativar produtos de vendedor suspenso).
  + **Associação de Usuários Administradores:** Vincular o perfil do vendedor a um ou mais UserIds gerenciados pelo BC de IAM. Esses usuários receberão papéis e permissões específicas (RBAC no IAM) para administrar as operações da loja (cadastrar produtos, gerenciar pedidos, etc.).
  + **Fornecimento de Dados do Vendedor:** Expor informações (públicas e internas, conforme necessário e autorizado) sobre os vendedores para outros BCs. Exemplos: Catálogo (para associar produtoId a vendedorId), Avaliações (para permitir avaliação do vendedor), Pedidos (para saber quem é o vendedor do item), Pagamentos/Financeiro (para saber para quem repassar o dinheiro).
* **Limites e Exclusões:**
  + **Não Gerencia Usuários ou Autenticação:** O cadastro, login e gerenciamento básico dos usuários (pessoas físicas) que *administram* a loja do vendedor são responsabilidade do BC de IAM. Este BC de Seller Management foca no perfil da *entidade vendedora* e a associa aos UserIds apropriados.
  + **Não Gerencia Catálogo, Pedidos ou Estoque:** Essas são operações do dia-a-dia gerenciadas pelos BCs correspondentes. Este BC apenas identifica o vendedor associado a essas operações.
  + **Não Executa Pagamentos ou Repasses:** Pode coletar as informações de destino do repasse, mas o cálculo de comissões, a lógica de quando repassar, e a execução da transferência financeira em si geralmente pertencem ao BC de Pagamento ou, idealmente, a um **BC Financeiro/Payout** mais especializado.
  + **Não Define Configurações Globais do Tenant:** Aspectos como o tema visual geral da plataforma do tenant ou features habilitadas para *todos* os vendedores naquele tenant são gerenciados pelo BC de Tenant Configuration. Este BC foca nas configurações *individuais* de cada vendedor.
* **Importância:** Crítico em qualquer marketplace que não seja operado exclusivamente pela própria plataforma (modelo 1P). Garante a conformidade regulatória (KYC/KYB é muitas vezes obrigatório), mitiga riscos (fraude, produtos ilegais), organiza a operação multi-vendedor e fornece a ligação essencial para que produtos, pedidos e finanças sejam corretamente atribuídos ao vendedor responsável.

**2. Event Storming (Focado em Seller Management)**

* **Eventos Externos Chave (Consumidos / Informativos / Gatilhos):**
  + UsuarioRegistrado (Do IAM, tipo "VENDEDOR\_ADMIN"): Pode iniciar o fluxo de cadastro da entidade vendedora.
  + TenantConfigurado (Do Tenant Config): Para saber o contexto operacional.
  + ResultadoVerificacaoDocumento (De serviço externo ou processo manual): Informa se um documento é válido.
  + MediaEntidadeAtualizada (Do Avaliações, tipo "VENDEDOR"): Para exibir a nota média no perfil público.
* **Fluxo Interno Principal (Onboarding, Verificação e Ativação):**
  + IniciarCadastroEntidadeVendedora 🟦 (Usuário Admin Vendedor 🧍, após login inicial).
  + SubmeterDadosCadastrais 🟦 (Usuário Admin Vendedor 🧍 -> CNPJ/CPF, Razão Social, Endereço, etc.). Comando para Vendedor Aggregate 🟨. -> Cria Vendedor status 'PENDENTE\_DOCUMENTOS'.
  + VendedorPendenteDocumentos 🟧 (Publicado). Persistido.
  + AdicionarDocumento 🟦 (Usuário Admin Vendedor 🧍 -> Upload Contrato Social, RG, etc.). Comando para Vendedor Aggregate 🟨.
  + DocumentoVendedorAdicionado 🟧 (Publicado). Persistido. -> Status pode mudar para 'PENDENTE\_VERIFICACAO'.
  + *(Política: "Verificar Documentos e Dados")* ⚙️ -> Aciona verificação manual/automática/externa ⬜.
  + *(Após Verificação)*
  + AprovarVendedor 🟦 (Admin Plataforma/Tenant 🧍). Comando para Vendedor Aggregate 🟨.
  + VendedorAprovado 🟧 (Publicado). Status 'APROVADO'. Persistido. **Evento Importante!**
  + AtualizarPerfilPublicoLoja 🟦 (Usuário Admin Vendedor 🧍 -> Nome da Loja, Logo, etc.). Comando para Vendedor Aggregate 🟨.
  + PerfilLojaAtualizado 🟧 (Publicado). Persistido.
  + ConfigurarDadosRepasse 🟦 (Usuário Admin Vendedor 🧍 -> Banco, Agência, Conta). Comando para Vendedor Aggregate 🟨 (interage com Vault/Segurança).
  + DadosRepasseConfigurados 🟧 (Publicado). Persistido.
  + AtivarOperacaoVendedor 🟦 (Admin ou Automático após pré-requisitos). Comando para Vendedor Aggregate 🟨.
  + VendedorAtivado 🟧 (Publicado). Status 'ATIVO'. Persistido. **Permite vender!**
* **Fluxo Interno (Suspender Vendedor por Violação):**
  + SolicitarSuspensaoVendedor 🟦 (Admin Plataforma/Tenant 🧍 -> vendedorId, motivo). Comando para Vendedor Aggregate 🟨.
  + VendedorSuspenso 🟧 (Publicado). Status 'SUSPENSO'. Persistido.
  + *(Política: "Propagar Suspensão")* ⚙️ -> Dispara eventos/comandos para outros BCs (ex: DesativarProdutosDoVendedor para Catálogo).

**3. Event Modelling (Exemplos de Eventos Chave)**

* VendedorPendenteVerificacao (v1): { ..., "aggregateId": "seller-uuid-abc", "payload": { "vendedorId": "...", "tenantId": "...", "usuarioAdminPrincipalId": "...", "tipoPessoa": "PJ", "documento": "...", "razaoSocial": "...", "status": "PENDENTE\_VERIFICACAO", ... } }
* VendedorAprovado (v1): { ..., "aggregateId": "seller-uuid-abc", "payload": { "vendedorId": "...", "tenantId": "...", "statusAnterior": "PENDENTE\_VERIFICACAO", "statusNovo": "APROVADO", "aprovadorId": "admin-xyz", ... } }
* PerfilLojaAtualizado (v1): { ..., "aggregateId": "seller-uuid-abc", "payload": { "vendedorId": "...", "tenantId": "...", "perfil": { "nomeLoja": "Loja Incrível", "descricao": "Vendemos de tudo!", "logoUrl": "...", ... } } }
* DadosRepasseConfigurados (v1): { ..., "aggregateId": "seller-uuid-abc", "payload": { "vendedorId": "...", "tenantId": "...", "referenciaContaSegura": "vault:path/...", "infoParcialConta": "Banco X Ag Y Ct Z", ... } } (Info parcial apenas para referência, NUNCA dados completos).
* VendedorAtivado (v1): { ..., "aggregateId": "seller-uuid-abc", "payload": { "vendedorId": "...", "tenantId": "...", "statusAnterior": "APROVADO", "statusNovo": "ATIVO", ... } }
* VendedorSuspenso (v1): { ..., "aggregateId": "seller-uuid-abc", "payload": { "vendedorId": "...", "tenantId": "...", "statusAnterior": "ATIVO", "statusNovo": "SUSPENSO", "motivo": "Violação política X", ... } }

**4. Modelagem dos Componentes (DDD Tático)**

* **Aggregates:**
  + **Vendedor Aggregate:** (O agregado central para a entidade vendedora)
    - **Root:** Vendedor (Entity)
    - **ID:** VendedorId (VO, UUID)
    - **Atributos:** TenantId (VO), TipoPessoa (Enum VO), DocumentoPrincipal (CPF/CNPJ VO), RazaoSocial, NomeFantasia, Status (Enum VO com máquina de estados: PendenteDocs, PendenteVerificacao, Aprovado, Ativo, Suspenso, Inativo, Reprovado), TimestampCriacao, TimestampStatus.
    - **Value Objects Internos:** EnderecoComercial, ContatoPrincipal, PerfilLoja (VO com nome, desc, logo, etc.), DadosRepasseSeguro (VO com referência ao segredo e dados mascarados/parciais).
    - **Coleções:** Set<UserId> (Admins Associados), List<DocumentoAnexado> (VO: Tipo, UrlSegura, StatusDoc).
    - **Invariantes:** Validação de formato e unicidade de DocumentoPrincipal. Transições de Status válidas. Vendedor só pode ser 'Ativo' se 'Aprovado' e 'DadosRepasseConfigurados'? Associação de pelo menos um UserId admin.
    - **Comandos:** Registrar, AnexarDocumento, ValidarDocumento, Aprovar, Reprovar, AtualizarPerfil, ConfigurarRepasse, Ativar, Suspender, Reativar, AdicionarAdmin, RemoverAdmin.
    - **Eventos:** VendedorPendenteVerificacao, DocumentoAnexado, DocumentoValidado, VendedorAprovado, VendedorReprovado, PerfilLojaAtualizado, DadosRepasseConfigurados, VendedorAtivado, VendedorSuspenso, VendedorReativado, AdminAssociado, AdminRemovido.
* **Value Objects:** VendedorId, TenantId, TipoPessoa, DocumentoPrincipal (CPF/CNPJ), StatusVendedor, Endereco, Contato, UserId, DocumentoAnexado, PerfilLoja, DadosRepasseSeguro.
* **Entities:** Vendedor.
* **CQRS:**
  + **Write Side:** Aggregate Vendedor gerencia o ciclo de vida. **PostgreSQL** é muito adequado, lidando bem com os dados relacionais (Vendedor, Usuários) e podendo usar JSONB para detalhes como Perfil ou Documentos. A segurança dos dados de repasse é crítica (usar referência a Vault). Eventos publicados no RabbitMQ.
  + **Read Side (Projections):**
    - **ProjecaoPerfilPublicoVendedor (Redis / PostgreSQL / MongoDB):** Cache (Redis: seller\_pub:{tenantId}:{sellerId}) ou documento/tabela com nome da loja, logo, descrição, nota média (vinda de Avaliações). Usado pelo Catálogo e UI. Populado por PerfilLojaAtualizado, VendedorAtivado, VendedorSuspenso, MediaEntidadeAtualizada.
    - **ProjecaoDetalhesVendedorAdmin (PostgreSQL / MongoDB):** Visão completa para administração interna, incluindo dados cadastrais, documentos, status, usuários. Populado por todos os eventos do Vendedor.
    - **ProjecaoVendedoresParaAprovacao (PostgreSQL / MongoDB):** Lista de vendedores com status PENDENTE\_VERIFICACAO para o painel de administração.
    - **ProjecaoVendedoresAtivos (PostgreSQL / Redis Set?):** Lista/Conjunto de VendedorIds ativos para referência rápida por outros BCs (ex: Catálogo, para saber quem pode ter produtos). Populado por VendedorAtivado, VendedorSuspenso, etc.

**5. Sagas / Process Managers**

* **SagaOnboardingVerificacaoVendedor:**
  + **Trigger:** VendedorPendenteVerificacao ou DocumentoAnexado.
  + **Orquestração:** Pode coordenar chamadas a serviços externos de KYC/KYB, aguardar resultados, criar tarefas para revisão manual, e finalmente disparar AprovarVendedor ou ReprovarVendedor.
* **ProcessManagerStatusOperacionalVendedor:**
  + **Trigger:** VendedorAtivado, VendedorSuspenso, VendedorInativado.
  + **Lógica:** Publica eventos mais genéricos ou envia comandos diretos a outros BCs para que reajam à mudança de status (ex: Comando DesativarProdutosDoVendedor para o BC Catálogo quando VendedorSuspenso ocorrer).

**6. Considerações Adicionais**

* **Segurança e Conformidade (KYC/KYB/LGPD):** A coleta e validação de documentos e dados bancários são áreas de alto risco e exigem processos seguros e conformidade com regulamentações locais (LGPD no Brasil). A integração com serviços especializados de verificação é comum. O armazenamento seguro (Vault) é mandatório para dados sensíveis.
* **Flexibilidade PF/PJ:** O modelo deve suportar ambos os tipos de pessoa, com campos e documentos potencialmente diferentes.
* **Relação Tenant x Vendedor:** O design deve ser claro se a plataforma suporta 1 vendedor por tenant (Tenant = Vendedor) ou múltiplos vendedores por tenant. A modelagem aqui (TenantId dentro de Vendedor) suporta o segundo caso, que é mais flexível.
* **Repasses Financeiros:** Este BC coleta os dados de destino, mas a lógica de *quando* e *quanto* repassar (incluindo cálculo de comissões do marketplace) pertence a um BC Financeiro/Payout, que consumiria os dados seguros daqui e os dados de vendas do BC de Pedidos/Pagamentos.
* **Interface Administrativa:** Uma interface clara é necessária para os vendedores gerenciarem seus perfis e para os administradores da plataforma/tenant realizarem o processo de aprovação e gerenciamento de status.

**Próxima Tarefa:**

Concluímos o detalhamento do Contexto de Gerenciamento de Vendedor. Conforme combinado, a próxima etapa será abordar o **Context Mapping**, onde iremos mapear formalmente as interações e dependências entre todos os Bounded Contexts que discutimos até agora.

O detalhamento do Contexto de Gerenciamento de Vendedor foi satisfatório? Podemos prosseguir para o Context Mapping?