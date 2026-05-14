# Build stage
FROM node:20-alpine AS web-builder
WORKDIR /app
RUN corepack enable pnpm

COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY contracts/package.json contracts/
COPY web/package.json web/
RUN pnpm install --frozen-lockfile

COPY contracts/ contracts/
RUN pnpm --filter @continua/contracts build

COPY web/ web/
RUN pnpm --filter web build

# Go build stage
FROM golang:1.24-alpine AS go-builder
WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-builder /app/web/dist ./internal/web/static/

ARG VERSION=dev
ARG COMMIT=unknown

RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /continua ./cmd/continua

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata

COPY --from=go-builder /continua /usr/local/bin/continua

EXPOSE 8080 8081

CMD ["continua", "serve"]
