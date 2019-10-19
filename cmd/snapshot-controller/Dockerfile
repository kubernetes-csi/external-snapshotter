FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="Snapshot Controller"

COPY ./bin/snapshot-controller snapshot-controller
ENTRYPOINT ["/snapshot-controller"]
