FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="CSI External Snapshotter Common"

COPY ./bin/csi-snapshotter-common csi-snapshotter-common
ENTRYPOINT ["/csi-snapshotter-common"]
