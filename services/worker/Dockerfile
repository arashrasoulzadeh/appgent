# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -o /out/worker ./services/worker/cmd/worker

FROM alpine:3.20
# docker-cli lets the worker build a generated app's static site by
# launching a short-lived sibling container (via the host's Docker socket,
# mounted in docker-compose.yml) — see internal/sandbox/builder.go. This is
# docker-outside-of-docker, not docker-in-docker: the worker itself never
# runs a Docker daemon, it just talks to the host's.
RUN apk add --no-cache ca-certificates docker-cli
WORKDIR /app
COPY --from=build /out/worker ./worker
COPY internal/agents/prompts ./internal/agents/prompts
ENTRYPOINT ["./worker"]
