**Deployment e Infraestrutura Parte 2: Orquestração de Containers com Kubernetes**

**1. Introdução: Por Que Orquestração?**

Empacotar nossos microsserviços e componentes de infraestrutura em containers Docker (como discutido na Parte 1) resolve o problema de consistência e portabilidade. No entanto, gerenciar manualmente dezenas ou centenas de containers em múltiplos servidores (hosts) é impraticável para um ambiente de produção. Precisamos de um sistema para automatizar:

* **Implantação (Deployment):** Colocar as versões corretas dos containers nos hosts certos.
* **Escalonamento (Scaling):** Ajustar o número de instâncias de cada container com base na demanda.
* **Rede (Networking):** Permitir que containers se comuniquem entre si e com o mundo exterior de forma confiável.
* **Autocorreção (Self-healing):** Reiniciar containers que falharam ou substituí-los.
* **Descoberta de Serviço (Service Discovery):** Permitir que serviços encontrem uns aos outros dinamicamente.
* **Gerenciamento de Configuração e Segredos:** Injetar configurações e dados sensíveis nos containers de forma segura.
* **Gerenciamento de Armazenamento (Storage):** Anexar armazenamento persistente a containers que precisam (ex: bancos de dados).

É aqui que entram os **Orquestradores de Containers**. A ferramenta que se tornou o padrão de fato da indústria para isso é o **Kubernetes (K8s)**. Embora existam alternativas (Docker Swarm, HashiCorp Nomad), Kubernetes oferece o ecossistema mais rico e é a escolha mais comum para arquiteturas de microsserviços como a nossa. Portanto, basearemos nossa estratégia de orquestração no Kubernetes.

**2. Conceitos Fundamentais do Kubernetes Aplicados à Nossa Arquitetura**

Entender os objetos principais do Kubernetes nos ajuda a visualizar como nossa aplicação será executada:

* **Cluster:** O conjunto completo de máquinas (Nós) que executam nossos containers, gerenciado por um Control Plane (que também roda no cluster ou é gerenciado externamente). Para produção, é **altamente recomendável usar um serviço Kubernetes gerenciado** oferecido pelos provedores de nuvem (AWS EKS, Google GKE, Azure AKS), pois eles cuidam da complexidade do Control Plane, upgrades e gerenciamento dos nós.
* **Node (Nó):** Uma máquina de trabalho (VM ou física) dentro do Cluster. O Kubernetes distribui a carga de trabalho (containers) entre os Nós disponíveis.
* **Pod:** A **menor unidade implantável** no Kubernetes. Um Pod encapsula um ou mais containers que são co-localizados, compartilham recursos de rede e armazenamento, e têm um ciclo de vida acoplado. **Regra geral:** Usaremos um container principal por Pod para cada instância de nossos microsserviços (ex: um Pod para o order-service, outro para o catalog-service). Podemos adicionar containers "sidecar" ao Pod para tarefas auxiliares (ex: OTel Collector, um proxy de Service Mesh como Istio/Linkerd, ou o agente Promtail para logs).
* **Deployment:** Objeto declarativo que gerencia um conjunto de Pods replicados (Réplicas) de uma aplicação **stateless**. Você define o estado desejado (ex: "quero 3 réplicas do payment-service usando a imagem meurepo/payment-service:v1.2.0") e o Kubernetes trabalha para manter esse estado. Ele gerencia atualizações (rolling updates, canary), rollbacks e garante que o número desejado de Pods esteja sempre rodando. **É como implantaremos nossos microsserviços Java Quarkus (que são stateless).**
* **StatefulSet:** Similar ao Deployment, mas projetado especificamente para aplicações **stateful** (como bancos de dados, message brokers) que precisam de:
  + Identificadores de rede estáveis e únicos para cada Pod.
  + Armazenamento persistente estável e único por Pod.
  + Implantação e escalonamento ordenados e graciosos.
  + **É como implantaríamos PostgreSQL, MongoDB, RabbitMQ, Redis, Logto (se auto-hospedado), Prometheus, Loki, Tempo** *se* decidíssemos rodá-los dentro do cluster Kubernetes. (Veja nota abaixo).
* **Service:** Abstração que define um conjunto lógico de Pods (selecionados via labels) e uma política para acessá-los. Fornece um **endereço IP estável e um nome DNS interno** para esse conjunto de Pods. Atua como um balanceador de carga interno. Essencial para que os serviços se descubram e comuniquem confiavelmente, mesmo que os Pods individuais sejam criados, destruídos ou movidos entre Nós. Ex: Um Service chamado catalog-service no namespace dev pode ser acessado internamente via DNS catalog-service.dev.svc.cluster.local. Os clientes gRPC/REST do Quarkus usarão esses nomes de serviço.
* **Ingress / Gateway API (e Traefik):** Enquanto o Service lida com a comunicação *interna*, o **Ingress** (ou a API mais moderna **Gateway API**) gerencia o acesso *externo* (HTTP/S) *para dentro* do cluster. Define regras de roteamento baseadas em hostnames (ex: tenantA.meumarketplace.com) e paths (ex: /api/v1/orders, /graphql) para direcionar o tráfego externo aos Services internos corretos. **Traefik**, em nossa arquitetura, atuará como um **Ingress Controller**: ele monitora os recursos Ingress/Gateway criados no cluster e configura a si mesmo dinamicamente para aplicar essas regras de roteamento. É a porta de entrada para as chamadas das UIs e clientes externos.
* **ConfigMap:** Usado para armazenar dados de configuração **não sensíveis** (pares chave-valor ou arquivos de texto) desacoplados da imagem do container. Podem ser injetados nos Pods como variáveis de ambiente ou montados como arquivos (ex: para sobrescrever partes do application.properties).
* **Secret:** Similar ao ConfigMap, mas projetado especificamente para armazenar dados **sensíveis** (senhas, tokens de API, certificados TLS). O Kubernetes os armazena de forma mais segura (embora a segurança exata dependa da configuração do cluster) e podem ser injetados nos Pods como variáveis de ambiente ou montados como arquivos. É a forma padrão de passar segredos para as aplicações. **Recomendado integrar com um Vault externo para gerenciamento centralizado de segredos.**
* **PersistentVolume (PV) e PersistentVolumeClaim (PVC):** Abstração para gerenciamento de armazenamento persistente. Um PVC é uma *solicitação* de armazenamento feita por um Pod (geralmente via StatefulSet), e o Kubernetes a *atende* vinculando-a a um PV disponível (que representa um disco físico real na nuvem – AWS EBS, GCE PD, Azure Disk – ou armazenamento de rede como NFS, Ceph). Essencial para os dados dos StatefulSets (bancos de dados, filas).
* **Namespace:** Permite criar "clusters virtuais" dentro de um cluster físico. Útil para isolar ambientes (ex: dev, staging, prod), times ou aplicações. Recursos dentro de um namespace precisam usar nomes qualificados para acessar recursos em outro.
* **Horizontal Pod Autoscaler (HPA):** Objeto que ajusta automaticamente o número de réplicas em um Deployment ou StatefulSet com base em métricas observadas. Pode escalar com base no uso de CPU/Memória dos Pods, ou em métricas customizadas (ex: profundidade de uma fila RabbitMQ, número de requisições por segundo – obtidas via Prometheus com o KEDA - Kubernetes Event-driven Autoscaling). Essencial para lidar com cargas variáveis de forma eficiente.

**3. Estratégia de Deployment no Kubernetes**

* **Microsserviços (BCs Stateless):** Cada BC (Order, Catalog, Payment, etc.) será um Deployment Kubernetes. O número de replicas será gerenciado por um HorizontalPodAutoscaler (HPA) baseado em CPU/Memória ou métricas customizadas relevantes.
* **Componentes Stateful (Se Auto-Hospedados):** PostgreSQL, MongoDB, Redis, RabbitMQ, Logto, Prometheus, Loki, Tempo seriam gerenciados por StatefulSets, cada um com seu PersistentVolumeClaim para garantir a persistência dos dados. **MAS ATENÇÃO:** Para ambientes de produção, **é fortemente recomendado considerar o uso de serviços gerenciados oferecidos pelos provedores de nuvem** para esses componentes (ex: AWS RDS para PostgreSQL, AWS ElastiCache para Redis, AWS MSK/MQ para Kafka/RabbitMQ, MongoDB Atlas). Isso transfere a complexidade operacional (backups, patching, scaling, alta disponibilidade) para o provedor de nuvem. Se usar serviços gerenciados, esses componentes rodam *fora* do K8s, e nossos serviços no K8s apenas se conectam a eles (via URLs configuradas como Secrets/ConfigMaps).
* **Ferramentas de Suporte:** OTel Collector (pode ser Deployment ou DaemonSet), Grafana (Deployment), etc.
* **Ingress (Traefik):** Traefik rodará como um Deployment (geralmente com múltiplas réplicas) e um Service do tipo LoadBalancer (em nuvem) ou NodePort para expor o tráfego externamente. Ele será configurado como o Ingress Controller padrão para o cluster.
* **Namespaces:** Usar namespaces para isolar os ambientes: marketplace-dev, marketplace-staging, marketplace-prod.

**4. Rede e Descoberta de Serviço**

* **Comunicação Interna:** Serviços se comunicam usando os nomes DNS dos Services do Kubernetes (ex: payment-service.marketplace-prod.svc.cluster.local). O Kube-proxy (ou CNI) em cada nó garante que o tráfego seja roteado para um Pod saudável daquele serviço. Quarkus (com extensões gRPC/REST Client) pode ser configurado para usar essa descoberta de serviço nativa do K8s.
* **Comunicação Externa:** O tráfego externo entra pelo Load Balancer que aponta para o Service do Traefik. Traefik lê os recursos Ingress (ou Gateway) e direciona a requisição para o Service interno apropriado (ex: api-gateway-service ou diretamente para graphql-service). Traefik também lida com a terminação TLS usando certificados gerenciados (ex: via Let's Encrypt ou Secrets).

**5. Integração Quarkus e Kubernetes**

* **Manifestos Gerados:** As extensões quarkus-kubernetes e quarkus-openshift podem analisar o application.properties e gerar automaticamente arquivos de manifesto K8s básicos (Deployment.yaml, Service.yaml, etc.) durante o build, simplificando a criação inicial desses recursos.
* **Health Checks:** Quarkus expõe endpoints de health check (/q/health, /q/live, /q/ready) que são compatíveis com as Liveness Probes e Readiness Probes do Kubernetes. K8s usa essas probes para saber se um container está vivo (deve ser reiniciado?) e se está pronto para receber tráfego. Isso aumenta a resiliência.
* **Native Build:** Compilar para nativo com Quarkus resulta em imagens menores e com startup mais rápido, o que é ideal para ambientes K8s, permitindo escalonamento mais rápido e melhor utilização de recursos dos nós.

**6. Observability no Kubernetes**

* O OTel Collector pode ser implantado como DaemonSet (um por nó, coletando de todos os pods no nó) ou Deployment (centralizado).
* Promtail é tipicamente um DaemonSet para coletar logs de todos os Pods em cada Nó.
* Prometheus, Loki, Tempo e Grafana rodam como Deployment/StatefulSet dentro do cluster (ou como serviços externos/gerenciados).
* O Service Discovery do Prometheus pode ser configurado para encontrar automaticamente os endpoints /metrics dos Pods/Services usando anotações do K8s.

**Conclusão Parcial:** Kubernetes fornecerá a plataforma robusta e escalável necessária para orquestrar nossa complexa aplicação baseada em microsserviços e EDA. Ele automatiza muitas das tarefas operacionais, permitindo focar na lógica de negócio. A escolha entre componentes auto-hospedados (StatefulSets) versus serviços gerenciados (nuvem) para a infraestrutura stateful é uma decisão arquitetural chave com trade-offs entre controle e complexidade operacional.