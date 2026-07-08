# syntax=docker/dockerfile:1
# ═══════════════════════════════════════════════════════════
# lango — WhatsApp gateway (Go). Single HTTP binary (Fiber, reads PORT).
# Receives provider webhooks (Twilio/Evolution/Meta) and routes messages
# to/from its consumers (haraka today). Provider-agnostic.
# ═══════════════════════════════════════════════════════════

FROM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/api ./cmd/api

FROM alpine:3.20 AS production
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/api /app/api
# Migrations ship with the image so they can be applied as a release step
# (goose) against the managed Postgres — see docs/DEPLOY.md.
COPY migrations ./migrations
CMD ["/app/api"]
