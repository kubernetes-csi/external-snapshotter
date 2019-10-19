FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="CSI External Snapshotter Sidecar"

COPY ./bin/csi-snapshotter csi-snapshotter
ENTRYPOINT ["/csi-snapshotter"]
