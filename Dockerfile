# Build a single static binary (pure-Go SQLite driver, no CGO).
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /durstworld ./cmd/durstworld

FROM alpine:3.21
WORKDIR /app
COPY --from=build /durstworld /app/durstworld
# mount these two volumes to persist the host key and the SQLite DB:
#   -v ./.ssh:/app/.ssh  -v ./data:/app/data
VOLUME ["/app/.ssh", "/app/data"]
# 2222 is the SSH world; 8080 serves the browser client (WEB_PORT=off to
# disable it and stay SSH-only).
EXPOSE 2222 8080
ENV PORT=2222
ENV WEB_PORT=8080
CMD ["/app/durstworld"]
