FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/worker ./services/worker/cmd/worker

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/worker ./worker
COPY internal/agents/prompts ./internal/agents/prompts
ENTRYPOINT ["./worker"]
