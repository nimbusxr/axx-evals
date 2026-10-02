package acceptance

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// service is the parcels service under test.
const service = "http://localhost:8080"

// response is an HTTP answer with its body as JSON (nil when it has none).
type response struct {
	status int
	header http.Header
	body   map[string]any
}

// call sends a request with a JSON body (when body is not nil).
func call(t *testing.T, method, url string, body any) response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	out := response{status: res.StatusCode, header: res.Header}
	b, _ := io.ReadAll(res.Body)
	if len(bytes.TrimSpace(b)) > 0 {
		_ = json.Unmarshal(b, &out.body)
	}
	return out
}

// parcel is a registration for a German recipient; change what a test needs.
func parcel(reference, sender string) map[string]any {
	return map[string]any{
		"reference":    reference,
		"sender":       sender,
		"weightGrams":  1200,
		"serviceLevel": "STANDARD",
		"recipient": map[string]any{
			"name": "Ada Example", "street": "Example Street 1", "city": "Berlin", "postcode": "10115", "country": "DE",
		},
	}
}

func recipient(p map[string]any) map[string]any { return p["recipient"].(map[string]any) }

func wantStatus(t *testing.T, what string, got response, want int) {
	t.Helper()
	if got.status != want {
		t.Fatalf("%s: status %d, want %d (%v)", what, got.status, want, got.body)
	}
}

func wantField(t *testing.T, body map[string]any, name string, want any) {
	t.Helper()
	got := body[name]
	if n, ok := want.(int); ok {
		want = float64(n) // JSON numbers
	}
	if got != want {
		t.Errorf("%s is %v, want %v", name, got, want)
	}
}

func wantContains(t *testing.T, what, s, part string) {
	t.Helper()
	if !strings.Contains(s, part) {
		t.Errorf("%s %q does not mention %q", what, s, part)
	}
}
