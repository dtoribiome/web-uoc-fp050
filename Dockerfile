# ---------- Etapa 1: compilación ----------
FROM golang:alpine AS build
WORKDIR /app

# Copiamos primero go.mod para aprovechar la caché de capas
COPY go.mod ./
RUN go mod download

# Copiamos el resto del código y compilamos un binario estático
# (plantilla e imagen van incrustadas dentro gracias a go:embed)
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o servidor .

# ---------- Etapa 2: imagen final ----------
FROM alpine:latest
WORKDIR /app

# Usuario sin privilegios (no ejecutar como root)
RUN adduser -D -u 10001 appuser
COPY --from=build /app/servidor .
USER appuser

ENV PORT=8080
EXPOSE 8080

# Docker comprueba cada 30 s que la web responde
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["./servidor", "-healthcheck"]

CMD ["./servidor"]
