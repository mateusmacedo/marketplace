**Deployment e Infraestrutura Parte 3: Pipelines de CI/CD**

**1. Introdução: Automatizando o Fluxo de Entrega**

Ter múltiplos microsserviços exige um processo robusto e automatizado para construir, testar e implantar cada um deles. É aqui que entram as práticas de **CI/CD (Continuous Integration / Continuous Delivery ou Continuous Deployment)**.

* **Integração Contínua (CI - Continuous Integration):** A prática de mesclar as alterações de código de todos os desenvolvedores em um repositório central *frequentemente* (várias vezes ao dia). Cada mesclagem dispara um **build automatizado** e a execução de **testes automatizados**. O objetivo é detectar problemas de integração, bugs e conflitos o mais cedo possível no ciclo de desenvolvimento.
* **Entrega Contínua (Continuous Delivery):** Estende a CI, garantindo que, após passar por todos os testes automatizados, o software (no nosso caso, a imagem Docker do microsserviço) esteja *sempre* em um estado **pronto para ser implantado** em produção com um simples clique (ou comando). A decisão de implantar em produção ainda pode ser manual.
* **Implantação Contínua (Continuous Deployment):** O passo final da automação. Cada alteração que passa com sucesso por todo o pipeline de CI/CD é **automaticamente implantada em produção**, sem intervenção manual. Requer um alto grau de confiança nos testes automatizados e monitoramento robusto.

**Benefícios da CI/CD para nossa Arquitetura:**

* **Feedback Rápido:** Desenvolvedores descobrem erros rapidamente.
* **Redução de Riscos:** Menos erros manuais no processo de build e deploy.
* **Consistência:** Implantações são feitas sempre da mesma forma.
* **Velocidade:** Permite entregar novas features e correções aos usuários muito mais rapidamente.
* **Gerenciabilidade:** Essencial para lidar com a complexidade de múltiplos serviços sendo atualizados independentemente.

**2. Etapas Chave de um Pipeline de CI/CD (Exemplo para um Microsserviço)**

Vamos descrever um fluxo típico de pipeline que seria acionado, por exemplo, quando um desenvolvedor mescla código na branch principal (ex: main ou master) do repositório Git de um microsserviço (ex: catalog-service):

1. **Checkout do Código Fonte:** O servidor de CI/CD obtém a versão mais recente do código do repositório Git (GitHub, GitLab, Bitbucket, etc.).
2. **Gerenciamento de Dependências:** Baixa e armazena em cache as dependências do projeto (ex: via mvn dependency:go-offline ou equivalente Gradle) para acelerar builds futuros.
3. **Verificações de Qualidade de Código (Análise Estática):**
   * Executa ferramentas como SonarQube (para análise abrangente), Checkstyle (para padrões de codificação), PMD (para detecção de código "ruim").
   * O build **falha** se as métricas de qualidade ou as regras definidas (Quality Gates no SonarQube, por exemplo) não forem atendidas.
4. **Testes Automatizados (Unitários e Integração):**
   * **Testes Unitários:** Executa testes rápidos que verificam pequenas unidades de código isoladamente (ex: métodos de uma classe), usando mocks para dependências externas (mvn test).
   * **Testes de Integração:** Executa testes que verificam a interação entre componentes dentro do microsserviço (ex: serviço -> repositório -> banco de dados de teste). Quarkus facilita com @QuarkusTest, que sobe uma instância da aplicação. Pode usar **Testcontainers** para instanciar dependências reais (PostgreSQL, RabbitMQ) em containers Docker durante os testes (mvn verify ou task Gradle separada).
   * O build **falha** se qualquer teste falhar. A cobertura de código (ex: via JaCoCo) também é frequentemente medida aqui.
5. **Build do Artefato da Aplicação:**
   * Compila o código fonte Java.
   * Empacota a aplicação:
     + Opção A: Gera o JAR executável (quarkus-app/quarkus-run.jar via mvn package).
     + Opção B (Recomendada): Compila para **Executável Nativo** usando GraalVM (mvn package -Pnative). Este passo pode levar mais tempo.
6. **Build e Push da Imagem Docker:**
   * Utiliza o Dockerfile do projeto e o artefato gerado (JAR ou nativo) para construir a imagem Docker.
   * Aplica tags à imagem (ex: meuregistro/catalog-service:1.3.2, meuregistro/catalog-service:{git-commit-sha}).
   * Autentica-se no Container Registry (ECR, GCR, Docker Hub, etc.).
   * Envia (push) a imagem tagueada para o Registry.
7. **Deploy no Ambiente de Staging (Entrega/Implantação Contínua):**
   * O pipeline aciona a implantação da nova imagem no ambiente de Staging (Namespace Kubernetes marketplace-staging).
   * Utiliza ferramentas como kubectl apply -f deployment.yaml (atualizando a tag da imagem), **Helm** (gerenciador de pacotes para K8s), **Argo CD** / **Flux CD** (ferramentas GitOps K8s-native), ou funcionalidades da própria plataforma de CI/CD (GitLab CI/CD Kubernetes integration, GitHub Actions deploy to K8s).
   * O Kubernetes executa a estratégia de atualização configurada no Deployment (geralmente Rolling Update).
8. **Testes Automatizados em Staging:**
   * Após o deploy em staging, executa testes mais abrangentes:
     + **Testes End-to-End (E2E):** Simulam fluxos de usuário completos através da interface gráfica ou APIs públicas, interagindo com múltiplos serviços. Ferramentas: Cypress, Playwright, Selenium.
     + **Testes de Contrato de API:** Verificam se os contratos entre serviços (APIs REST, gRPC, eventos) ainda são compatíveis após a mudança. Ferramentas: Pact.io.
     + **(Opcional) Testes de Carga/Performance:** Executados periodicamente ou sob demanda para verificar o desempenho sob carga simulada.
9. **Promoção para Produção (Entrega/Implantação Contínua):**
   * **Portão de Aprovação Manual (Continuous Delivery):** Em muitos casos, especialmente no início, há um passo de aprovação manual aqui. Um líder técnico, gerente de produto ou QA revisa os resultados dos testes de staging e autoriza o deploy em produção.
   * **Promoção Automática (Continuous Deployment):** Se a confiança nos testes automatizados for muito alta, o pipeline pode prosseguir automaticamente para o deploy em produção após o sucesso em staging.
   * **Deploy em Produção:** Usa as mesmas ferramentas do passo 7, mas apontando para o ambiente de Produção (Namespace marketplace-prod).
   * **Estratégias de Deploy em Produção:** Para minimizar riscos:
     + *Rolling Update:* Padrão K8s, substitui Pods antigos por novos gradualmente. Rápido, mas pode ter um período com versões mistas.
     + *Blue/Green Deployment:* Prepara a nova versão (Green) ao lado da antiga (Blue). Quando pronta, o tráfego é chaveado instantaneamente para a Green. Rollback rápido (voltar para Blue). Requer mais recursos temporariamente.
     + *Canary Release:* Libera a nova versão para um pequeno subconjunto de usuários/tráfego. Monitora de perto. Se tudo OK, aumenta gradualmente a exposição até 100%. Permite detectar problemas com impacto limitado. Frequentemente implementado com Service Mesh (Istio, Linkerd) ou ferramentas como Argo Rollouts.
10. **Monitoramento Pós-Deploy:**
    * Opcionalmente, o pipeline pode executar "smoke tests" básicos em produção para verificar a saúde da nova versão.
    * É crucial monitorar de perto os dashboards de Observability (Grafana) nos minutos/horas seguintes ao deploy para detectar anomalias (aumento de erros, latência) que possam não ter sido pegas pelos testes.
    * Ter a capacidade de fazer **rollback automático ou rápido** para a versão anterior é fundamental se problemas sérios forem detectados.

**3. Ferramentas de CI/CD**

A escolha da ferramenta depende de muitos fatores (custo, integração com K8s, preferência da equipe, self-hosted vs SaaS):

* **Opções Populares:** GitLab CI/CD (muito integrado ao Git e K8s), GitHub Actions (flexível, ótimo ecossistema), Jenkins (clássico, muito extensível, mas pode ser complexo de gerenciar), Tekton (K8s-native, baseado em CRDs), Argo Workflows/Events/CD (K8s-native, forte em GitOps), Flux CD (K8s-native, GitOps), AWS CodePipeline, Google Cloud Build, Azure DevOps.

**4. Estrutura do Pipeline (Monorepo vs. Polyrepo)**

* **Monorepo:** Todos os microsserviços no mesmo repositório Git.
  + *Prós:* Refatorações cross-service mais fáceis, gerenciamento de dependências unificado, uma única fonte da verdade.
  + *Contras:* Pipelines podem ficar complexos (precisam detectar quais serviços mudaram para buildar/testar/deployar apenas eles), controle de acesso granular pode ser mais difícil, tempo de build pode aumentar se não otimizado. Requer ferramentas que suportem bem monorepos (ex: Bazel, Nx, ou scripts customizados na CI).
* **Polyrepo:** Cada microsserviço em seu próprio repositório Git.
  + *Prós:* Pipelines individuais mais simples, clareza de propriedade, controle de acesso fácil por repo.
  + *Contras:* Gerenciamento de dependências entre repos pode ser complexo (bibliotecas compartilhadas precisam ser versionadas e publicadas), descobrir quais serviços precisam ser atualizados juntos pode ser difícil, refatorações cross-service exigem coordenação entre múltiplos PRs/repos.
* **Escolha:** Ambas são viáveis. A decisão depende da cultura da equipe e do ferramental disponível. Monorepos estão ganhando popularidade com boas ferramentas de suporte.

**5. Gerenciamento de Infraestrutura e Migrações na CI/CD**

* Mudanças na infraestrutura (próximo tópico: IaC) também devem ser gerenciadas via pipeline.
* **Migrações de Banco de Dados (Flyway/Liquibase):** Devem ser executadas como um passo **antes** do deploy da nova versão da aplicação que depende do schema atualizado. Pode ser um job separado no pipeline ou um Init Container no manifesto K8s do serviço.

**6. Gerenciamento de Segredos na CI/CD**

* O pipeline precisa de acesso a segredos para operar (ex: credenciais do Container Registry, credenciais para deployar no K8s, chaves de API para testes E2E).
* Utilizar os mecanismos seguros da própria ferramenta de CI/CD (ex: GitLab CI/CD Variables (masked), GitHub Secrets, Jenkins Credentials) ou, preferencialmente, integrar a pipeline com um sistema externo como HashiCorp Vault ou o Secrets Manager do provedor de nuvem para obter segredos dinamicamente em tempo de execução. **Nunca commitar segredos no código ou na definição do pipeline.**

**Conclusão Parcial:** Uma pipeline de CI/CD bem definida e automatizada é a espinha dorsal para entregar e operar nossa arquitetura de microsserviços de forma eficiente, rápida e segura. Ela automatiza tarefas repetitivas, impõe qualidade e permite que a equipe foque em entregar valor de negócio.