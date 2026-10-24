**Bounded Context - Avaliações (Reviews & Ratings)**

**1. Definição Descritiva Refinada**

* **Propósito Central:** Este Bounded Context (BC) é dedicado a capturar, gerenciar e expor o **feedback dos usuários** sobre as entidades principais do marketplace, mais comumente **produtos** e, em cenários multi-vendedor, os próprios **vendedores**. Ele lida com todo o ciclo de vida das avaliações, desde a submissão pelo cliente, passando pela moderação de conteúdo, até a publicação e o cálculo de métricas agregadas (como notas médias).
* **Responsabilidades Principais:**
  + **Submissão de Avaliações:** Fornecer a interface (via API consumida pela UI) para que usuários elegíveis (tipicamente clientes que realizaram uma compra verificada do item) possam submeter suas opiniões. Isso geralmente inclui:
    - Uma nota numérica (ex: 1 a 5 estrelas).
    - Um título e/ou comentário textual.
    - Opcionalmente, upload de mídias (fotos, vídeos) relacionadas à avaliação.
  + **Validação de Elegibilidade:** Verificar se o usuário que está submetendo a avaliação está realmente apto a fazê-lo para aquele produto/vendedor específico (geralmente consultando o histórico no BC de Pedidos para confirmar a compra).
  + **Moderação de Conteúdo:** Implementar e gerenciar um fluxo para revisar o conteúdo submetido antes de torná-lo público. O objetivo é garantir a aderência às políticas da plataforma (filtrar spam, linguagem ofensiva, informações pessoais, conteúdo irrelevante ou fraudulento). A moderação pode ser:
    - Manual: Revisores humanos analisam cada avaliação.
    - Automática: Algoritmos (IA/ML) tentam detectar problemas comuns.
    - Híbrida: Uma combinação das duas abordagens.
  + **Gerenciamento do Ciclo de Vida da Avaliação:** Controlar o status de cada avaliação (Pendente Moderação, Aprovada, Rejeitada, Removida).
  + **Publicação de Avaliações:** Tornar as avaliações aprovadas visíveis nas interfaces apropriadas (página de detalhes do produto, perfil do vendedor).
  + **Gerenciamento de Respostas (Opcional):** Permitir que a entidade avaliada (vendedor do produto, administrador do tenant) submeta uma resposta pública a uma avaliação de cliente. Essas respostas também podem passar por um fluxo de moderação.
  + **Cálculo de Métricas Agregadas:** Calcular e manter atualizadas as métricas de avaliação para cada entidade (produto/vendedor), como:
    - Nota média (rating).
    - Número total de avaliações aprovadas.
    - Distribuição de notas (quantas avaliações de 5 estrelas, 4 estrelas, etc.).
  + **Consulta e Exibição:** Fornecer dados (via projeções de leitura) para que outros BCs (como Catálogo) ou a UI possam exibir as avaliações publicadas, as respostas e as métricas agregadas.
  + **Sinalização de Conteúdo (Opcional):** Permitir que usuários da plataforma sinalizem ("reportem") avaliações que considerem inadequadas, direcionando-as para revisão pela equipe de moderação.
* **Limites e Exclusões:**
  + **Não Gerencia Produtos/Vendedores:** Ele apenas *referencia* produtos e vendedores por seus IDs, obtidos de outros BCs (Catálogo, IAM/Seller Management). Não armazena os detalhes desses produtos/vendedores.
  + **Não Gerencia Pedidos:** Apenas *consulta* o BC de Pedidos para validar a elegibilidade do avaliador. Não altera ou gerencia o estado dos pedidos.
  + **Não é um Sistema de Suporte/SAC:** Foca no feedback público estruturado (nota/comentário), não na resolução de problemas específicos do cliente através de tickets ou chat.
* **Importância:** Fundamental para construir **confiança, prova social e transparência** no marketplace. As avaliações de outros clientes são um dos fatores mais influentes na decisão de compra. O feedback coletado é extremamente valioso para que vendedores melhorem seus produtos/serviços e para que a plataforma identifique tendências e problemas. Um sistema de moderação eficaz é vital para manter a credibilidade e evitar abusos.

**2. Event Storming (Focado em Reviews & Ratings)**

* **Eventos Externos Chave (Consumidos / Informativos / Gatilhos):**
  + PedidoEntregue 🟧 (Do Pedido/Entrega): Pode disparar um processo (via Saga/Scheduler + Notificações) para *convidar* o cliente a avaliar.
  + ProdutoDetalhesConsultados 🟩 (Leitura do Catálogo): Para associar a avaliação ao produto correto.
  + VendedorDetalhesConsultados 🟩 (Leitura do IAM/Seller Mgmt): Para associar a avaliação ao vendedor correto.
  + UsuarioLogado 🟩 (Leitura do IAM): Para identificar o autor.
* **Fluxo Interno Principal (Cliente Submete Avaliação de Produto):**
  + SubmeterAvaliacao 🟦 (Cliente 🧍 via formulário na UI) -> Dados: produtoId, pedidoId (prova de compra), userId, nota, comentario, midias?. Comando para Avaliacao Aggregate 🟨.
  + *(Política: "Verificar Compra")* ⚙️ -> Consulta BC Pedidos 🟩: Cliente userId comprou produtoId no pedidoId?
  + *(Se Compra Verificada)*
  + AvaliacaoSubmetida 🟧 (Publicado) -> Status "PENDENTE\_MODERACAO". Persistido.
  + *(Política: "Enviar para Moderação")* -> Atualiza ProjecaoAvaliacoesPendentesModeracao 🟩.
* **Fluxo Interno (Moderação Manual Aprova):**
  + *(Moderador visualiza ProjecaoAvaliacoesPendentesModeracao)* 🟩.
  + AprovarAvaliacaoSubmetida 🟦 (Moderador 🧍 via painel de moderação) -> Comando para Avaliacao Aggregate 🟨 (avaliacaoId).
  + AvaliacaoAprovada 🟧 (Publicado) -> Status "APROVADA". Persistido.
  + *(Política: "Atualizar Métricas e Visibilidade")* ⚙️
    - Dispara RecalcularMetricasEntidade 🟦 (Comando para EntidadeAvaliada Aggregate/Projeção 🟨/🟩).
    - Atualiza ProjecaoAvaliacoesPublicadas 🟩.
* **Fluxo Interno (Moderação Rejeita):**
  + RejeitarAvaliacaoSubmetida 🟦 (Moderador 🧍) -> Comando para Avaliacao Aggregate 🟨 (avaliacaoId, motivoRejeicao).
  + AvaliacaoRejeitada 🟧 (Publicado) -> Status "REJEITADA". Persistido. (Pode disparar Notificação para o autor).
* **Fluxo Interno (Cálculo de Média Após Aprovação):**
  + *(Listener consome AvaliacaoAprovada)* 🟧 -> Dados: avaliacaoId, entidadeAvaliadaId, nota.
  + RecalcularMetricasEntidade 🟦 (Comando para EntidadeAvaliada Aggregate/Projeção 🟨/🟩).
  + *(Lógica interna: busca todas as notas aprovadas, recalcula média, total, distribuição)* ⚙️.
  + MediaEntidadeAtualizada 🟧 (Publicado). Persistido/Cache atualizado.
* **Fluxo Interno (Vendedor Responde):**
  + SubmeterResposta 🟦 (Vendedor 🧍 via painel) -> Dados: avaliacaoId, textoResposta. Comando para Avaliacao Aggregate 🟨.
  + RespostaSubmetida 🟧 (Publicado) -> Status da Resposta "PENDENTE\_MODERACAO". Persistido no Avaliacao.
  + *(Fluxo de Moderação da Resposta similar ao da avaliação)*
  + AprovarRespostaSubmetida 🟦 -> Comando para Avaliacao Aggregate 🟨.
  + RespostaPublicada 🟧 (Publicado) -> Resposta visível. Persistido.

**3. Event Modelling (Exemplos de Eventos Chave)**

* AvaliacaoSubmetida (v1): { ..., "aggregateId": "review-uuid-777", "payload": { "avaliacaoId": "...", "tenantId": "...", "entidade": { "tipo": "PRODUTO", "id": "prod-xyz" }, "autor": { "userId": "...", "nomeExibicao": "Cliente S." }, "pedidoOrigemId": "order-123", "nota": 4, "titulo": "Bom produto", "comentario": "Gostei, mas a cor é um pouco diferente...", "midiasUrls": [], "status": "PENDENTE\_MODERACAO", ... } }
* AvaliacaoAprovada (v1): { ..., "aggregateId": "review-uuid-777", "payload": { "avaliacaoId": "...", "statusNovo": "APROVADA", "moderadorId": "admin-user-mod", ... } }
* AvaliacaoRejeitada (v1): { ..., "aggregateId": "review-uuid-888", "payload": { "avaliacaoId": "...", "statusNovo": "REJEITADA", "motivo": "SPAM", "moderadorId": "...", ... } }
* RespostaPublicada (v1): { ..., "aggregateId": "review-uuid-777", "payload": { "avaliacaoId": "...", "resposta": { "autorTipo": "VENDEDOR", "autorId": "seller-user-abc", "autorNomeExibicao": "Loja XYZ", "texto": "Lamentamos pela diferença na cor...", "timestamp": "..." } } }
* MediaEntidadeAtualizada (v1): { ..., "aggregateId": "prod-xyz", "payload": { "entidade": { "tipo": "PRODUTO", "id": "prod-xyz" }, "tenantId": "...", "metricas": { "notaMedia": 4.2, "totalAvaliacoes": 55, "distribuicao": { "5": 30, "4": 15, "3": 5, "2": 3, "1": 2 } }, ... } }

**4. Modelagem dos Componentes (DDD Tático)**

* **Aggregates:**
  + **Avaliacao Aggregate:** (Representa uma avaliação individual e sua possível resposta)
    - **Root:** Avaliacao (Entity)
    - **ID:** AvaliacaoId (VO, UUID)
    - **Atributos:** TenantId (VO), EntidadeAvaliada (VO: Tipo, Id), Autor (VO: UserId, NomeExibicao), PedidoIdOrigem (VO), Nota (int), Titulo (String), Comentario (String), Status (Enum VO: PendenteModeracao, Aprovada, Rejeitada, Removida), TimestampSubmissao, TimestampModeracao.
    - **Value Objects Internos:** List<UrlMidia> (VOs), RespostaAvaliacao (VO: AutorResposta, Texto, StatusResposta, TimestampResposta).
    - **Invariantes:** Nota dentro da faixa permitida. Status transita corretamente. Resposta só pode ser adicionada a avaliação Aprovada.
    - **Comandos:** AprovarModeracao, RejeitarModeracao (com motivo), AdicionarResposta, AprovarResposta, RejeitarResposta, RemoverAvaliacao. (Comando SubmeterAvaliacao seria tratado por um Application Service que cria o Aggregate).
    - **Eventos:** AvaliacaoSubmetida, AvaliacaoAprovada, AvaliacaoRejeitada, AvaliacaoRemovida, RespostaSubmetida, RespostaAprovada, RespostaRejeitada.
  + **EntidadeAvaliadaMetricas Aggregate:** (Mantém as métricas agregadas; alternativa: projeção com lógica de escrita)
    - **Root:** EntidadeAvaliadaMetricas (Entity)
    - **ID:** Composto TenantId + TipoEntidade + EntidadeId (VO).
    - **Atributos:** NotaMedia (Decimal), TotalAvaliacoes (int), SomaNotas (long), DistribuicaoNotas (Map<Integer, Integer> VO), Version.
    - **Invariantes:** Métricas consistentes (Média = SomaNotas / TotalAvaliacoes). Contagens >= 0.
    - **Comandos (internos):** AdicionarNotaAprovada (recebe a nota da nova avaliação), RemoverNota (quando avaliação é removida/rejeitada após aprovação). Esses comandos recalculam as métricas.
    - **Eventos:** MediaEntidadeAtualizada.
* **Value Objects:** AvaliacaoId, TenantId, EntidadeAvaliada (Tipo, Id), Autor (UserId, Nome), PedidoId, Nota, StatusAvaliacao, UrlMidia, RespostaAvaliacao, StatusResposta, AutorResposta, EntidadeAvaliadaMetricasId, DistribuicaoNotas.
* **Entities:** Avaliacao, EntidadeAvaliadaMetricas.
* **CQRS:**
  + **Write Side:** Aggregates Avaliacao e EntidadeAvaliadaMetricas. **PostgreSQL** é uma boa escolha, armazenando avaliações textuais, notas, status e as métricas calculadas. JSONB útil para DistribuicaoNotas ou Midias. **MongoDB** também se encaixa bem pela natureza documental das avaliações. Eventos publicados no RabbitMQ.
  + **Read Side (Projections):**
    - **ProjecaoAvaliacoesPublicadas (Elasticsearch / PostgreSQL / MongoDB):** A projeção principal para exibição. Contém avaliações com status "Aprovada" e suas respostas aprovadas. Indexada por EntidadeAvaliadaId. **Elasticsearch** é ideal se busca textual, filtragem por nota, ou ordenação complexa forem necessárias. Populada por AvaliacaoAprovada, RespostaPublicada, AvaliacaoRemovida.
    - **ProjecaoMetricasEntidade (Redis / PostgreSQL / MongoDB):** Cache rápido (Chave metrics:{tenantId}:{tipo}:{id} -> {notaMedia, total}) em **Redis** para exibição rápida na listagem/detalhe do produto/vendedor. A projeção completa com distribuição pode ficar em PostgreSQL/MongoDB. Populada por MediaEntidadeAtualizada.
    - **ProjecaoPainelModeracao (PostgreSQL / MongoDB):** Lista de avaliações e respostas com status "PendenteModeracao" para a interface dos moderadores. Populada por AvaliacaoSubmetida, RespostaSubmetida e limpa/atualizada por AvaliacaoAprovada/Rejeitada, etc.

**5. Sagas / Process Managers**

* **ProcessManagerAtualizacaoMetricas:**
  + **Trigger:** AvaliacaoAprovada, AvaliacaoRemovida.
  + **Lógica:** Envia comando (AdicionarNotaAprovada / RemoverNota) para o Aggregate/Projeção EntidadeAvaliadaMetricas.
* **(Potencial) SagaConviteParaAvaliar:**
  + **Trigger:** PedidoEntregue.
  + **Lógica:** Verifica se o cliente já foi convidado para este pedido. Se não, agenda (com delay) o envio de uma notificação (via BC Notificações) e marca como convidado (pode ser no próprio BC Pedidos ou um estado gerenciado pela Saga).

**6. Considerações Adicionais**

* **Moderação:** Definir claramente as políticas e o fluxo de moderação (manual, auto, híbrido) é essencial. Ferramentas para moderadores devem ser eficientes.
* **Prevenção de Fraude/Spam:** Implementar mecanismos para evitar avaliações falsas ou spam (validação de compra, captchas, limites de submissão, análise de padrões).
* **Performance de Leitura:** As métricas (nota média) e as primeiras avaliações são exibidas em muitas páginas. O cache (Redis) para métricas e uma projeção eficiente (Elasticsearch?) para as avaliações são importantes. Paginação nas avaliações é necessária.
* **Cálculo da Média:** A simples média aritmética é comum, mas podem existir algoritmos mais complexos (ex: ponderado por tempo, Wilson score). A lógica de cálculo deve ser clara e consistente.
* **Escalabilidade:** Plataformas com muitos produtos e usuários podem gerar milhões de avaliações. A arquitetura de persistência e consulta deve suportar esse volume.

**Próxima Tarefa:**

Exploramos o Contexto de Avaliações. Com isso, cobrimos a maioria dos BCs diretamente relacionados à jornada principal do cliente. Uma próxima etapa lógica, especialmente relevante para o modelo multi-tenant/multi-vendedor, seria detalhar o **Contexto de Gerenciamento de Vendedor (Seller Management)**.

Alternativamente, podemos começar a abordar: a) **Context Mapping:** Mapear formalmente as relações entre todos os BCs que discutimos. b) **Cross-Cutting Concerns:** Detalhar aspectos como Observability, o fluxo de Autenticação/Autorização ponta-a-ponta, design das APIs (REST/GraphQL/CLI), etc.

O detalhamento do Contexto de Avaliações foi satisfatório? Qual dos próximos tópicos (Seller Management, Context Mapping, Cross-Cutting Concerns) você prefere abordar agora?