#!/usr/bin/env bash

set -o errexit
set -o pipefail

ROOTDIR="$(dirname "${BASH_SOURCE[0]}")/.."

cd ${ROOTDIR}
make chart_setup chart_package 2>/dev/null >&2

find kubernetes/.packaged -type f -name 'cray-powerdns-manager-*.tgz' | while read chart; do
    helm template release $chart --dry-run --replace --dependency-update \
        --set manager.base_domain=example.com \
    | yq e -N '.. | .image? | select(.)' -
done
