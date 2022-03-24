#!/usr/bin/env zsh

# HO.

set -euxo pipefail

TEMPLATE="$1"
NAME="$2"

cat "${TEMPLATE}" | sed "s/{{SUFFIX}}/$NAME/g" | kubectl get -o json -f - | jq '.items[] | .status.conditions[] | select(.type=="Ready")' | jq .status
