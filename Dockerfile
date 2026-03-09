FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /grpc-mcp-server ./cmd/grpc-mcp-server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

RUN adduser -D -s /bin/false mcpuser

COPY --from=builder /grpc-mcp-server /usr/local/bin/grpc-mcp-server

USER mcpuser

ENTRYPOINT ["grpc-mcp-server"]
CMD ["--config", "/etc/grpc-mcp/grpc-mcp.yaml"]
