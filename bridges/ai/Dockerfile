FROM golang:1-alpine3.23 AS builder
RUN apk add --no-cache git ca-certificates build-base su-exec olm-dev
COPY . /build
WORKDIR /build/bridges/ai
RUN go build -o /build/ai-bridge ./cmd/bridge

FROM alpine:3.23
RUN apk add --no-cache su-exec ca-certificates olm bash jq yq-go curl
COPY --from=builder /build/ai-bridge /usr/bin/ai-bridge
COPY docker-run.sh /docker-run.sh
VOLUME /data
ENV UID=1337 GID=1337 BINARY_NAME=/usr/bin/ai-bridge
CMD ["/docker-run.sh"]
