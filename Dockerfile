FROM alpine
LABEL maintainers="Kubernetes Authors"
LABEL description="CSI External Snapshotter"

COPY ./bin/csi-snapshotter csi-snapshotter
ENTRYPOINT ["/csi-snapshotter"]
