**Bounded Context - Notificações (Notification)**

**1. Definição Descritiva Refinada**

* **Propósito Central:** Este Bounded Context (BC) funciona como o centro de comunicações da plataforma, responsável por **gerenciar e despachar mensagens transacionais e informativas** para os diversos usuários (clientes finais, vendedores, administradores de tenant, administradores da plataforma). Ele opera reagindo a eventos significativos que ocorrem em outros BCs, utilizando templates pré-definidos para formatar mensagens e interagindo com provedores externos para enviá-las através de múltiplos canais (como Email, SMS, Push Notifications em aplicativos móveis e, potencialmente, mensagens In-App).
* **Responsabilidades Principais:**
  + **Consumo de Eventos de Negócio:** Atuar como um "ouvinte" central (subscriber) para eventos de domínio publicados por outros BCs no message broker (RabbitMQ). Identificar quais eventos requerem o envio de uma notificação (ex: PedidoPago, PedidoEnviado, SenhaRedefinidaSolicitada, NivelBaixoEstoqueAtingido).
  + **Gerenciamento Centralizado de Templates:** Armazenar, versionar e gerenciar os modelos (templates) para cada tipo de notificação e canal. Esses templates devem ser dinâmicos, permitindo a inserção de dados contextuais do evento gatilho (ex: Olá {{nomeCliente}}, seu pedido {{numeroPedido}} foi enviado!). Deve suportar:
    - Múltiplos Canais (Email, SMS, Push, etc.).
    - Múltiplos Idiomas (Internacionalização - i18n).
    - Customização por Tenant (permitindo que tenants ajustem o texto ou o visual básico dos templates - White Label).
  + **Seleção de Canal e Destinatário:** Com base no tipo de notificação, nas configurações do tenant e, idealmente, nas preferências do usuário (se gerenciadas), determinar o(s) canal(is) mais apropriado(s) para o envio. Identificar os dados de contato corretos do destinatário (consultando o BC de IAM/Perfil ou usando dados presentes no evento).
  + **Orquestração e Despacho do Envio:** Preparar a mensagem final renderizando o template apropriado com os dados contextuais. Interagir com o provedor de serviço externo correspondente ao canal escolhido (ex: AWS SES ou SendGrid para Email; Twilio ou Sinch para SMS; Firebase Cloud Messaging (FCM) ou Apple Push Notification Service (APNS) para Push) através de Adapters específicos.
  + **Rastreamento Básico de Status:** Registrar o sucesso ou falha do envio da notificação *ao provedor externo*. Opcionalmente, processar webhooks dos provedores para rastrear o status final da entrega (Entregue, Falhou, Aberto, Clicado), se disponível.
  + **Gestão de Preferências de Notificação (Opcional):** Poderia incluir a funcionalidade para usuários definirem quais tipos de notificação desejam receber e por quais canais. Alternativamente, isso pode residir no BC de IAM/Perfil do Usuário, e este BC de Notificações apenas consultaria essas preferências.
* **Limites e Exclusões:**
  + **Não Decide o Conteúdo de Negócio:** A *razão* pela qual uma notificação deve ser enviada e os *dados* a serem incluídos vêm dos eventos dos outros BCs. Este BC foca no *como* enviar.
  + **Não Cria a Lógica de Negócio:** Ele reage a eventos, não executa a lógica principal do pedido, pagamento, etc.
  + **Não Gerencia Usuários ou Contatos:** Apenas consome informações de contato (email, telefone, device token) obtidas de outras fontes (IAM, evento).
  + **Não é Plataforma de Marketing:** Seu foco são mensagens transacionais/informativas disparadas por eventos específicos da plataforma, não o envio de emails em massa, newsletters ou campanhas de marketing (que exigiriam ferramentas e arquitetura distintas, com gestão de opt-in/opt-out mais complexa).
* **Importância:** Fundamental para a experiência do usuário e a comunicação operacional. Mantém os usuários (clientes, vendedores, admins) informados sobre o progresso de suas ações e eventos importantes na plataforma. Notificações claras, relevantes e oportunas aumentam a confiança, reduzem a ansiedade (ex: rastreio de pedido) e podem diminuir a carga sobre o suporte ao cliente. A confiabilidade na entrega é essencial.

**2. Event Storming (Focado em Notification)**

* **Eventos Externos Chave (Consumidos - Gatilhos):** Uma vasta gama de eventos de quase todos os outros BCs:
  + Do Pedido: PedidoCriado, PedidoPago, PagamentoPedidoFalhou, PedidoEnviado, PedidoEntregue, PedidoCancelado 🟧...
  + Do Pagamento: BoletoGerado, PagamentoAprovado (pode já ser coberto pelo do Pedido), ReembolsoConfirmado 🟧...
  + Da Entrega: StatusEnvioAtualizado (para marcos específicos como "Saiu para Entrega"), FalhaNaEntrega 🟧...
  + Do Inventário: NivelBaixoEstoqueAtingido (para vendedor/admin) 🟧...
  + Do IAM: UsuarioRegistrado (bem-vindo), SenhaRedefinidaSolicitada, EmailConfirmado 🟧...
  + De Avaliações: NovaAvaliacaoPublicada (para vendedor) 🟧...
  + De Suporte/Chat: NovaMensagemRecebida 🟧...
* **Fluxo Interno Principal (Notificar "Pedido Enviado" por Email e Push):**
  + *(Listener consome PedidoEnviado)* 🟧 -> Dados: pedidoId, tenantId, userId, dadosRastreio.
  + IdentificarTiposNotificacao 🟦 (Política interna) -> Mapeia PedidoEnviado para Notificações: "PEDIDO\_ENVIADO\_EMAIL", "PEDIDO\_ENVIADO\_PUSH".
  + BuscarDadosDestinatario 🟦 (Query/Comando para IAM/Perfil) -> userId.
  + DadosDestinatarioObtidos 🟩 -> Contém email, pushTokens.
  + **Para Email:**
    - BuscarTemplate 🟦 -> Tipo="PEDIDO\_ENVIADO\_EMAIL", Canal="EMAIL", Idioma, TenantId.
    - TemplateEmailObtido 🟩.
    - PrepararNotificacaoEmail 🟦 (Comando para Notificacao Aggregate 🟨). -> Cria Notif. status 'PENDENTE'.
    - NotificacaoPreparada 🟧 (para Email). Persistido.
    - *(Adapter Email: "Enviar via SES")* ⚙️ -> Renderiza template, chama API SES ⬜.
    - *(Resposta SES: OK)*
    - MarcarNotificacaoComoEnviada 🟦 (Comando para Notificacao Aggregate 🟨). -> Atualiza status, armazena messageId.
    - NotificacaoEnviada 🟧 (para Email). Persistido.
  + **Para Push:** (Fluxo similar em paralelo)
    - BuscarTemplate 🟦 -> Tipo="PEDIDO\_ENVIADO\_PUSH", Canal="PUSH", Idioma, TenantId.
    - TemplatePushObtido 🟩.
    - PrepararNotificacaoPush 🟦 (Comando para Notificacao Aggregate 🟨).
    - NotificacaoPreparada 🟧 (para Push). Persistido.
    - *(Adapter Push: "Enviar via FCM")* ⚙️ -> Formata payload FCM, chama API FCM ⬜.
    - *(Resposta FCM: OK)*
    - MarcarNotificacaoComoEnviada 🟦 (Comando para Notificacao Aggregate 🟨).
    - NotificacaoEnviada 🟧 (para Push). Persistido.
* **Fluxo Interno (Recebendo Status de Entrega do Provedor):**
  + *(Webhook Controller recebe notificação do SES: Email Entregue)* -> Valida -> Publica evento interno WebhookStatusEmailRecebido 🟧 -> Dados: messageId externo, status "DELIVERED".
  + *(Listener consome WebhookStatusEmailRecebido)*
  + BuscarNotificacaoPorMessageId 🟦 (Query interna).
  + NotificacaoEncontrada 🟩 -> Contém notificacaoId.
  + AtualizarStatusEntregaNotificacao 🟦 (Comando para Notificacao Aggregate 🟨). -> Atualiza status para 'ENTREGUE'.
  + EntregaNotificacaoConfirmada 🟧 (Publicado). Persistido.

**3. Event Modelling (Exemplos de Eventos Chave)**

* NotificacaoPreparada (v1): { ..., "aggregateId": "notif-uuid-xyz", "payload": { "notificacaoId": "...", "tenantId": "...", "eventoOrigemId": "evt-ped-env-123", "tipoNotificacao": "PEDIDO\_ENVIADO", "canal": "SMS", "destinatario": "+55119...", "status": "PENDENTE\_ENVIO", "templateId": "tpl-ped-env-sms-ptbr", "dadosRenderizacao": {"codigoRastreio": "PX...", "nomeCliente": "..."} } }
* NotificacaoEnviada (v1): { ..., "aggregateId": "notif-uuid-xyz", "payload": { "notificacaoId": "...", "statusAnterior": "PENDENTE\_ENVIO", "statusNovo": "ENVIADA", "provedor": "Twilio", "idMensagemProvedor": "SM...", "timestampEnvio": "..." } }
* FalhaEnvioNotificacao (v1): { ..., "aggregateId": "notif-uuid-abc", "payload": { "notificacaoId": "...", "statusAnterior": "PENDENTE\_ENVIO", "statusNovo": "FALHA\_ENVIO", "tentativaNumero": 3, "provedor": "FCM", "codigoErro": "...", "mensagemErro": "...", "timestampFalha": "..." } }
* EntregaNotificacaoConfirmada (v1): { ..., "aggregateId": "notif-uuid-xyz", "payload": { "notificacaoId": "...", "statusAnterior": "ENVIADA", "statusNovo": "ENTREGUE", "timestampEventoProvedor": "..." } }

**4. Modelagem dos Componentes (DDD Tático)**

* **Aggregates:**
  + **Notificacao Aggregate:** (Representa uma instância de mensagem a ser enviada ou já enviada)
    - **Root:** Notificacao (Entity)
    - **ID:** NotificacaoId (VO, UUID)
    - **Atributos:** TenantId (VO), UserId (VO, opcional), EventoOrigemId (String, rastreabilidade), TipoNotificacao (String/Enum), Canal (Enum VO: EMAIL, SMS, PUSH, IN\_APP), Destinatario (String - email/tel/token), StatusEnvio (Enum VO: Pendente, Enviada, Entregue, FalhaEnvio, FalhaEntrega, Cancelada), TemplateUtilizadoId (String), DadosUtilizadosSnapshot (JSON/Map VO), ProvedorUtilizado (String/Enum), IdMensagemProvedor (String), CodigoUltimoErro (String), MensagemUltimoErro (String), NumeroTentativasEnvio (int), TimestampCriacao, TimestampUltimaAtualizacao, Version.
    - **Invariantes:** Status transita corretamente. NumeroTentativasEnvio é incrementado em falhas.
    - **Comandos (internos):** MarcarComoEnviadaComSucesso, RegistrarFalhaNoEnvio, RegistrarConfirmacaoEntrega, RegistrarFalhaNaEntrega.
    - **Eventos:** NotificacaoPreparada (disparado pelo serviço de preparação), NotificacaoEnviada, FalhaEnvioNotificacao, EntregaNotificacaoConfirmada, FalhaEntregaNotificacaoRegistrada.
  + **TemplateNotificacao Aggregate:** (Gerencia a definição de um template)
    - **Root:** TemplateNotificacao (Entity)
    - **ID:** TemplateId (VO - pode ser uma chave composta: tenantId:tipo:canal:idioma ou UUID).
    - **Atributos:** TenantId (VO, ou 'GLOBAL'), TipoNotificacao (String/Enum), Canal (Enum VO), Idioma (String), AssuntoTemplate (String), CorpoTemplate (String - com placeholders), Status (Enum VO: Ativo, Inativo).
    - **Invariantes:** Unicidade da chave ID. Validade da sintaxe do template.
    - **Comandos:** CriarOuAtualizarTemplate, AtivarTemplate, DesativarTemplate.
    - **Eventos:** TemplateNotificacaoSalvo, TemplateNotificacaoAtivado, TemplateNotificacaoDesativado.
* **Value Objects:** NotificacaoId, TenantId, UserId, TipoNotificacao, Canal, StatusEnvio, TemplateId, DadosUtilizadosSnapshot, Idioma, StatusTemplate.
* **Entities:** Notificacao, TemplateNotificacao.
* **CQRS:**
  + **Write Side:** Aggregates Notificacao e TemplateNotificacao. **PostgreSQL** é uma boa escolha para armazenar o histórico detalhado das notificações (para auditoria e retentativas) e os templates. **MongoDB** também é viável, especialmente para a flexibilidade dos templates. Eventos publicados no RabbitMQ.
  + **Read Side (Projections):**
    - **ProjecaoTemplatesAtivos (Redis / Cache em Memória):** Cache CRÍTICO contendo os templates ativos (TemplateId -> CorpoTemplate, AssuntoTemplate). Chave pode ser template:{tenantId}:{tipo}:{canal}:{idioma}. Essencial para a performance do serviço de envio. Populado/Invalidado pelos eventos TemplateNotificacao\*.
    - **ProjecaoLogNotificacoesUsuario (PostgreSQL/MongoDB):** Histórico de notificações enviadas para um usuário específico, para exibição na UI ("Minhas Notificações"). Indexado por UserId, TenantId. Populado por NotificacaoEnviada, etc.
    - **ProjecaoDashboardNotificacoes (Elasticsearch/PostgreSQL):** Visão agregada para monitoramento operacional: contagem de envios/falhas/entregas por tipo, canal, provedor, tenant ao longo do tempo. Populado por todos os eventos de status da Notificacao.

**5. Sagas / Process Managers**

* **Principalmente Reativo:** Este BC é majoritariamente reativo aos eventos de outros BCs.
* **ProcessManagerRetentativaNotificacao:**
  + **Trigger:** Evento FalhaEnvioNotificacao.
  + **Lógica:** Verifica NumeroTentativasEnvio. Se abaixo do limite configurado (ex: 3), agenda (com backoff exponencial) uma nova tentativa de envio, possivelmente publicando um comando ReenviarNotificacao com delay para um listener específico ou usando um scheduler. Incrementa o contador no Aggregate.

**6. Considerações Adicionais**

* **Confiabilidade e Retentativas:** Implementar um mecanismo de retentativas robusto para falhas de envio (problemas de rede com o provedor, etc.) é importante para notificações críticas.
* **Escalabilidade:** O volume de eventos de negócio pode gerar um grande volume de notificações. A arquitetura dos listeners e do serviço de envio precisa ser escalável horizontalmente. Usar filas dedicadas no RabbitMQ por canal pode ajudar.
* **Gerenciamento de Templates:** Facilitar a criação, edição (com preview) e tradução de templates é um requisito importante para a administração da plataforma e dos tenants.
* **Preferências do Usuário e LGPD/GDPR:** É fundamental ter um mecanismo (neste BC ou no IAM/Perfil) para que os usuários controlem quais notificações recebem e por quais canais, e respeitar essas preferências no momento do envio.
* **Monitoramento:** Monitorar taxas de falha de envio, taxas de entrega (via webhooks, se possível), latência dos provedores e custos é essencial.
* **Dependência de Provedores:** A qualidade e confiabilidade dos provedores externos (SES, Twilio, FCM, etc.) impactam diretamente a funcionalidade deste BC.

**Próxima Tarefa:**

Cobrimos o Contexto de Notificações. Após um pedido ser entregue e o cliente notificado, um passo comum em marketplaces é permitir que o cliente avalie a experiência. Assim, a próxima tarefa é detalhar o **Contexto de Avaliações (Reviews & Ratings)**.

O detalhamento do Contexto de Notificações foi satisfatório? Podemos prosseguir para o Contexto de Avaliações?