**Bounded Context - Identidade e Acesso (IAM)**

**1. Definição Descritiva Refinada**

* **Propósito Central:** Este Bounded Context (BC) é o guardião da identidade digital e das permissões de acesso dentro de toda a plataforma marketplace multi-tenant. Ele gerencia quem são os usuários, a quais tenants eles pertencem e o que eles estão autorizados a fazer.
* **Responsabilidades Principais:**
  + **Gerenciamento de Tenants:** Criação (onboarding/provisioning), ativação, desativação e gerenciamento de informações básicas dos tenants que utilizam a plataforma white-label.
  + **Gerenciamento de Usuários:** Registro, ativação, desativação e gerenciamento de perfis para todos os tipos de usuários (Clientes Finais, Vendedores, Administradores de Tenant, Administradores da Plataforma). Cada usuário está associado a um tenant específico (exceto talvez os administradores globais da plataforma).
  + **Autenticação (Delegação):** Integra-se com o provedor de SSO (Logto) para delegar o processo de autenticação (login, senha, MFA, social login). Este BC não armazena senhas, mas reage aos eventos de autenticação bem-sucedida providos pelo Logto (via JWT).
  + **Autorização (Implementação RBAC):** Define e gerencia Papéis (Roles) e Permissões dentro da aplicação. Associa usuários a papéis e implementa a lógica para verificar se um usuário autenticado tem a permissão necessária para executar uma determinada ação (Role-Based Access Control - RBAC).
  + **Gerenciamento de Perfis:** Armazena dados básicos do perfil do usuário (nome, talvez informações de contato, preferências básicas).
* **Limites e Exclusões:**
  + Não gerencia configurações *específicas da aplicação* de um tenant (como temas, features ativas, regras de comissão) – isso pertence ao BC "Tenant Configuration".
  + Não lida com o processo de login em si (páginas, fluxos de senha) – isso é responsabilidade do Logto.
  + Não contém informações de domínio de outros BCs (produtos, pedidos, etc.).
* **Importância na Multi-Tenancy:** É fundamental. Responsável por:
  + Identificar o TenantId associado a cada requisição (geralmente extraído do JWT após login via Logto).
  + Garantir que as operações e consultas de dados sejam sempre filtradas pelo TenantId correto.
  + Aplicar regras de autorização baseadas nos papéis do usuário *dentro* do seu tenant.
* **Relação com Logto:** Logto atua como o *Identity Provider* (IdP). O fluxo típico é:
  1. Usuário acessa a aplicação, é redirecionado para o Logto para login.
  2. Logto autentica o usuário e emite um JWT contendo informações essenciais (pelo menos userId e tenantId, possivelmente via custom claims configuradas no Logto).
  3. A aplicação (API Gateway ou os próprios serviços) recebe o JWT em cada requisição.
  4. O JWT é validado (assinatura, expiração).
  5. O userId e tenantId são extraídos do token validado.
  6. Este BC de IAM é consultado (usando userId e tenantId) para obter os papéis e permissões *específicos da aplicação* daquele usuário naquele tenant.
  7. A lógica de autorização (RBAC) é aplicada com base nessas permissões para permitir ou negar a ação solicitada.
  8. Pode haver sincronização de dados básicos (criação/desativação de usuário) entre Logto e este BC via APIs ou Webhooks do Logto.

**2. Event Storming Focado (Exemplos de Fluxos Internos)**

* **Fluxo: Onboarding de Novo Tenant**
  + SolicitarCriacaoTenant 🟦 (Admin da Plataforma 🧍 via CLI/UI)
  + ProvisionamentoTenantIniciado 🟧
  + *(Comando para Logto: CriarOrganizacaoLogto)* 🟦 -> Logto ⬜
  + OrganizacaoLogtoCriada 🟧 (Vindo do Logto, contém ID da organização no Logto)
  + CriarTenant 🟦 (Comando interno para o Aggregate Tenant 🟨) -> contém ID Logto
  + TenantCriado 🟧 (Publicado, contém tenantId, nome, logtoOrgId)
  + RegistrarAdminInicialTenant 🟦 (Comando para o Aggregate User 🟨) -> contém tenantId, email admin
  + UsuarioAdminTenantRegistrado 🟧 (Publicado, contém userId, tenantId, email)
  + AtribuirRoleAdminAoNovoUsuario 🟦 (Comando para o Aggregate User 🟨) -> contém userId, roleId="AdminTenant"
  + RoleAdminAtribuidaAoUsuario 🟧 (Publicado)
  + TenantProntoParaConfiguracao 🟧 (Evento final do fluxo, sinaliza para outros BCs)
* **Fluxo: Registro de Cliente Final**
  + RegistrarNovoCliente 🟦 (Cliente 🧍 via UI do Tenant, contém email, nome, senha?) -> Interage com Logto para criar usuário lá primeiro? Ou cria aqui e sincroniza? *Decisão de design importante.* Assumindo que Logto lida com o registro inicial e senha:
  + *(Usuário se registra no Logto)* -> Logto ⬜
  + EventoUsuarioCriadoLogto 🟧 (Webhook/Callback do Logto)
  + ProcessarNovoUsuarioLogto 🟦 (Listener interno) -> contém logtoUserId, email, tenantId (da Org Logto)
  + RegistrarUsuarioNoSistema 🟦 (Comando para Aggregate User 🟨)
  + UsuarioClienteRegistrado 🟧 (Publicado)
  + AtribuirRoleCliente 🟦 (Comando para Aggregate User 🟨)
  + RoleClienteAtribuida 🟧 (Publicado)
* **Fluxo: Verificação de Permissão (Autorização)**
  + (Requisição Chega com JWT validado, userId e tenantId extraídos)
  + (Código precisa verificar permissão, ex: "visualizar-catalogo-admin")
  + VerificarPermissaoUsuario 🟩 (Consulta ao Read Model/Projeção) -> Input: userId, tenantId, permissaoRequerida
  + PermissaoVerificada 🟩 (Resultado da consulta: true ou false)
  + (Lógica de negócio continua ou retorna erro 403 Forbidden)

**3. Event Modelling (Exemplos de Eventos Chave)**

* TenantCriado (v1)
  {
   "eventId": "uuid-evento-1",
   "eventType": "TenantCriado",
   "eventVersion": "1",
   "timestamp": "2025-04-22T15:04:00Z",
   "aggregateId": "tenant-uuid-123", // TenantId
   "correlationId": "uuid-correlacao-abc", // Para rastrear fluxo
   "payload": {
   "tenantId": "tenant-uuid-123",
   "nomeTenant": "Loja Exemplo SA",
   "planoId": "plano-pro",
   "logtoOrganizationId": "logto-org-xyz",
   "dataCriacao": "2025-04-22T15:03:59Z"
   }
  }
* UsuarioRegistrado (v1)
  {
   "eventId": "uuid-evento-2",
   "eventType": "UsuarioRegistrado",
   "eventVersion": "1",
   "timestamp": "2025-04-22T15:10:00Z",
   "aggregateId": "user-uuid-456", // UserId
   "correlationId": "uuid-correlacao-def",
   "payload": {
   "userId": "user-uuid-456",
   "tenantId": "tenant-uuid-123",
   "email": "cliente@exemplo.com",
   "nome": "Nome do Cliente",
   "tipoUsuario": "CLIENTE", // Enum: CLIENTE, VENDEDOR, ADMIN\_TENANT, ADMIN\_PLATAFORMA
   "logtoUserId": "logto-user-abc",
   "dataRegistro": "2025-04-22T15:09:59Z"
   }
  }
* RoleAtribuidaAoUsuario (v1)
  {
   "eventId": "uuid-evento-3",
   "eventType": "RoleAtribuidaAoUsuario",
   "eventVersion": "1",
   "timestamp": "2025-04-22T15:11:00Z",
   "aggregateId": "user-uuid-456", // UserId
   "correlationId": "uuid-correlacao-ghi",
   "payload": {
   "userId": "user-uuid-456",
   "tenantId": "tenant-uuid-123",
   "roleId": "role-cliente-uuid"
   }
  }

**4. Modelagem dos Componentes (DDD Tático)**

* **Aggregates:**
  + **Tenant Aggregate:**
    - **Root:** Tenant (Entity)
    - **ID:** TenantId (Value Object, ex: UUID)
    - **Atributos:** Nome (VO?), Status (Enum VO: Ativo, Inativo, Pendente), PlanoId (VO?), LogtoOrganizationId (VO?), DataCriacao (VO?).
    - **Invariantes:** Nome não pode ser vazio. Status segue transições válidas. LogtoOrganizationId é obrigatório após provisionamento.
    - **Comandos:** CriarTenant, AtivarTenant, DesativarTenant, AssociarLogtoOrg.
    - **Eventos:** TenantCriado, TenantAtivado, TenantDesativado, LogtoOrgAssociada.
  + **User Aggregate:**
    - **Root:** User (Entity)
    - **ID:** UserId (Value Object, ex: UUID)
    - **Atributos:** TenantId (VO, FK para Tenant), LogtoUserId (VO, identificador único do Logto), Email (VO, com validação), Nome (VO?), Status (Enum VO: Ativo, Inativo, PendenteVerificacao), TipoUsuario (Enum VO).
    - **Coleções:** Set<RoleId> (Conjunto de IDs dos papéis associados, VOs).
    - **Invariantes:** Email deve ser válido. TenantId e LogtoUserId são obrigatórios. Um usuário deve ter pelo menos um RoleId após ativação? Status segue transições válidas. TipoUsuario não pode ser alterado levianamente.
    - **Comandos:** RegistrarUsuario, AtivarUsuario, DesativarUsuario, AtribuirRole, RemoverRole, AtualizarPerfil.
    - **Eventos:** UsuarioRegistrado, UsuarioAtivado, UsuarioDesativado, RoleAtribuidaAoUsuario, RoleRemovidaDoUsuario, PerfilUsuarioAtualizado.
  + **Role Aggregate:**
    - **Root:** Role (Entity)
    - **ID:** RoleId (Value Object, ex: UUID ou String significativa como "admin-tenant")
    - **Atributos:** TenantId (VO, pode ter valor especial 'GLOBAL' ou referenciar um Tenant específico, permitindo roles globais ou por tenant), Nome (VO?), Descricao (VO?).
    - **Coleções:** Set<PermissionId> (Conjunto de IDs das permissões associadas, VOs. PermissionId pode ser uma String como "catalog:read", "order:create:own").
    - **Invariantes:** Nome da Role único por TenantId. Permissões devem ser válidas (pertencer a uma lista predefinida ou a outro aggregate/entidade Permission se forem dinâmicas).
    - **Comandos:** CriarRole, ExcluirRole, AdicionarPermissao, RemoverPermissao.
    - **Eventos:** RoleCriada, RoleExcluida, PermissaoAdicionadaARole, PermissaoRemovidaDaRole.
* **Value Objects (VOs):** TenantId, UserId, RoleId, PermissionId (pode ser String), LogtoUserId, LogtoOrganizationId, Email, StatusUsuario, StatusTenant, TipoUsuario, Nome, Descricao, PlanoId.
* **Entities:** Tenant, User, Role.
* **CQRS:**
  + **Write Side:** Os Aggregates (Tenant, User, Role) processam os Comandos, validam invariantes e publicam os Eventos de Domínio no RabbitMQ. Persistência primária em **PostgreSQL**, que lida bem com as relações (Tenant -> User, User -> Roles, Role -> Permissions).
  + **Read Side (Projections):** Consumers assíncronos leem os eventos do RabbitMQ e atualizam modelos de leitura otimizados. Exemplos:
    - ProjecaoPerfilUsuarioAutorizacao: Armazena dados essenciais para UI e, crucialmente, para verificação rápida de permissões. Pode conter { userId, tenantId, email, nome, status, roles: ["roleId1", ...], permissions: ["perm1", "perm2", ...] }. Poderia estar no **PostgreSQL** (tabela denormalizada) ou **Redis** (cache rápido chaveado por userId:tenantId). Atualizado por Usuario\*, Role\*, Permissao\* eventos.
    - ProjecaoListaUsuariosTenant: Para admins de tenant. { userId, email, nome, status, roles: [...] }. Em **PostgreSQL**. Atualizado por Usuario\* e Role\* eventos filtrados por tenantId.
    - ProjecaoListaTenants: Para admins da plataforma. { tenantId, nome, status, planoId }. Em **PostgreSQL**. Atualizado por Tenant\* eventos.

**5. Sagas / Process Managers**

* **SagaOnboardingTenant:**
  + **Trigger:** Evento SolicitacaoCriacaoTenantRecebida ou Comando CriarTenant.
  + **Orquestração:**
    1. Envia Comando CriarOrganizacaoLogto (para um adaptador Logto).
    2. Espera Evento OrganizacaoLogtoCriada.
    3. Envia Comando CriarTenant (para Aggregate Tenant) com logtoOrgId.
    4. Espera Evento TenantCriado.
    5. Envia Comando RegistrarUsuario (para Aggregate User) para o admin inicial.
    6. Espera Evento UsuarioRegistrado.
    7. Envia Comando AtribuirRole (para Aggregate User) com roleId="AdminTenant".
    8. Espera Evento RoleAtribuidaAoUsuario.
    9. Publica Evento TenantProntoParaConfiguracao.
  + **Estado:** A Saga precisa manter o estado do processo (ex: tenantId, userIdAdmin, statusOnboarding) para lidar com falhas e timeouts. Pode ser persistido em DB.
* **SagaSincronizacaoUsuarioLogto (Alternativa/Complemento):**
  + **Trigger:** Webhook/Evento UsuarioCriadoLogto ou UsuarioAtualizadoLogto ou UsuarioDeletadoLogto.
  + **Orquestração:** Envia comandos correspondentes (RegistrarUsuario, AtualizarPerfil, DesativarUsuario) para o Aggregate User neste BC. Garante que o estado do usuário no IAM reflita o estado no Logto.

**6. Considerações Adicionais**

* **Escolha do Banco:** PostgreSQL é uma escolha sólida para o write-side devido à natureza relacional dos dados de IAM e às garantias ACID. Para o read-side, pode-se usar PostgreSQL (views materializadas ou tabelas denormalizadas) ou Redis para lookups de permissão ultra-rápidos se necessário.
* **Permissões:** A modelagem das permissões (PermissionId) é importante. Podem ser simples strings, ou se houver hierarquia ou regras complexas, podem precisar de sua própria entidade ou até aggregate. Manter como strings (recurso:verbo, ex: product:read, order:create) é comum e flexível.
* **Desafios:** Garantir a consistência entre o Logto e este BC. Lidar com a propagação de mudanças (ex: exclusão de role que afeta múltiplos usuários). Performance das consultas de autorização.