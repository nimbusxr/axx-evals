package acceptance

import "testing"

// Shops register parcels, look them up, change them before pickup and cancel them.

func TestAShopRegistersAParcel(t *testing.T) {
	p := parcel("PX-LIFE-REG-001", "shop-life-reg")
	p["weightGrams"], p["serviceLevel"] = 1500, "EXPRESS"
	recipient(p)["name"] = "Ada Register"
	res := call(t, "POST", service+"/api/parcels", p)
	wantStatus(t, "registering", res, 201)
	wantField(t, res.body, "reference", "PX-LIFE-REG-001")
	wantField(t, res.body, "status", "REGISTERED")
	wantField(t, res.body, "weightGrams", 1500)
	wantField(t, res.body, "serviceLevel", "EXPRESS")
	r, _ := res.body["recipient"].(map[string]any)
	wantField(t, r, "name", "Ada Register")
	wantField(t, r, "postcode", "10115")
	wantField(t, r, "country", "DE")
}

func TestARegisteredParcelCanBeLookedUpByItsReference(t *testing.T) {
	p := parcel("PX-LIFE-GET-001", "shop-life-get")
	p["weightGrams"] = 2200
	wantStatus(t, "registering", call(t, "POST", service+"/api/parcels", p), 201)
	res := call(t, "GET", service+"/api/parcels/PX-LIFE-GET-001", nil)
	wantStatus(t, "looking it up", res, 200)
	wantField(t, res.body, "reference", "PX-LIFE-GET-001")
	wantField(t, res.body, "sender", "shop-life-get")
	wantField(t, res.body, "status", "REGISTERED")
	wantField(t, res.body, "weightGrams", 2200)
}

func TestAChangedWeightIsKept(t *testing.T) {
	p := parcel("PX-LIFE-CHG-001", "shop-life-chg")
	p["weightGrams"] = 1000
	wantStatus(t, "registering", call(t, "POST", service+"/api/parcels", p), 201)
	wantStatus(t, "changing the weight", call(t, "PATCH", service+"/api/parcels/PX-LIFE-CHG-001", map[string]any{"weightGrams": 3100}), 200)
	res := call(t, "GET", service+"/api/parcels/PX-LIFE-CHG-001", nil)
	wantStatus(t, "looking it up", res, 200)
	wantField(t, res.body, "weightGrams", 3100)
}

func TestACancelledParcelCanNoLongerBeFound(t *testing.T) {
	wantStatus(t, "registering", call(t, "POST", service+"/api/parcels", parcel("PX-LIFE-DEL-001", "shop-life-del")), 201)
	wantStatus(t, "cancelling", call(t, "DELETE", service+"/api/parcels/PX-LIFE-DEL-001", nil), 204)
	wantStatus(t, "looking it up", call(t, "GET", service+"/api/parcels/PX-LIFE-DEL-001", nil), 404)
}
