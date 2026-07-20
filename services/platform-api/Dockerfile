FROM golang:1.26.1-alpine AS build

WORKDIR /workspace

COPY go.work ./
COPY services/platform-api/go.mod services/platform-api/go.mod
COPY services/platform-api services/platform-api

RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/platform-api ./services/platform-api/cmd/platform-api

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/platform-api /platform-api

EXPOSE 8080

ENTRYPOINT ["/platform-api"]
