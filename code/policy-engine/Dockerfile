# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.26

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG TARGETOS TARGETARCH VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" -o /out/policy-engine ./cmd/policy-engine

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/policy-engine /policy-engine
USER 65532:65532
EXPOSE 8443 8080
ENTRYPOINT ["/policy-engine"]
