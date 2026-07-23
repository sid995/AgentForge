FROM golang:1.26.1-alpine AS build

WORKDIR /workspace

ARG BUILD_VERSION=development
ARG BUILD_COMMIT=unknown
ARG BUILD_TIME=unknown

COPY go.work ./
COPY operator operator
COPY services/platform-api services/platform-api

RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w \
      -X github.com/sid995/agentforge/services/platform-api/internal/buildinfo.Version=${BUILD_VERSION} \
      -X github.com/sid995/agentforge/services/platform-api/internal/buildinfo.Commit=${BUILD_COMMIT} \
      -X github.com/sid995/agentforge/services/platform-api/internal/buildinfo.BuildTime=${BUILD_TIME}" \
    -o /out/platform-api ./services/platform-api/cmd/platform-api

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/platform-api /platform-api

EXPOSE 8080

ENTRYPOINT ["/platform-api"]
