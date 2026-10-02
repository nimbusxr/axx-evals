We have no acceptance tests yet for the basic parcel lifecycle in the parcels service.
Please add them to this repository (see `README.md` for the service).

Acceptance criteria:

1. A shop can register a parcel. The service confirms with **201 Created** and returns the
   parcel with status `REGISTERED` and the weight, service level and recipient it was
   registered with.
2. A registered parcel can be looked up by its reference.
3. A shop can change the weight of a registered parcel, and looking the parcel up afterwards
   shows the new weight.
4. A shop can cancel a parcel; after that, the parcel can no longer be found (**404**).

The tests must pass against the service as it is today, and fail if any of these behaviors
breaks.
Write them with whatever tools you like: `./acceptance-tests.sh` at the repository root must
run them all and exit 0 when they pass, and non-zero when any fails. When it runs, the service
is already up at `http://localhost:8080` with its infrastructure, on freshly wiped data. Don't
change `README.md`, `openapi.yaml`, `docs/`, `infra/` or `schemas/`.
