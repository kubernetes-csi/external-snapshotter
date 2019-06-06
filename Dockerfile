FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="CSI External Snapshotter"

COPY ./bin/csi-snapshotter csi-snapshotter
ENTRYPOINT ["/csi-snapshotter"]
