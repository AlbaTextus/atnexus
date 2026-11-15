#!/usr/bin/env bash
set -euo pipefail
exec node "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/build-linux-amd64.mjs" "$@"
