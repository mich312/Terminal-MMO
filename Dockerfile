# Build a single static binary (pure-Go SQLite driver, no CGO).
FROM golang:1.24-alpine AS build
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
EXPOSE 2222
ENV PORT=2222
CMD ["/app/durstworld"]
