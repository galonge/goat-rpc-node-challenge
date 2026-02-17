# stage 1: build the Go binary
FROM golang:1.22-alpine AS builder

# install CA certificates for HTTPS RPC calls
RUN apk add --no-cache ca-certificates

WORKDIR /build

# cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

# copy source and build static binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /goat-monitor \
    .

# stage 2: minimal runtime image
FROM alpine:3.19

# install CA certificates for HTTPS RPC calls and wget for healthcheck
RUN apk add --no-cache ca-certificates wget

# run as non-root user
RUN adduser -D -u 1000 monitor
USER monitor

# copy the compiled binary from the builder stage
COPY --from=builder /goat-monitor /usr/local/bin/goat-monitor

EXPOSE 9090

HEALTHCHECK --interval=15s --timeout=5s --retries=3 \
    CMD wget --spider --quiet http://localhost:9090/health || exit 1

ENTRYPOINT ["goat-monitor"]
