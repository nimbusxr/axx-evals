This repository holds the API contract (`openapi.yaml`), the schema of our events and the docs of
the parcels service, but no acceptance tests. We want to write them with axx (it is installed as
`axx`). Please set the repository up for axx and add acceptance tests for these behaviors;
`README.md` explains how the service and its infrastructure run in this environment.

Acceptance criteria:

1. A manifest line a shop's system writes into the `parcels.manifest_lines` table is imported as
   a registered parcel: the parcel keeps the line's sender, weight and recipient postcode, and
   records that it came from the manifest (source `manifest`). The line is then marked
   `IMPORTED`.
2. A manifest line for a parcel heavier than 30000 g is marked `REJECTED`, and no parcel is
   created for it.
3. Registering a parcel through the API (**201 Created**) publishes a `ParcelRegistered` event
   on the `parcel-events` topic, keyed by the parcel's reference, with its weight in grams.

`axx run` must start the service, run the tests and pass against the service as it is today; the
tests must fail if any of these behaviors breaks. Don't change `README.md`, `openapi.yaml`,
`schemas/` or the docs.
