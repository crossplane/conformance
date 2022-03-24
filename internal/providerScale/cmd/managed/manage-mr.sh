#!/usr/bin/env zsh
# Copyright 2022 The Crossplane Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


# HO.

set -euxo pipefail

# Example usage: ./manage-mr.sh create virtualnetwork.yaml 10
# Example usage: ./manage-mr.sh delete virtualnetwork.yaml 10 | tee exp-virtualnetwork.log
OPERATION="$1"
TEMPLATE="$2"
NAME="$3"

cat "${TEMPLATE}" | sed "s/{{SUFFIX}}/$NAME/g" | kubectl --wait=false "${OPERATION}" -f -
