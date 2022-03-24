#!/usr/bin/env zsh

# HO.

set -euxo pipefail

TEMPLATE="$1"
NAME="$2"

cat "${TEMPLATE}" | sed "s/{{SUFFIX}}/$NAME/g" | kubectl get --ignore-not-found -f -