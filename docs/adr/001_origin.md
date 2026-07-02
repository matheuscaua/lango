# ADR 001: Origem — extração do haraka

## Status

Aceito — 2026-07-02

## Contexto

lango nasceu como uma extração do repositório `haraka`, que até então misturava
integração WhatsApp multi-provider com a lógica do bot determinístico do Kituo
Menyu. A decisão completa — motivação, opções consideradas, fronteira exata
entre os dois serviços, contrato de comunicação, modelo de consumidor e
auditoria — está documentada na ADR de origem, mantida no repo do haraka (é lá
que a decisão foi tomada, com o contexto histórico do serviço que estava sendo
dividido):

**[haraka ADR-008: Extração do Gateway WhatsApp (`lango`) do haraka](../../../haraka/docs/adr/008_whatsapp_gateway_extraction.md)**

## Decisão

Este repositório é o resultado dessa extração. Novas decisões de arquitetura
específicas do lango (que não dizem respeito à fronteira com o haraka) devem
ser registradas como novas ADRs neste diretório, numeradas a partir de 002.

## Consequências

Qualquer mudança na fronteira lango↔haraka (contrato de mensagens, modelo de
consumidor, auditoria) deve ser refletida nos dois repositórios — atualize a
ADR-008 do haraka como fonte da verdade e referencie a partir daqui.
