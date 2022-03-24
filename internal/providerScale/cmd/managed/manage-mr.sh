#!/usr/bin/env zsh

# HO.

set -euxo pipefail

# Example usage: ./manage-mr.sh create virtualnetwork.yaml 10
# Example usage: ./manage-mr.sh delete virtualnetwork.yaml 10 | tee exp-virtualnetwork.log
OPERATION="$1"
TEMPLATE="$2"
NAME="$3"

cat "${TEMPLATE}" | sed "s/{{SUFFIX}}/$NAME/g" | kubectl --wait=false "${OPERATION}" -f -
