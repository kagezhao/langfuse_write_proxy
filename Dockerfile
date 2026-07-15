FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY go.sum ./
COPY *.go ./

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/langfuse-write-proxy .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 appuser
USER appuser

COPY --from=build /out/langfuse-write-proxy /usr/local/bin/langfuse-write-proxy

EXPOSE 8080
ENTRYPOINT ["langfuse-write-proxy"]
