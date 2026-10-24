**Deployment e Infraestrutura Parte 1: Conceitos Fundamentais e Containerização**

**1. Introdução: Da Arquitetura à Execução**

Até agora, focamos no design lógico e arquitetural dos nossos Bounded Contexts (BCs) e nas interações entre eles. Agora, precisamos definir como esses componentes de software (microsserviços Java Quarkus), juntamente com a infraestrutura de apoio (bancos de dados, message broker, etc.), serão empacotados, implantados, configurados, gerenciados e interconectados em um ambiente real (produção, staging, etc.).

**Objetivos:**

* **Escalabilidade:** Capacidade de aumentar ou diminuir os recursos alocados para cada serviço individualmente, conforme a demanda.
* **Resiliência:** Garantir que a falha de um componente não derrube o sistema inteiro; capacidade de auto-recuperação.
* **Manutenibilidade:** Facilidade para atualizar, corrigir e gerenciar os serviços e a infraestrutura.
* **Implantação Automatizada (CI/CD):** Processos automatizados para build, teste e deploy, reduzindo erros manuais e acelerando a entrega.
* **Segurança:** Proteger a infraestrutura e os dados em todas as camadas.
* **Consistência Ambiental:** Garantir que o ambiente de execução seja o mais similar possível entre desenvolvimento, testes, staging e produção.

**2. Containerização com Docker: O Padrão para Microsserviços**

A base da nossa estratégia de deployment será a **containerização**, utilizando **Docker** como ferramenta principal.

* **Por quê Containers?**
  + **Consistência:** Empacotam a aplicação e *todas* as suas dependências (bibliotecas, runtime) em uma unidade isolada (imagem), garantindo que funcione da mesma forma em qualquer máquina que rode Docker. Adeus ao "funciona na minha máquina!".
  + **Isolamento:** Containers rodam isolados uns dos outros no mesmo host OS, compartilhando o kernel, mas tendo seus próprios filesystems, processos e redes.
  + **Eficiência de Recursos:** Mais leves que máquinas virtuais (VMs), pois não incluem um sistema operacional completo. Permitem maior densidade de aplicações por host.
  + **Imutabilidade e Reprodutibilidade:** Imagens Docker são imutáveis. Uma vez construída, ela não muda, garantindo implantações consistentes.
  + **Portabilidade:** Rodam em qualquer lugar onde o Docker Engine esteja instalado (desenvolvedor local, servidor on-premise, qualquer nuvem).
  + **Ecossistema:** Vasto ecossistema de ferramentas para orquestração (Kubernetes), registro, segurança, etc.
* **Empacotando Nossas Aplicações Quarkus:**
  + Quarkus foi projetado com containers em mente e oferece excelente suporte.
  + **Opções de Build da Imagem:**
    1. **Imagem JVM:** Cria um Dockerfile que usa uma imagem base Java (ex: registry.access.redhat.com/ubi8/openjdk-17, amazoncorretto:17-alpine) e copia o JAR executável gerado pelo Quarkus (na pasta quarkus-app/quarkus-run.jar).
       - *Prós:* Builds mais rápidos, runtime Java padrão.
       - *Contras:* Imagem maior, startup mais lento, maior consumo de memória comparado ao nativo.
    2. **Imagem Nativa (GraalVM):** Utiliza a capacidade do Quarkus de compilar a aplicação Java para um **executável nativo** usando GraalVM.
       - *Prós:* **Startup extremamente rápido** (milissegundos), **consumo de memória significativamente menor**, imagem final potencialmente menor (usando base UBI minimal ou distroless). Ideal para microsserviços, funções serverless e escalonamento rápido.
       - *Contras:* Tempo de build consideravelmente maior, necessidade de configurar testes de integração nativos, pequenas diferenças de comportamento em casos raros (reflexão, etc.) que exigem atenção durante o desenvolvimento.
       - **Recomendação:** Dada a escolha por Quarkus, **visar a compilação Nativa para a maioria dos microsserviços em produção** devido aos ganhos de performance e eficiência de recursos. A imagem JVM pode ser usada para desenvolvimento e testes iniciais.
  + **Exemplo Conceitual Dockerfile (Nativo):**
    # Stage 1: Build Nativo (pode rodar na pipeline de CI/CD)
    FROM quay.io/quarkus/ubi-quarkus-native-image:22.3-java17 AS builder
    COPY --chown=quarkus:quarkus mvnw /code/mvnw
    COPY --chown=quarkus:quarkus .mvn /code/.mvn
    COPY --chown=quarkus:quarkus pom.xml /code/
    USER quarkus
    WORKDIR /code
    RUN ./mvnw -B org.apache.maven.plugins:maven-dependency-plugin:3.1.2:go-offline # Download deps
    COPY src /code/src
    RUN ./mvnw package -Pnative -Dquarkus.native.container-build=true # Build nativo

    # Stage 2: Imagem Final Mínima
    FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
    WORKDIR /work/
    COPY --from=builder /code/target/\*-runner /work/application # Copia o executável nativo
    EXPOSE 8080
    CMD ["./application", "-Dquarkus.http.host=0.0.0.0"]
* **Containerizando a Infraestrutura:** Todos os componentes de infraestrutura (PostgreSQL, MongoDB, Redis, RabbitMQ, Logto - se auto-hospedado, Prometheus, Loki, Tempo, Grafana, Traefik) também serão executados como containers, utilizando imagens oficiais ou confiáveis, para padronização e facilidade de gerenciamento.

**3. Container Registry: Armazenando as Imagens**

Precisamos de um local centralizado e seguro para armazenar nossas imagens Docker construídas: um **Container Registry**.

* **Opções:** Docker Hub (público/privado), AWS Elastic Container Registry (ECR), Google Artifact Registry (GAR) / Container Registry (GCR), Azure Container Registry (ACR), GitHub Packages, Harbor (solução open-source auto-hospedada).
* **Fluxo:** A pipeline de CI/CD (Integração Contínua / Entrega Contínua) será responsável por:
  1. Executar testes.
  2. Construir a imagem Docker (JVM ou Nativa).
  3. Taguear a imagem (ex: com a versão do Git ou um número de release).
  4. Fazer o "push" da imagem para o Container Registry configurado.
  5. O ambiente de deployment (ex: Kubernetes) fará o "pull" da imagem do registry para executar os containers.

**4. Configuração de Ambientes**

Nossos serviços precisam de configurações que mudam entre ambientes (desenvolvimento, staging, produção), como:

* Strings de conexão de banco de dados.
* Endereços do RabbitMQ, Redis, etc.
* Chaves de API para serviços externos (Logto, gateways de pagamento).
* Níveis de log.
* Feature flags específicas do ambiente.

É **fundamental** que essas configurações **não** estejam "hardcoded" na imagem Docker. Estratégias:

* **Variáveis de Ambiente:** A prática mais comum em ambientes containerizados. O valor é injetado no container no momento da execução. Quarkus lê variáveis de ambiente nativamente para sobrescrever propriedades do application.properties. Ex: QUARKUS\_DATASOURCE\_JDBC\_URL como variável de ambiente sobrescreve quarkus.datasource.jdbc.url. Ideal para a maioria das configurações, especialmente segredos.
* **Arquivos de Configuração Montados:** Montar arquivos de configuração (ex: application-prod.properties) como volumes dentro do container em tempo de execução. Útil para configurações muito extensas. Quarkus suporta perfis de configuração.
* **Serviços de Configuração Externa:** Para gerenciamento centralizado e dinâmico, especialmente em larga escala:
  + **HashiCorp Consul:** Armazena K/V e oferece service discovery. Quarkus tem extensão.
  + **Serviços de Nuvem:** AWS Parameter Store, AWS Secrets Manager, Google Secret Manager, Azure Key Vault. Oferecem armazenamento seguro e gerenciado para segredos e configurações. Quarkus também possui extensões para integração.
  + **Recomendação:** Para produção, utilizar um **serviço de configuração externa e/ou secrets manager** é a abordagem mais robusta e segura. Variáveis de ambiente são um bom fallback.
* **Gerenciamento de Segredos (Secrets):** **CRÍTICO!** Dados sensíveis (senhas de banco, chaves de API, certificados TLS, segredos de JWT) NUNCA devem estar no código fonte ou na imagem Docker. Devem ser gerenciados através de:
  + Variáveis de ambiente injetadas a partir de um Secrets Manager (AWS SM, GCP SM, Vault).
  + Arquivos secretos montados como volumes no container (ex: Kubernetes Secrets).
  + Integração direta da aplicação com Vault (Quarkus tem extensão).

**5. Princípio: Serviços Stateless**

Nossos microsserviços (implementando os BCs) devem ser projetados para serem **stateless (sem estado)**. Isso significa que qualquer estado necessário para processar uma requisição ou manter uma sessão deve ser armazenado externamente:

* Estado de sessão de usuário: Armazenado no cliente (ex: JWT) ou em um datastore externo (ex: Redis, se necessário).
* Dados de negócio: Nos bancos de dados (PostgreSQL, MongoDB).
* Estado de fluxo assíncrono: No message broker (RabbitMQ) ou em um banco de dados de Sagas.
* Cache: No Redis.

**Benefícios:** Permite escalar horizontalmente de forma simples (adicionar mais instâncias idênticas do container) e aumenta a resiliência (se uma instância falhar, outra pode assumir a carga sem perda de estado da aplicação).

**6. Migrações de Banco de Dados (Schema Evolution)**

À medida que os BCs evoluem, seus esquemas de banco de dados (PostgreSQL, talvez estruturas em MongoDB) precisarão mudar. É essencial ter um processo controlado para aplicar essas migrações.

* **Ferramentas:** **Flyway** e **Liquibase** são as ferramentas padrão no mundo Java/Quarkus para gerenciamento de migrações de schema de banco de dados.
* **Processo:**
  1. Desenvolvedores escrevem scripts de migração (SQL para Flyway, XML/YAML/JSON/SQL para Liquibase) para cada mudança de schema.
  2. Esses scripts são versionados junto com o código da aplicação.
  3. Na pipeline de CI/CD, antes de implantar a nova versão da aplicação, a ferramenta de migração (Flyway/Liquibase) é executada apontando para o banco de dados do ambiente alvo.
  4. A ferramenta verifica quais migrações já foram aplicadas (usando uma tabela de metadados) e aplica apenas as novas, garantindo que o schema esteja na versão correta esperada pela aplicação que será implantada.