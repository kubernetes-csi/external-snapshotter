# update crds
rm charts/external-snapshotter/crds/*
kubectl kustomize deploy/kubernetes/snapshot-controller | yq 'del(.metadata.creationTimestamp)' > charts/external-snapshotter/crds/external-snapshotter.crds.yaml

# update templates
rm charts/external-snapshotter/templates/*
kubectl kustomize deploy/kubernetes/snapshot-controller | yq 'del(.metadata.namespace)' > charts/external-snapshotter/templates/snapshot-controller.yaml

cp charts/external-snapshotter/Chart.yaml .tmp.Chart.yaml

# update version
yq '.version | capture("(?P<n>[0-9]$)") | .n tag= "!!int" | load(".tmp.Chart.yaml") * {"version": (load(".tmp.Chart.yaml") | .version | capture("(?P<n>[0-9]+\.[0-9]+\.)[0-9]+$") | .n) + .n + 1}' .tmp.Chart.yaml > charts/external-snapshotter/Chart.yaml

cp charts/external-snapshotter/Chart.yaml .tmp.Chart.yaml

# update appVersion
kubectl kustomize deploy/kubernetes/snapshot-controller | yq 'select(.kind == "Deployment") | .spec.template.spec.containers[] | select(.name == "snapshot-controller") | .image | capture(".*v(?P<n>[0-9\.]+)") | load(".tmp.Chart.yaml") * {"appVersion": .n}' > charts/external-snapshotter/Chart.yaml

rm .tmp.Chart.yaml
