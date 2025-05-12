FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="Snapshot Webhook"
ARG binary=./bin/snapshot-webhook

COPY ${binary} snapshot-webhook
ENTRYPOINT ["/snapshot-webhook"]
