package acceptance

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	brokers  = "kafka:9092"
	registry = "http://schema-registry:8081"
	topic    = "parcel-events"
)

func TestARegisteredParcelIsAnnouncedAsAParcelRegisteredEvent(t *testing.T) {
	p := parcel("PX-E2E-EVENT-001", "shop-e2e-event")
	p["weightGrams"] = 3150
	r := recipient(p)
	r["city"], r["postcode"], r["country"] = "Leipzig", "04109", "DE"
	wantStatus(t, "registering", call(t, "POST", service+"/api/parcels", p), 201)
	event := decode(t, eventFor(t, "PX-E2E-EVENT-001").Value)
	for k, want := range map[string]any{"reference": "PX-E2E-EVENT-001", "weightGrams": int64(3150), "zone": "DE-1"} {
		if event[k] != want {
			t.Errorf("event %s is %v, want %v", k, event[k], want)
		}
	}
}

// eventFor reads the topic from its start until the event keyed by the reference.
func eventFor(t *testing.T, reference string) *kgo.Record {
	t.Helper()
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for {
		fetches := client.PollFetches(ctx)
		if ctx.Err() != nil {
			t.Fatalf("no event keyed %s on %s within 15s", reference, topic)
		}
		var found *kgo.Record
		fetches.EachRecord(func(r *kgo.Record) {
			if found == nil && string(r.Key) == reference {
				found = r
			}
		})
		if found != nil {
			return found
		}
	}
}

// decode reads a value in the Confluent wire format: magic byte 0, the schema id, then the
// Avro record, decoded with the schema the Schema Registry has under that id.
func decode(t *testing.T, value []byte) map[string]any {
	t.Helper()
	if len(value) < 5 || value[0] != 0 {
		t.Fatalf("the value is not in the Confluent wire format: % x", value[:min(len(value), 8)])
	}
	id := binary.BigEndian.Uint32(value[1:5])
	res, err := http.Get(fmt.Sprintf("%s/schemas/ids/%d", registry, id))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var reg struct {
		Schema string `json:"schema"`
	}
	if err := json.NewDecoder(res.Body).Decode(&reg); err != nil || res.StatusCode != 200 {
		t.Fatalf("schema %d from the Schema Registry: status %d, %v", id, res.StatusCode, err)
	}
	var schema struct {
		Name   string `json:"name"`
		Fields []struct {
			Name string          `json:"name"`
			Type json.RawMessage `json:"type"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(reg.Schema), &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Name != "ParcelRegistered" {
		t.Fatalf("the value's schema is %s, want ParcelRegistered", schema.Name)
	}
	b := value[5:]
	out := map[string]any{}
	for _, f := range schema.Fields {
		var typ any
		_ = json.Unmarshal(f.Type, &typ)
		if m, ok := typ.(map[string]any); ok {
			typ = m["type"]
		}
		switch typ {
		case "string":
			n, rest := varint(t, b)
			out[f.Name], b = string(rest[:n]), rest[n:]
		case "int", "long":
			out[f.Name], b = varint(t, b)
		default:
			t.Fatalf("field %s: type %v not expected in ParcelRegistered", f.Name, typ)
		}
	}
	return out
}

// varint reads an Avro zig-zag long.
func varint(t *testing.T, b []byte) (int64, []byte) {
	t.Helper()
	u, n := binary.Uvarint(b)
	if n <= 0 {
		t.Fatal("a truncated Avro value")
	}
	return int64(u>>1) ^ -int64(u&1), b[n:]
}
