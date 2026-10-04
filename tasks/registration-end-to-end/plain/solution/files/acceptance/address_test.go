package acceptance

import (
	"strings"
	"testing"
)

// A registration is checked with the address service, stored with the delivery zone it gives,
// and announced to billing and the depots as a ParcelRegistered event.

const addressService = "http://address-service:8080"

// postcodeChecks are the requests the address service (WireMock) received for a postcode.
func postcodeChecks(t *testing.T, country, postcode string) []map[string]any {
	t.Helper()
	res := call(t, "POST", addressService+"/__admin/requests/find", map[string]any{
		"method": "GET", "url": "/v1/postcodes/" + country + "/" + postcode,
	})
	wantStatus(t, "asking the address service what it received", res, 200)
	var out []map[string]any
	reqs, _ := res.body["requests"].([]any)
	for _, r := range reqs {
		out = append(out, r.(map[string]any))
	}
	return out
}

func header(req map[string]any, name string) string {
	h, _ := req["headers"].(map[string]any)
	for k, v := range h {
		if strings.EqualFold(k, name) {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func TestARegisteredParcelIsStoredWithTheZoneTheAddressServiceGives(t *testing.T) {
	p := parcel("PX-E2E-STORED-001", "shop-e2e-stored")
	r := recipient(p)
	r["city"], r["postcode"], r["country"] = "Lyon", "69002", "FR"
	wantStatus(t, "registering", call(t, "POST", service+"/api/parcels", p), 201)
	got := call(t, "GET", service+"/api/parcels/PX-E2E-STORED-001", nil)
	wantStatus(t, "looking it up", got, 200)
	wantField(t, got.body, "zone", "FR-1")
}

func TestRegisteringAParcelChecksThePostcodeOnceWithOurAPIKey(t *testing.T) {
	p := parcel("PX-E2E-CHECK-001", "shop-e2e-check")
	recipient(p)["postcode"] = "10318"
	wantStatus(t, "registering", call(t, "POST", service+"/api/parcels", p), 201)
	checks := postcodeChecks(t, "DE", "10318")
	if len(checks) != 1 {
		t.Fatalf("the address service checked postcode 10318 %d times, want exactly once", len(checks))
	}
	if key := header(checks[0], "X-Api-Key"); key != "evals-address-key" {
		t.Errorf("X-Api-Key %q, want evals-address-key", key)
	}
}

func TestAnUndeliverablePostcodeIsRefusedAndNothingIsStored(t *testing.T) {
	p := parcel("PX-E2E-NODELIVERY-001", "shop-e2e-nodelivery")
	recipient(p)["postcode"] = "99930"
	res := call(t, "POST", service+"/api/parcels", p)
	wantStatus(t, "registering", res, 422)
	detail, _ := res.body["detail"].(string)
	wantContains(t, "the problem detail", detail, "address not deliverable")
	wantStatus(t, "looking it up", call(t, "GET", service+"/api/parcels/PX-E2E-NODELIVERY-001", nil), 404)
}
