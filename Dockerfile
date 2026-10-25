# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
        -o /out/oido-google-calendar ./cmd/oido-google-calendar \
    && mkdir -p /out/data

# Static binary on distroless: no shell, no package manager, runs as uid 65532.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/oido-google-calendar /usr/local/bin/oido-google-calendar
COPY --from=build --chown=65532:65532 /out/data /data
ENV MCP_TRANSPORT=http \
    DATA_DIR=/data \
    PORT=3000
EXPOSE 3000
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/oido-google-calendar", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/oido-google-calendar", "serve"]
