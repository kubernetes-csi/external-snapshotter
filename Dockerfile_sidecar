FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="CSI External Snapshotter Sidecar"

COPY ./bin/csi-snapshotter-sidecar csi-snapshotter-sidecar
ENTRYPOINT ["/csi-snapshotter-sidecar"]
