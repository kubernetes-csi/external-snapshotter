FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="Snapshot Webhook"
ARG binary=./bin/snapshot-conversion-webhook

COPY ${binary} snapshot-conversion-webhook
ENTRYPOINT ["/snapshot-conversion-webhook"]
