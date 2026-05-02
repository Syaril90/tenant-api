FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN go build -o /out/api ./cmd/api

FROM alpine:3.20

WORKDIR /app

COPY --from=builder /out/api /app/api

EXPOSE 8080

CMD ["/app/api"]
