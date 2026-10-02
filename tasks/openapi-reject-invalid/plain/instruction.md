Parcels we can't carry must be refused at registration, before anything is stored. We have
no acceptance tests for that yet. Please add them to this repository (see `README.md` for
the service and `docs/registration.md` for the rules).

Acceptance criteria:

1. A parcel heavier than 30 kg (more than 30000 g) is refused with **400 Bad Request**, and
   the problem details returned by the service mention the weight.
2. A parcel with a service level we don't offer (for example `OVERNIGHT`) is refused with
   **400 Bad Request**.
3. A refused parcel is not registered: looking up its reference afterwards finds nothing
   (**404**).

The tests must pass against the service as it is today, and fail if any of these rules stops
being enforced.
Write them with whatever tools you like: `./acceptance-tests.sh` at the repository root must
run them all and exit 0 when they pass, and non-zero when any fails. When it runs, the service
is already up at `http://localhost:8080` with its infrastructure, on freshly wiped data. Don't
change `README.md`, `openapi.yaml`, `docs/`, `infra/` or `schemas/`.
