FROM gcr.io/distroless/static:latest-amd64
LABEL maintainers="Kubernetes Authors"
LABEL description="CSI External Snapshotter Sidecar"
ARG binary=./bin/csi-snapshotter

COPY ${binary} csi-snapshotter
ENTRYPOINT ["/csi-snapshotter"]
