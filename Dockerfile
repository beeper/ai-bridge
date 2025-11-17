FROM --platform=$BUILDPLATFORM golang:1.22 AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
ENV GOTOOLCHAIN=auto

RUN go mod download

COPY . .

# libolm headers/libs are required for the mautrix crypto layer.
RUN apt-get update \
  && apt-get install -y --no-install-recommends libolm-dev pkg-config build-essential ca-certificates \
  && rm -rf /var/lib/apt/lists/*

RUN CGO_ENABLED=1 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/ai-bridge ./cmd/ai-bridge

FROM gcr.io/distroless/base-debian12:nonroot

COPY --from=build /out/ai-bridge /usr/local/bin/ai-bridge
# libolm shared libraries are needed at runtime.
# libolm shared libraries are needed at runtime (multiarch path).
COPY --from=build /usr/lib/*-linux-gnu/libolm.so.3* /usr/lib/
# libstdc++ is required by libolm (CGO).
COPY --from=build /usr/lib/*-linux-gnu/libstdc++.so.6* /usr/lib/
# libgcc_s is also required by libolm (CGO).
COPY --from=build /lib/*-linux-gnu/libgcc_s.so.1 /lib/

WORKDIR /data
VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/ai-bridge", "--config", "/data/config.yaml", "--registration", "/data/registration.yaml"]
