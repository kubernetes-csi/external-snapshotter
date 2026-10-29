FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="Snapshot Topology Webhook"
ARG binary=./bin/snapshot-topology-webhook

COPY ${binary} snapshot-topology-webhook
ENTRYPOINT ["/snapshot-topology-webhook"]
