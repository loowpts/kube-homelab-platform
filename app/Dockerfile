FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20
# Зеркало пакетов Alpine (dl-cdn из РФ работает нестабильно); переопределяется
# docker build --build-arg ALPINE_MIRROR=dl-cdn.alpinelinux.org/alpine
ARG ALPINE_MIRROR=mirror.yandex.ru/mirrors/alpine
RUN sed -i "s#dl-cdn.alpinelinux.org/alpine#${ALPINE_MIRROR}#" /etc/apk/repositories && \
    apk add --no-cache ca-certificates && \
    addgroup -S -g 10001 app && adduser -S -u 10001 -G app app
WORKDIR /app

COPY --from=build /out/server ./server
COPY --from=build /src/web ./web

# Числовой UID: иначе securityContext.runAsNonRoot в k8s не сможет проверить пользователя
USER 10001:10001
EXPOSE 8080 9090
ENTRYPOINT ["./server"]
