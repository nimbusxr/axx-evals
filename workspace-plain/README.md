# Parcels: acceptance tests

Black-box acceptance tests for **parcels**, our parcel-registration service. Shops register
parcels through the REST API or upload manifests in bulk; depot scanners feed the tracking
view; every registered parcel gets a shipping label and a `ParcelRegistered` event.

There are no acceptance tests yet.

## The service

| What | Where |
| --- | --- |
| Binary | `parcels` (on the `PATH`): start it in the background; it needs no configuration here |
| API | `http://localhost:8080`, health at `/health` (200 once it is ready) |
| OpenAPI 3.1 | `http://localhost:8080/openapi.json` (also `openapi.yaml` in this repository) |
| Business rules | `docs/` (registration, manifest import, tracking, labels, events) |
| Database | `docs/database.md`: the PostgreSQL tables and the MongoDB collections |

## Infrastructure

The environment runs these next to the service:

| Service | Address | Credentials |
| --- | --- | --- |
| PostgreSQL | `postgres:5432`, database `parcels`, schema `parcels` | `parcels` / `parcels` |
| MongoDB | `mongo:27017`, database `parcels` (authenticate against `admin`) | `parcels` / `parcels` |
| Address service (WireMock) | `http://address-service:8080`; stubs in `infra/address-service/mappings/` | none |
| Kafka and Schema Registry | `kafka:9092`, `http://schema-registry:8081` (only where events are enabled) | none |

The service authenticates to the address service with the API key `evals-address-key`, and
signs shipping labels with the key `evals-label-secret` (test environment values).

## Tools

Go, Node.js, Python (with `uv`), `git`, `curl` and `jq` are installed; install whatever else
the tests need.

## Test data

Data stays in the databases, the address service's request journal and Kafka until it is
wiped. `parcels reset` wipes all of it (run it only while the service is stopped).
