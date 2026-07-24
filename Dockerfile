# syntax=docker/dockerfile:1.7

# ---------- Builder ----------
FROM golang:1.25-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git

# Cache dependencies separately from source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ENV CGO_ENABLED=0 GOOS=linux

RUN go build \
    -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/server \
    ./cmd/server

# ---------- Runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/server /server

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/server"]
