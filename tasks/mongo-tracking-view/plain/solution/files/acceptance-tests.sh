#!/usr/bin/env bash
# Runs the acceptance tests against the running service.
set -euo pipefail
cd "$(dirname "$0")/acceptance"
exec go test -count=1 ./...
