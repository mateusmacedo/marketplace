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