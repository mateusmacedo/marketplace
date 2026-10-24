**Cross-Cutting Concerns (Parte 2: Fluxo de Autenticação e Autorização)**

**1. Introdução e Definições**

Em nossa arquitetura distribuída, garantir que apenas os usuários certos possam acessar os recursos corretos é fundamental e permeia todos os serviços. Dividimos isso em dois conceitos:

1. **Autenticação (AuthN - Quem é você?):** O processo de verificar a identidade de um usuário ou serviço que tenta acessar o sistema. Precisamos ter certeza de quem está fazendo a requisição.
2. **Autorização (AuthZ - O que você pode fazer?):** O processo de determinar se um usuário *autenticado* tem as permissões necessárias para realizar uma ação específica ou acessar um recurso particular.

**Objetivo:** Implementar um fluxo de ponta-a-ponta que seja seguro, robusto, escalável e o mais performático possível, utilizando as ferramentas e BCs definidos: Logto como provedor de identidade externo, o API Gateway como ponto de controle inicial, e nosso BC de IAM interno como a autoridade para permissões específicas da aplicação (RBAC).

**2. Componentes Envolvidos (Recap)**

* **Usuário:** Interage com a aplicação via navegador ou app móvel.
* **Aplicação Frontend (SPA/Mobile):** Interface com o usuário, inicia fluxos de login, armazena tokens, envia requisições para o backend.
* **Logto (Provedor SSO):** Nosso Identity Provider (IdP) externo. Lida com telas de login/registro, gerenciamento de senhas, MFA, logins sociais (Google, etc.). Emite tokens JWT (JSON Web Tokens) após autenticação bem-sucedida. Gerencia o conceito de "Organizations", que mapearemos para nossos Tenants.
* **API Gateway (Traefik + Middleware?):** Ponto de entrada único para as requisições de API. Responsável primário pela validação dos tokens de acesso (AuthN). Pode ter um middleware customizado ou usar plugins para interagir com validação JWT.
* **BC Identidade e Acesso (IAM - Serviço Interno):** Nosso serviço de backend que gerencia perfis de usuário específicos da aplicação (vinculados aos usuários do Logto), associações de tenant, e, crucialmente, define e armazena os **Papéis (Roles)** e **Permissões (Permissions)** da aplicação (implementando Role-Based Access Control - RBAC).
* **Serviços de Backend (BCs: Pedidos, Catálogo, etc.):** Recebem requisições já (idealmente) autenticadas e precisam realizar verificações de autorização (AuthZ) consultando o IAM BC antes de executar a lógica de negócio.

**3. Fluxo de Autenticação (AuthN - Com Logto e JWT)**

Este fluxo descreve como um usuário faz login e obtém os tokens necessários. Usaremos o fluxo **Authorization Code com PKCE (Proof Key for Code Exchange)**, que é o padrão ouro para SPAs e aplicações móveis.

1. **Início do Login:** Usuário clica em "Login" no Frontend.
2. **Redirecionamento para Logto:** O Frontend redireciona o navegador do usuário para a página de login do Logto, passando parâmetros como client\_id (da aplicação registrada no Logto), redirect\_uri (para onde Logto deve retornar após o login), scope (ex: openid profile email offline\_access), response\_type=code, e os parâmetros do PKCE (code\_challenge, code\_challenge\_method). Pode incluir um organization\_id se o tenant for conhecido.
3. **Autenticação no Logto:** O usuário interage **exclusivamente com o Logto** para se autenticar (usuário/senha, MFA, login social). Logto verifica as credenciais.
4. **Retorno com Código:** Se a autenticação for bem-sucedida, Logto redireciona o navegador de volta para a redirect\_uri registrada no Frontend, incluindo um code (Authorization Code) na URL.
5. **Troca do Código por Tokens:** O Frontend (ou seu BFF - Backend for Frontend) faz uma requisição **segura e direta** (backend-to-backend) para o endpoint de token do Logto, enviando:
   * O code recebido.
   * grant\_type=authorization\_code.
   * redirect\_uri (o mesmo usado no passo 2).
   * client\_id e client\_secret (se for uma aplicação confidencial) ou apenas client\_id.
   * O code\_verifier do PKCE.
6. **Logto Emite Tokens:** Se a troca for válida, Logto retorna uma resposta JSON contendo:
   * **id\_token (JWT):** Contém informações sobre o usuário autenticado (identificador sub do Logto, email, nome, etc.) e a organização (org\_id que mapeia para nosso TenantId). Destina-se principalmente ao Frontend para exibir informações do usuário.
   * **access\_token (JWT):** O token que será enviado às APIs do backend. Contém informações para autorização (issuer iss, audience aud, expiration exp, sub, org\_id, e possivelmente scopes).
   * **refresh\_token (Opaque String):** Usado para obter novos access\_token e id\_token quando os atuais expirarem, sem que o usuário precise logar novamente. Deve ser armazenado com segurança máxima.
7. **Armazenamento Seguro de Tokens:** O Frontend armazena o access\_token (geralmente em memória ou local storage seguro) para uso nas chamadas de API. O refresh\_token deve ser armazenado da forma mais segura possível (ex: cookie HttpOnly, se houver um BFF, ou mecanismos seguros de armazenamento do dispositivo móvel).
8. **Chamadas de API Subsequentes:** Para toda requisição enviada ao API Gateway do backend, o Frontend inclui o access\_token no cabeçalho HTTP: Authorization: Bearer <access\_token>.

**4. Fluxo de Validação do Token (API Gateway / Entrada do Serviço)**

Cada requisição que chega ao API Gateway precisa ter seu access\_token validado.

1. **Receber Requisição:** O Gateway (Traefik + possível middleware) recebe a requisição com o header Authorization: Bearer <token>.
2. **Extrair Token:** O token JWT é extraído do header.
3. **Validar Assinatura (Verifica Autenticidade):** O middleware valida a assinatura do JWT usando as chaves públicas do Logto. Essas chaves são obtidas do endpoint JWKS (JSON Web Key Set) do Logto (configurado no middleware). Isso garante que o token foi emitido pelo Logto e não foi adulterado. O JWKS deve ser cacheado localmente com um TTL curto para performance.
4. **Validar Claims (Verifica Validade e Relevância):** O middleware valida as claims padrão do JWT:
   * iss (Issuer): Deve corresponder ao identificador do seu tenant Logto.
   * aud (Audience): Deve corresponder ao identificador da sua API (configurado no Logto e no middleware).
   * exp (Expiration Time): A data/hora de expiração deve ser futura.
   * nbf (Not Before): A data/hora deve ser passada.
   * iat (Issued At): Padrão.
5. **(Opcional) Verificação de Scopes:** Se sua aplicação usar scopes OAuth2, o middleware pode verificar se os scopes presentes no token são suficientes para acessar o endpoint solicitado (autorização muito básica).
6. **Extrair Contexto Relevante:** Se o token for válido, extrair as claims essenciais para os serviços backend:
   * sub: O identificador único do usuário no Logto (será usado para ligar ao UserId no nosso IAM BC).
   * org\_id: O identificador da Organização Logto (será mapeado para o TenantId da nossa aplicação).
7. **Encaminhar Requisição com Contexto:** A requisição é encaminhada para o serviço de backend apropriado. O contexto autenticado (userId mapeado do sub, tenantId mapeado do org\_id) é injetado de forma segura para o serviço (ex: através de cabeçalhos HTTP internos como X-User-ID, X-Tenant-ID, que só podem ser setados pelo Gateway). *Alternativa: cada serviço revalida o token, mas isso adiciona latência.* Confiar no Gateway após validação rigorosa é comum.

**5. Fluxo de Autorização (AuthZ - Com IAM BC e RBAC)**

Uma vez que a requisição chega a um serviço de backend (ex: Serviço de Pedidos) com o contexto de usuário (userId) e tenant (tenantId) validados:

1. **Identificar Permissão Necessária:** O código do serviço (ex: no Controller/Resource ou na camada de Serviço/Aplicação) determina qual permissão específica é necessária para executar a ação solicitada. Ex: pedido:ler:proprio, produto:criar, admin:tenant:gerenciar\_usuarios. Essa definição faz parte do design de segurança da aplicação.
2. **Consultar o IAM BC:** O serviço faz uma chamada interna (idealmente via gRPC pela baixa latência, ou REST) para o BC de IAM, perguntando essencialmente: "O usuário {userId} no tenant {tenantId} possui a permissão {permissao\_necessaria}?".
3. **Lógica no IAM BC:** O IAM BC executa a lógica de RBAC:
   * Busca o registro do usuário (User Aggregate/Projeção) pelo userId e tenantId.
   * Obtém a lista de RoleIds associados a esse usuário.
   * Para cada RoleId, busca a lista de PermissionIds associadas a esse papel (Role Aggregate/Projeção).
   * Agrega todas as permissões efetivas do usuário.
   * Verifica se a {permissao\_necessaria} está contida na lista de permissões efetivas.
   * Retorna a decisão (true ou false) para o serviço solicitante.
   * **Otimização:** Esta consulta precisa ser extremamente rápida. O IAM BC deve usar projeções de leitura altamente otimizadas e cacheadas (Redis, Caffeine cache em memória) para os dados de papéis e permissões dos usuários, evitando buscas complexas no banco de dados a cada requisição. Uma query como "Buscar todas as permissões para usuário X no tenant Y" pode ser mais eficiente do que checar uma permissão por vez.
4. **Tomar Decisão no Serviço:** O serviço que fez a consulta recebe a resposta do IAM BC:
   * Se true: A autorização foi concedida. O serviço prossegue com a execução da lógica de negócio.
   * Se false: A autorização foi negada. O serviço **imediatamente** rejeita a requisição, geralmente retornando um status HTTP 403 Forbidden.
5. **RBAC Explícito:** Este fluxo implementa o Role-Based Access Control. As permissões são o nível mais granular. Os papéis agrupam permissões. Os usuários recebem papéis. Os serviços verificam apenas a permissão final necessária, desacoplando a lógica de negócio das estruturas de papéis.
6. **Verificações Adicionais (Ex: Ownership):** Para ações que dependem não só do papel, mas também da relação do usuário com o recurso específico (ex: editar *apenas* o *seu próprio* pedido), o serviço fará a checagem de permissão no IAM *E TAMBÉM* verificará a condição de posse. Ex: No serviço de Pedidos, para editar o pedido XYZ, verifica se pedidoXYZ.userId == request.userId *após* confirmar que o usuário tem a permissão pedido:editar:proprio.

**6. Considerações de Segurança e Performance**

* **Tokens:** Usar Access Tokens de curta duração (minutos) e Refresh Tokens de longa duração (dias/semanas), com armazenamento seguro para os Refresh Tokens. Implementar rotação de Refresh Tokens se possível.
* **Comunicação:** TODO o tráfego (frontend <-> Logto, frontend <-> gateway, gateway <-> serviços, serviço <-> serviço) DEVE usar HTTPS/TLS. Avaliar mTLS para comunicação interna serviço-a-serviço se o nível de segurança exigir.
* **Validação JWT:** Fazer a validação completa (assinatura, issuer, audience, expiração) no Gateway/middleware. Usar bibliotecas confiáveis. Cachear JWKS do Logto com TTL curto.
* **Performance da Autorização:** A chamada interna para o IAM BC para checar permissões é um ponto crítico de latência. Otimizar essa consulta com caches agressivos (Redis, em memória com TTL) nas projeções de leitura do IAM BC é fundamental. Considere estratégias como o IAM BC publicar eventos de mudança de permissão para que serviços interessados possam manter um cache local (com os desafios de consistência eventual).
* **Rate Limiting e WAF:** Proteger o API Gateway e os endpoints públicos do Logto com Rate Limiting e um Web Application Firewall (WAF).

**Próxima Tarefa:**

Cobrimos o fluxo detalhado de Autenticação e Autorização. O próximo aspecto relevante dentro dos Cross-Cutting Concerns seria o **Design das APIs (REST para Comandos, GraphQL para Consultas, e CLI Administrativa)**, definindo como os clientes interagem com nosso sistema.

O detalhamento do fluxo de Autenticação e Autorização foi satisfatório? Podemos prosseguir para o Design das APIs?