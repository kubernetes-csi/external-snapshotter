#! /bin/bash

# This is specific to the release-2.1 branch and overrides the
# version set in the prow config.
export CSI_SNAPSHOTTER_VERSION=v2.1.2

. release-tools/prow.sh

main
