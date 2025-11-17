FROM golang:1.22 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/openai-bridge ./cmd/openai-bridge

FROM gcr.io/distroless/base-debian12:nonroot

COPY --from=build /out/openai-bridge /usr/local/bin/openai-bridge

WORKDIR /data
VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/openai-bridge", "--config", "/data/config.yaml", "--registration", "/data/registration.yaml"]
