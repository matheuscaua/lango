# CLAUDE.md

## Este projeto

**lango** é o gateway de integração WhatsApp, extraído do haraka — ver
`docs/adr/001_origin.md` e a ADR-008 do haraka
(`../haraka/docs/adr/008_whatsapp_gateway_extraction.md`) para o contexto
completo da decisão.

Responsabilidade: falar com os providers (Meta Cloud API, Evolution API,
Twilio), cadastrar/gerenciar integrações (números), e rotear mensagens
inbound/outbound para os consumidores que se registraram (hoje: só o haraka).
**Não sabe nada sobre cardápio, pedidos, ou qualquer domínio de negócio de
qualquer consumidor** — isso é fronteira dura, não uma convenção informal.

- **Go:** ver `go.mod`
- **Module:** `github.com/kituomenyu/lango`

## Comandos principais

```bash
make setup   # go mod download + instalar ferramentas
make lint    # gofmt + goimports + golangci-lint
make test    # go test -race -coverprofile=coverage.out ./...
make build   # go build -o bin/ ./cmd/api
```

## Estrutura

```
cmd/api/       # entrypoint
internal/      # código privado do módulo
pkg/           # código reutilizável
migrations/    # goose SQL migrations
docs/adr/      # decisões de arquitetura
```

## Conceitos centrais

- **Consumer**: um serviço externo que fala com o lango (hoje: `haraka`).
  Autenticado por API key própria (header `X-Lango-Api-Key`), nunca um
  token global.
- **Integration**: um número/canal WhatsApp, pertence a exatamente um
  Consumer. Toda operação valida ownership — um consumer nunca acessa a
  integração de outro.
- **MessageAuditEntry**: trilha append-only de todo envio/recebimento,
  nunca apagada. Ver `internal/domain/audit.go`.
