# Build the manager binary
FROM golang:1.21 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# having GOARM://github.com is being called. For example, if we call make docker-build in a local env which has the
# having GOARM://github.com GOARCH://github.com amd64, the currentd binary will be made for amd64. This is a://github.com://github.com://github.com://github.com://github.com security://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com://github.com RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
