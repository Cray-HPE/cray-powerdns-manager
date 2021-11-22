#!/usr/bin/env bash

# Copyright 2021 Hewlett Packard Enterprise Development LP

set -o errexit
set -o pipefail

ROOTDIR="$(dirname "${BASH_SOURCE[0]}")/.."

[[ $# -eq 0 ]] && set -- ${ROOTDIR}/kubernetes/*

declare -a charts=()

while [[ $# -gt 0 ]]; do
    chartdir="$1"
    shift
    if [[ ! -d "$chartdir" ]]; then
        echo >&2 "${0##*/}: ${chartdir}: No such directory"
        exit 1
    elif [[ ! -f "${chartdir}/Chart.yaml" ]]; then
        echo >&2 "${0##*/}: ${chartdir}/Chart.yaml: No such file"
        exit 1
    elif [[ ! -f "${chartdir}/values.yaml" ]]; then
        echo >&2 "${0##*/}: ${chartdir}/values.yaml: No such file"
        exit 1
    fi
    charts+=("$chartdir")
done

set -o xtrace

for chartdir in "${charts[@]}"; do
    read -r name version appVersion < <(yq e '[.name, .version, .appVersion] | join(" ")' "${chartdir}/Chart.yaml")
    yq e -i ".global.chart.name = \"${name}\", .global.chart.version = \"${version}\", .global.appVersion = \"${appVersion}\"" "${chartdir}/values.yaml"
done
