# PRD — Fulfillment & Logistics

## Propósito

Simplificar a orquestração do despacho, integrando e normalizando as diversas transportadoras em uma camada única, provendo geração de etiquetas e monitoramento de rastreio de pacotes de forma padronizada.

## Escopo

- Integração com sistemas de frete (Ex: MelhorEnvio, Kangu, Loggi, Correios).
- Compra e geração de Etiquetas de Postagem (Labels).
- Ingestão de atualizações de status de transporte (Tracking).
- Roteamento de eventos lógicos para liberar valores em custódia (Escrow).
- Gestão da Declaração de Conteúdo (quando NFe não for aplicável).

## Fora do escopo

- Operação física de frota (motoristas, caminhões).
- Estoque físico em WMS (Warehouse Management System), a menos que evolua para *Fulfillment By Plataform*.

## Modelo de domínio

- `Shipment`: A unidade física a ser despachada.
- `ShippingLabel`: O documento (PDF, ZPL) contendo o código de rastreio e dados de roteamento.
- `TrackingEvent`: Ponto no tempo registrado pela transportadora (ex: "Saiu para entrega").
- `Carrier`: Transportador e suas capacidades.

## Requisitos funcionais

1. Disponibilizar botão/API para o Vendedor gerar a etiqueta assim que o pedido for faturado.
2. Descontar o custo real da etiqueta do saldo futuro do vendedor (se a plataforma pagou) ou validar que o vendedor assumiu o envio (FOB).
3. Poll/Webhooks diários nas transportadoras para sincronizar o status de cada `Shipment` pendente.
4. Traduzir o dicionário de status logísticos de diferentes transportadoras para uma taxonomia padrão da plataforma (`IN_TRANSIT`, `OUT_FOR_DELIVERY`, `DELIVERED`, `EXCEPTION`).
5. Acionar o Order Management comunicando quando um pacote foi entregue.
6. Gerenciar exceções logísticas: endereço não encontrado, extravio e roubo de carga, facilitando as regras de seguro do pacote.

## Estados do Transporte

`LABEL_PENDING → LABEL_CREATED → DISPATCHED → IN_TRANSIT → OUT_FOR_DELIVERY → DELIVERED`

Exceções: `LOST_IN_TRANSIT`, `STOLEN`, `RETURN_TO_SENDER`, `DELIVERY_FAILED`.

## Invariantes

- Etiquetas só podem ser geradas para pedidos com status financeiro `CAPTURED`.
- O status oficial `DELIVERED` exige evidência rastreável externa ou confirmação formal pelo comprador (quando frota própria do vendedor).
- Perdas (`LOST_IN_TRANSIT`) não punem o Vendedor se o frete era de responsabilidade da plataforma, acionando seguro interno.

## Comandos e eventos

Comandos: `PurchaseLabel`, `PrintLabel`, `SyncTracking`, `MarkAsException`.

Eventos: `LabelPurchased`, `ShipmentDispatched`, `ShipmentDelivered`, `ShipmentExceptionDetected`.

## APIs principais

- `POST /shipments/{sellerOrderId}/labels`
- `GET /shipments/{trackingCode}/events`
- `POST /webhooks/carriers` (Endpoints para receber atualizações logísticas)

## Critérios de aceitação

- O vendedor não precisa sair do painel da plataforma para gerar e imprimir o PDF da etiqueta dos Correios.
- Ao receber o webhook da Jadlog de que o pacote foi entregue, o painel do comprador é atualizado, e o ciclo do pedido é encerrado via OMS.
- Eventos da transportadora com nomes esotéricos ("Encaminhado para CTE CEE") são mapeados para `IN_TRANSIT` com o texto original armazenado.

## Métricas

- Prazo Logístico Médio Prometido vs. Realizado (OTIF - On Time In Full).
- Taxa de ocorrências (Extravio, Roubo).
- Custo logístico como % do GMV.
- SLA de geração de etiqueta pelo Seller.

## Dependências

Order Management fornece a ordem para gerar envios. Payment & Split requer os eventos logísticos de sucesso para repassar dinheiro.