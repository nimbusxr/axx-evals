A shop can change or cancel a parcel only until a depot picks it up (see
`docs/registration.md`): from then on the depot has it, and a change or a cancellation must be
refused without touching the parcel. A shop recently found a picked-up parcel changed anyway, so
we want acceptance tests for these rules. Please add them to this repository (see `README.md`
for the service and how the tests run, and `docs/database.md` for its tables).

Acceptance criteria:

1. A parcel a depot has picked up can no longer be changed: a change is refused with
   **409 Conflict**, and the parcel keeps its weight.
2. A parcel a depot has picked up can no longer be cancelled: the cancellation is refused with
   **409 Conflict**, and the parcel can still be looked up.
3. Until then, a registered parcel can still be changed: the change is accepted (**200**) and
   stored.

The depots' system marks a picked-up parcel in the database (its `status` becomes `PICKED_UP`);
there is no API for it. The tests must pass against the service as it is today and fail if any
of these rules breaks. Put them in `features/` and the rows you insert in seed files in `seeds/`;
don't change anything else in the repository.
