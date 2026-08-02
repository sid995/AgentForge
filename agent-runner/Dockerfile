FROM golang:1.26.1-alpine AS build

WORKDIR /workspace

COPY agent-runner/go.mod ./agent-runner/go.mod
COPY agent-runner ./agent-runner

WORKDIR /workspace/agent-runner

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/agent-runner ./cmd/agent-runner

FROM python:3.12-alpine

RUN addgroup -S -g 65532 agent && adduser -S -D -H -u 65532 -G agent agent

COPY --from=build /out/agent-runner /agent-runner

USER 65532:65532
ENTRYPOINT ["/agent-runner"]
