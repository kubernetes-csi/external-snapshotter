#! /bin/bash

. release-tools/prow.sh


CSI_PROW_DRIVER_VERSION='fix_deploy_loop_for_k8s116'
CSI_PROW_DRIVER_REPO='https://github.com/ggriffiths/csi-driver-host-path'

main
