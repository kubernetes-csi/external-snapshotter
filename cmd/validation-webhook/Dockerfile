FROM gcr.io/distroless/base:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="Snapshot Validation Webhook"
ARG binary=./bin/validation-webhook

COPY ${binary} validation-webhook
ENTRYPOINT ["/validation-webhook"]
