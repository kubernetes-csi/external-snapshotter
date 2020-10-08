FROM gcr.io/distroless/base:latest-amd64
LABEL maintainers="Kubernetes Authors"
LABEL description="Snapshot Validation Webhook"
ARG binary=./bin/snapshot-validation-webhook

COPY ${binary} snapshot-validation-webhook
ENTRYPOINT ["/snapshot-validation-webhook"]
