FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /coredns-manager .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /coredns-manager /usr/local/bin/coredns-manager
COPY templates/ /app/templates/

WORKDIR /app

EXPOSE 8080

ENTRYPOINT ["coredns-manager"]
