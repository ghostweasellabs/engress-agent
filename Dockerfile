# syntax=docker/dockerfile:1
FROM golang:1.25-bookworm AS build
ARG MODULE_PATH=github.com/ghostweasellabs/engress-agent
ARG BINARY_PATH=./cmd/engress-agent
ARG VERSION=dev
ARG COMMIT=unknown
ENV GOPRIVATE=github.com/ghostweasellabs/*
ENV GONOSUMDB=github.com/ghostweasellabs/*
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=secret,id=github_token \
    git config --global url."https://x-access-token:$(cat /run/secrets/github_token)@github.com/".insteadOf "https://github.com/" && \
    go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
  -ldflags "-X github.com/ghostweasellabs/engress-sdk/observability.Version=${VERSION} -X github.com/ghostweasellabs/engress-sdk/observability.Commit=${COMMIT}" \
  -o /out/engress-agent ${BINARY_PATH}

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/engress-agent /usr/local/bin/engress-agent
ENTRYPOINT ["/usr/local/bin/engress-agent"]
CMD ["version"]
