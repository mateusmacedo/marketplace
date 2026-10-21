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