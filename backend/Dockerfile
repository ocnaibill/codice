# Códice API. Two static binaries: the server and the local recovery command.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/codice-api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/codice-admin ./cmd/codice-admin

FROM alpine:3.21
# ca-certificates: to reach metadata providers over HTTPS. tzdata: readable timestamps.
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S -g 10001 codice && adduser -S -u 10001 -G codice codice \
 && mkdir -p /app/storage && chown codice:codice /app/storage
COPY --from=build /out/ /usr/local/bin/
USER codice
WORKDIR /app
ENV PORT=8080 CODICE_STORAGE_PATH=/app/storage
EXPOSE 8080
# Owner recovery runs inside this image: docker compose exec backend codice-admin help
HEALTHCHECK --interval=15s --timeout=4s --start-period=20s --retries=5 \
  CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
CMD ["codice-api"]
