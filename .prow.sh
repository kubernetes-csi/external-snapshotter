#! /bin/bash

# This is specific to the release-2.1 branch and overrides the
# version set in the prow config.
export CSI_SNAPSHOTTER_VERSION=v2.1.2

# The problem that this solves is that the prow config assumes
# that Kubernetes 1.20 uses the v1 snapshotter API, whereas this
# branch still uses v1beta1. By downgrading to an older deployment
# and e2e.test suite we get tests to run.
export CSI_PROW_DRIVER_VERSION=v1.4.0
export CSI_PROW_E2E_VERSION=v1.19.7

. release-tools/prow.sh

main
