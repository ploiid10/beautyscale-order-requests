FROM golang:1.27-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/server /app/server
COPY migrations /app/migrations
COPY seed /app/seed

ENV HTTP_ADDR=:8080 \
    MIGRATIONS_DIR=/app/migrations \
    SEED_FILE=/app/seed/seed.sql \
    SEED=true

EXPOSE 8080
HEALTHCHECK --interval=2s --timeout=3s --retries=20 CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz
CMD ["/app/server"]
