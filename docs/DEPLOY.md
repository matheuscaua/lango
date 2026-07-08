# Deploy — lango (gateway WhatsApp)

Parte da stack de staging no Railway. Para a **visão geral da topologia**
(frontends no Vercel + backend no Railway, rede interna, plugins), ver
`../../haraka/docs/DEPLOY.md`. Este doc cobre o específico do lango + Evolution.

## Build

`lango/Dockerfile` gera um único binário HTTP (`/app/api`, Fiber, lê `PORT`).
O `railway.json` já define o build por Dockerfile e o healthcheck `/health`.

## Exposição

O `lango-api` **precisa de domínio público** — é ele quem recebe os webhooks dos
providers (Twilio/Meta/Evolution). É o único serviço do backend do WhatsApp que
fica público; haraka e Evolution ficam internos.

## Variáveis de ambiente (lango)

| Var | Valor no Railway |
|---|---|
| `PORT` | injetado pelo Railway (não setar) |
| `LOG_LEVEL` | `info` |
| `DATABASE_URL` | `${{Postgres.DATABASE_URL}}` (DB do lango) |
| `REDIS_URL` | `${{Redis.REDIS_URL}}` |
| `PUBLIC_WEBHOOK_BASE_URL` | domínio **público** do próprio lango-api (ex: `https://lango.staging.seusocio.app`) — usado pra verificar assinatura do webhook do Twilio |
| `WHATSAPP_APP_SECRET` | app secret da Meta (só se usar provider Meta) |
| `EVOLUTION_API_URL` | `http://evolution.railway.internal:8080` (só se usar Evolution) |
| `EVOLUTION_API_KEY` | chave admin da instância Evolution |
| `EVOLUTION_WEBHOOK_BASE_URL` | `http://lango-api.railway.internal:$PORT` (Evolution → lango, interno) |

> Consumers do lango (ex: haraka) autenticam por API key própria
> (`X-Lango-Api-Key`). Cadastre o consumer `haraka` e use a mesma chave em
> `LANGO_API_KEY` no haraka.

## Evolution (só se for o provider escolhido)

Serviço Docker `evoapicloud/evolution-api:v2.3.6` com:
- **Volume persistente** em `/evolution/instances` (sessão do WhatsApp — sem ele,
  precisa reescanear o QR a cada deploy).
- Postgres próprio (`DATABASE_CONNECTION_URI` → DB `evolution`).
- Env equivalente ao `docker-compose.yml` local (AUTHENTICATION_API_KEY, etc.).
- **Interno** (só o lango fala com ele).

> Nota: o Baileys do Evolution é instável (mensagens presas em `PENDING`,
> risco de flag por cliente não-oficial). Para um ambiente compartilhado estável,
> avaliar Meta Cloud API. O lango abstrai os três providers, então a troca não
> mexe na infra.

## Migrations

Igual ao haraka — goose, não roda sozinho. Ver a seção "Migrations" em
`../../haraka/docs/DEPLOY.md`:

```bash
goose -dir migrations postgres "$DATABASE_URL" up
```

## Webhook do provider

Depois do deploy, apontar o webhook do provider pro domínio público do lango:
- **Twilio**: `https://<lango público>/webhooks/twilio/<integration_id>`
- **Evolution**: configurado automaticamente pelo fluxo de conexão via QR.
- **Meta**: `https://<lango público>/webhooks/meta/<integration_id>`
