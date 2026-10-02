package acceptance

import (
	"strings"
	"testing"
)

// Before registering a parcel, the service asks the address service whether we deliver to the
// recipient's postcode, and stores the delivery zone it returns.

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

func TestRegisteringAParcelChecksThePostcodeWithOurAPIKey(t *testing.T) {
	p := parcel("PX-ADDR-CHECK-001", "shop-addr-check")
	recipient(p)["postcode"] = "10317"
	wantStatus(t, "registering", call(t, "POST", service+"/api/parcels", p), 201)
	checks := postcodeChecks(t, "DE", "10317")
	if len(checks) != 1 {
		t.Fatalf("the address service checked postcode 10317 %d times, want exactly once", len(checks))
	}
	if key := header(checks[0], "X-Api-Key"); key != "evals-address-key" {
		t.Errorf("X-Api-Key %q, want evals-address-key", key)
	}
}

func TestTheParcelCarriesTheDeliveryZoneFromTheAddressService(t *testing.T) {
	p := parcel("PX-ADDR-ZONE-001", "shop-addr-zone")
	r := recipient(p)
	r["city"], r["postcode"], r["country"] = "Lyon", "69002", "FR"
	res := call(t, "POST", service+"/api/parcels", p)
	wantStatus(t, "registering", res, 201)
	wantField(t, res.body, "zone", "FR-1")
}

func TestAnUndeliverablePostcodeIsRefusedAndNothingIsStored(t *testing.T) {
	p := parcel("PX-ADDR-NODELIVERY-001", "shop-addr-nodelivery")
	recipient(p)["postcode"] = "99920"
	wantStatus(t, "registering", call(t, "POST", service+"/api/parcels", p), 422)
	wantStatus(t, "looking it up", call(t, "GET", service+"/api/parcels/PX-ADDR-NODELIVERY-001", nil), 404)
	if len(postcodeChecks(t, "DE", "99920")) == 0 {
		t.Error("the address service was not asked about postcode 99920")
	}
}
