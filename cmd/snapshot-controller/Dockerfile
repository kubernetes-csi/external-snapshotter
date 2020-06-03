FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="Snapshot Controller"
ARG binary=./bin/snapshot-controller

COPY ${binary} snapshot-controller
ENTRYPOINT ["/snapshot-controller"]
