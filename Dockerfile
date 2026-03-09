# Stage 1 - Builder
FROM golang:1.24-alpine AS builder

ARG VERSION=dev

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /grepai ./cmd/grepai

# Stage 2 - Runtime
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder /grepai /grepai

USER 65534

WORKDIR /workspace

ENTRYPOINT ["/grepai", "watch", "--no-ui", "--auto-init"]
