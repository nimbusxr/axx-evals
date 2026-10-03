package acceptance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// A shop can change or cancel a parcel while it is registered. Once a depot has picked it up,
// a change or a cancellation is refused, and the parcel stays as it was.

const database = "postgres://parcels:parcels@postgres:5432/parcels"

// seedParcel writes a parcel row with a status, as the depots' system leaves it.
func seedParcel(t *testing.T, reference, status string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := pgx.Connect(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(context.Background())
	recipient, _ := json.Marshal(map[string]string{"name": "Ada Rules", "street": "Example Street 1", "city": "Berlin", "postcode": "10115", "country": "DE"})
	details, _ := json.Marshal(map[string]string{"source": "api", "zone": "DE-1"})
	if _, err := db.Exec(ctx,
		`INSERT INTO parcels.parcels (reference, sender, status, weight_grams, service_level, recipient, details)
		 VALUES ($1, 'shop-rules', $2, 1200, 'STANDARD', $3, $4)`, reference, status, recipient, details); err != nil {
		t.Fatal(err)
	}
}

func TestAPickedUpParcelCanNoLongerBeChangedAndKeepsItsWeight(t *testing.T) {
	seedParcel(t, "PX-RULES-CHANGE-001", "PICKED_UP")
	res := call(t, "PATCH", service+"/api/parcels/PX-RULES-CHANGE-001", map[string]any{"weightGrams": 2400})
	wantStatus(t, "changing", res, 409)
	detail, _ := res.body["detail"].(string)
	wantContains(t, "the problem detail", detail, "can no longer be changed")
	got := call(t, "GET", service+"/api/parcels/PX-RULES-CHANGE-001", nil)
	wantStatus(t, "looking it up", got, 200)
	wantField(t, got.body, "weightGrams", 1200)
}

func TestAPickedUpParcelCanNoLongerBeCancelledAndCanStillBeLookedUp(t *testing.T) {
	seedParcel(t, "PX-RULES-CANCEL-001", "PICKED_UP")
	res := call(t, "DELETE", service+"/api/parcels/PX-RULES-CANCEL-001", nil)
	wantStatus(t, "cancelling", res, 409)
	detail, _ := res.body["detail"].(string)
	wantContains(t, "the problem detail", detail, "can no longer be cancelled")
	wantStatus(t, "looking it up", call(t, "GET", service+"/api/parcels/PX-RULES-CANCEL-001", nil), 200)
}

func TestARegisteredParcelCanStillBeChangedAndTheChangeIsStored(t *testing.T) {
	seedParcel(t, "PX-RULES-REGISTERED-001", "REGISTERED")
	wantStatus(t, "changing", call(t, "PATCH", service+"/api/parcels/PX-RULES-REGISTERED-001", map[string]any{"weightGrams": 2600}), 200)
	got := call(t, "GET", service+"/api/parcels/PX-RULES-REGISTERED-001", nil)
	wantStatus(t, "looking it up", got, 200)
	wantField(t, got.body, "weightGrams", 2600)
}
