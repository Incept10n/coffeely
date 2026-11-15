# syntax=docker/dockerfile:1
# coffeely backend: статический бинарник на alpine (CGO off), непривилегированный юзер.
FROM golang:1.27-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 app

WORKDIR /app
COPY --from=build /out/server /out/migrate /app/
COPY migrations /app/migrations

USER app
EXPOSE 8080
ENTRYPOINT ["/app/server"]
