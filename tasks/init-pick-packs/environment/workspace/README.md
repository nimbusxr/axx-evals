# Parcels

The contract and documentation of **parcels**, our parcel-registration service: shops register
parcels through the API or write manifest lines in bulk, and every registered parcel is announced
to billing and the depots as an event.

- `openapi.yaml`: the API contract (OpenAPI 3.1). The running service also serves it at
  `/openapi.json`.
- `schemas/parcel-registered.avsc`: the Avro schema of the `ParcelRegistered` event.
- `docs/`: the registration rules, the manifest import, the events and the database tables.

## Running the service

The service is installed in this environment as the `parcels` command. It needs no
configuration here: it connects to its infrastructure on its own, listens on
`http://localhost:8080` and answers `GET /health` with 200 once it is ready. `parcels reset`
wipes all of its data (run it only while the service is stopped).

## Infrastructure

The environment runs these next to the service:

| Service | Address | Credentials |
| --- | --- | --- |
| PostgreSQL | `postgres:5432`, database `parcels`, schema `parcels` | `parcels` / `parcels` |
| MongoDB | `mongo:27017`, database `parcels` (authenticate against `admin`) | `parcels` / `parcels` |
| Address service | `http://address-service:8080` | none |
| Kafka and Schema Registry | `kafka:9092`, `http://schema-registry:8081` | none |

## Acceptance tests

None yet. We want to adopt [axx](https://axx.nimbusxr.us) (installed as `axx`).
