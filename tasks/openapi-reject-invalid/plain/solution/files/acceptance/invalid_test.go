package acceptance

import (
	"fmt"
	"strings"
	"testing"
)

// Parcels we can't carry are refused at registration, before anything is stored.

func TestAParcelHeavierThan30KgIsRefused(t *testing.T) {
	p := parcel("PX-INV-HEAVY-001", "shop-inv-heavy")
	p["weightGrams"] = 30001
	res := call(t, "POST", service+"/api/parcels", p)
	wantStatus(t, "registering", res, 400)
	if ct := res.header.Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Errorf("Content-Type %q, want application/problem+json", ct)
	}
	wantContains(t, "the problem detail", fmt.Sprint(res.body["detail"]), "weightGrams")
}

func TestAServiceLevelWeDontOfferIsRefused(t *testing.T) {
	p := parcel("PX-INV-LEVEL-001", "shop-inv-level")
	p["serviceLevel"] = "OVERNIGHT"
	res := call(t, "POST", service+"/api/parcels", p)
	wantStatus(t, "registering", res, 400)
	wantContains(t, "the problem detail", fmt.Sprint(res.body["detail"]), "serviceLevel")
}

func TestARefusedParcelIsNotRegistered(t *testing.T) {
	p := parcel("PX-INV-NOTSTORED-001", "shop-inv-notstored")
	p["weightGrams"] = 45000
	wantStatus(t, "registering", call(t, "POST", service+"/api/parcels", p), 400)
	wantStatus(t, "looking it up", call(t, "GET", service+"/api/parcels/PX-INV-NOTSTORED-001", nil), 404)
}
