FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="Snapshot Validation Webhook"
ARG binary=./bin/snapshot-validation-webhook

COPY ${binary} snapshot-validation-webhook
ENTRYPOINT ["/snapshot-validation-webhook"]
