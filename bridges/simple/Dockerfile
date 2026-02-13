FROM golang:1-alpine3.23 AS builder
RUN apk add --no-cache git ca-certificates build-base su-exec olm-dev
COPY . /build
WORKDIR /build/bridges/simple
RUN go build -o /build/ai-simple ./cmd/bridge

FROM alpine:3.23
RUN apk add --no-cache su-exec ca-certificates olm bash jq yq-go curl
COPY --from=builder /build/ai-simple /usr/bin/ai-simple
COPY docker-run.sh /docker-run.sh
VOLUME /data
ENV UID=1337 GID=1337 BINARY_NAME=/usr/bin/ai-simple
CMD ["/docker-run.sh"]
