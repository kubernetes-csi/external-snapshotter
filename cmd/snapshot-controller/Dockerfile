FROM gcr.io/distroless/static:latest-amd64
LABEL maintainers="Kubernetes Authors"
LABEL description="Snapshot Controller"
ARG binary=./bin/snapshot-controller

COPY ${binary} snapshot-controller
ENTRYPOINT ["/snapshot-controller"]
