# The database

The service keeps its data in PostgreSQL, in the schema `parcels` (created when the service
starts), and its tracking view in MongoDB, in the database `parcels`. Tests can read both to check
what the service stored, and write rows the service then works with (a seeded parcel shows up in
the API like any other).

## `parcels.parcels`

One row per registered parcel. Cancelling a parcel (`DELETE /api/parcels/{reference}`) deletes
its row.

| Column | Type | Default | Holds |
| --- | --- | --- | --- |
| `reference` (primary key) | text | | the shop's reference |
| `sender` | text | | the shop's id |
| `status` | text | `REGISTERED` | the parcel's status |
| `weight_grams` | integer | | the weight in grams |
| `service_level` | text | `STANDARD` | `STANDARD` or `EXPRESS` |
| `recipient` | jsonb | | `{"name", "street", "city", "postcode", "country"}`; `street`, `city` and `postcode` may be absent |
| `details` | jsonb | `{}` | where the parcel came from and its delivery zone: `{"source": "api", "zone": <zone>}`, or for an imported manifest line `{"source": "manifest", "manifestId": <manifest_id>, "lineId": <id>, "zone": <zone>}` |
| `label_number` | bigint | the next number of the sequence `parcels.label_numbers`, which starts at 100000001 | the shipping label's number (`labels.md`) |
| `created_at` | timestamptz | `now()` | when the parcel was registered |
| `updated_at` | timestamptz | `now()` | when it last changed |

The API's `zone` and `source` come from `details`: a row without `details.source` reads as
`api`, and one without `details.zone` has a `null` zone. Listing a sender's parcels orders them by
`created_at`, then `reference`.

## `parcels.manifest_lines`

One row per manifest line a shop writes (`manifest-import.md` has the import's rules).

| Column | Type | Default | Holds |
| --- | --- | --- | --- |
| `id` (primary key) | text | | the line's id |
| `manifest_id` | text | | the manifest the line belongs to |
| `reference` | text | | the parcel's reference |
| `sender` | text | | the shop's id |
| `weight_grams` | integer | | the weight in grams |
| `service_level` | text | `STANDARD` | `STANDARD` or `EXPRESS` |
| `recipient` | jsonb | | `{"name", "street", "city", "postcode", "country"}` |
| `status` | text | `PENDING` | `PENDING` when written; the importer sets `IMPORTED` or `REJECTED` |
| `error` | text | none | why a line was rejected |
| `parcel_reference` | text | none | the registered parcel's reference, for imported lines |
| `received_at` | timestamptz | `now()` | when the line arrived |
| `processed_at` | timestamptz | none | when the importer processed it |

The importer takes pending lines in `received_at` order.

## MongoDB: the database `parcels`

| Collection | Holds |
| --- | --- |
| `scans` | the depot scans, as `tracking.md` describes them: `scanId`, `parcelRef`, `status`, `location`, `scannedAt`. The service adds `projected: true` to a scan once the tracking view includes it. |
| `tracking` | the tracking view, one document per parcel, its `_id` the parcel reference: the fields `tracking.md` lists, and `updatedAt`, when the service last updated it. |
