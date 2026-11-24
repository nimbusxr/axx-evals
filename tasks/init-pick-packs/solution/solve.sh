#!/usr/bin/env bash
# Reference solution: `axx init`, the packs for the database and the events,
# point the service at the parcels command, add the features.
set -euo pipefail
cd /app
axx init --no-ci
axx pack add sql kafka

# axx init leaves a placeholder start command: the service is the parcels binary.
python3 - <<'PY'
import re
p = "axx.yaml"
s = open(p).read()
s, n = re.subn(
    r'    command: echo "replace with the command that starts your app" && exit 1\n    shell: true\n',
    "    command: parcels\n    cleanup: parcels reset   # wipes the service's data after the run\n",
    s,
)
assert n == 1, "axx init output changed; update the reference solution"
open(p, "w").write(s)
PY

cp -R /solution/files/. /app/
axx validate
