Registering a parcel touches everything the parcels service depends on: it asks our address
service whether we deliver to the recipient's postcode, stores the parcel with the delivery zone
it gets back, and announces it to billing and the depots as a `ParcelRegistered` event (see
`docs/registration.md` and `docs/events.md`). We have no acceptance tests that follow a
registration end to end. Please add them to this repository (see `README.md` for the service and
its infrastructure).

Acceptance criteria:

1. A registered parcel (**201 Created**) is stored: looking up its reference afterwards returns
   it with the delivery zone the address service gave its postcode.
2. Registering a parcel checks the recipient's postcode with the address service exactly once,
   with our API key (the `X-Api-Key` header; the key is `evals-address-key` in this environment).
3. When the address service says a postcode is not deliverable, the registration is refused
   with **422** and the parcel is not stored. (In this environment the address service answers
   "not deliverable" for postcodes that start with `999`.)
4. A registered parcel is announced on the `parcel-events` topic as a `ParcelRegistered` event,
   keyed by its reference, with its weight in grams and its delivery zone.

The address service is a WireMock mock (`http://address-service:8080`, stubs in
`infra/address-service/mappings/`); leave the stubs as they are. The tests must pass against the
service as it is today and fail if any of these behaviors breaks.
Write them with whatever tools you like: `./acceptance-tests.sh` at the repository root must
run them all and exit 0 when they pass, and non-zero when any fails. When it runs, the service
is already up at `http://localhost:8080` with its infrastructure, on freshly wiped data. Don't
change `README.md`, `openapi.yaml`, `docs/`, `infra/` or `schemas/`.
