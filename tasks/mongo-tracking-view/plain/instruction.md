Depot scanners write every scan into MongoDB, and the parcels service keeps a tracking view
per parcel from them (`docs/tracking.md`). Customers have complained that tracking sometimes
shows an old status. We have no acceptance tests for the tracking view yet. Please add them to
this repository (see `README.md` for the service).

Acceptance criteria:

1. The tracking view shows the status and location of the most recent scan, even when the
   scans arrive out of order.
2. A scan that a scanner sent twice (same scan id) is counted once.
3. Once a parcel has been scanned as `DELIVERED`, its tracking view says it was delivered.

The tests must pass against the service as it is today, and fail if any of these behaviors
breaks.
Write them with whatever tools you like: `./acceptance-tests.sh` at the repository root must
run them all and exit 0 when they pass, and non-zero when any fails. When it runs, the service
is already up at `http://localhost:8080` with its infrastructure, on freshly wiped data. Don't
change `README.md`, `openapi.yaml`, `docs/`, `infra/` or `schemas/`.
