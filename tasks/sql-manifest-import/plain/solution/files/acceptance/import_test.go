package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Shops write manifest lines into the manifest table; the service imports each line as a
// registered parcel, or rejects it with a reason.

const database = "postgres://parcels:parcels@postgres:5432/parcels"

func connect(t *testing.T) *pgx.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := pgx.Connect(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	return db
}

// addLine writes a manifest line, as a shop's system does.
func addLine(t *testing.T, db *pgx.Conn, id, manifest, reference, sender string, grams int, level string, recipient map[string]string) {
	t.Helper()
	r, _ := json.Marshal(recipient)
	_, err := db.Exec(context.Background(),
		`INSERT INTO parcels.manifest_lines (id, manifest_id, reference, sender, weight_grams, service_level, recipient)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, manifest, reference, sender, grams, level, r)
	if err != nil {
		t.Fatal(err)
	}
}

// within retries check until it returns nil or ten seconds have passed: the import runs every
// few seconds.
func within(t *testing.T, what string, check func() error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := check()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %v", what, err)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func TestAValidManifestLineBecomesARegisteredParcel(t *testing.T) {
	db := connect(t)
	addLine(t, db, "ML-IMPORT-VALID-01", "MF-IMPORT-VALID", "PX-IMPORT-VALID-01", "shop-import-valid", 1800, "EXPRESS",
		map[string]string{"name": "Mia Import", "street": "Hafenstrasse 1", "city": "Hamburg", "postcode": "20457", "country": "DE"})
	var sender, level, status string
	var grams int
	var recipient, details map[string]any
	within(t, "the parcel PX-IMPORT-VALID-01", func() error {
		return db.QueryRow(context.Background(),
			`SELECT sender, weight_grams, service_level, status, recipient, details FROM parcels.parcels WHERE reference = $1`,
			"PX-IMPORT-VALID-01").Scan(&sender, &grams, &level, &status, &recipient, &details)
	})
	if sender != "shop-import-valid" || grams != 1800 || level != "EXPRESS" || status != "REGISTERED" {
		t.Errorf("parcel: sender %q, weight %d g, service level %q, status %q", sender, grams, level, status)
	}
	for k, want := range map[string]string{"name": "Mia Import", "street": "Hafenstrasse 1", "city": "Hamburg", "postcode": "20457", "country": "DE"} {
		if recipient[k] != want {
			t.Errorf("recipient %s is %v, want %q", k, recipient[k], want)
		}
	}
	for k, want := range map[string]string{"source": "manifest", "manifestId": "MF-IMPORT-VALID", "lineId": "ML-IMPORT-VALID-01"} {
		if details[k] != want {
			t.Errorf("details %s is %v, want %q", k, details[k], want)
		}
	}
}

func TestAnImportedManifestLineIsMarkedImported(t *testing.T) {
	db := connect(t)
	addLine(t, db, "ML-IMPORT-MARKED-01", "MF-IMPORT-MARKED", "PX-IMPORT-MARKED-01", "shop-import-marked", 700, "STANDARD",
		map[string]string{"name": "Noah Marked", "city": "Munich", "postcode": "80331", "country": "DE"})
	within(t, "the manifest line ML-IMPORT-MARKED-01", func() error {
		var status string
		var parcel *string
		err := db.QueryRow(context.Background(), `SELECT status, parcel_reference FROM parcels.manifest_lines WHERE id = $1`,
			"ML-IMPORT-MARKED-01").Scan(&status, &parcel)
		switch {
		case err != nil:
			return err
		case status != "IMPORTED":
			return errors.New("status " + status + ", want IMPORTED")
		case parcel == nil || *parcel != "PX-IMPORT-MARKED-01":
			return errors.New("it does not point to the parcel PX-IMPORT-MARKED-01")
		}
		return nil
	})
}

func TestAnOverweightManifestLineIsRejectedAndCreatesNoParcel(t *testing.T) {
	db := connect(t)
	addLine(t, db, "ML-IMPORT-HEAVY-01", "MF-IMPORT-HEAVY", "PX-IMPORT-HEAVY-01", "shop-import-heavy", 30500, "STANDARD",
		map[string]string{"name": "Lea Heavy", "city": "Cologne", "postcode": "50667", "country": "DE"})
	within(t, "the manifest line ML-IMPORT-HEAVY-01", func() error {
		var status string
		var reason *string
		err := db.QueryRow(context.Background(), `SELECT status, error FROM parcels.manifest_lines WHERE id = $1`,
			"ML-IMPORT-HEAVY-01").Scan(&status, &reason)
		switch {
		case err != nil:
			return err
		case status != "REJECTED":
			return errors.New("status " + status + ", want REJECTED")
		case reason == nil || *reason != "weight exceeds 30000 g":
			return errors.New("the reason is not \"weight exceeds 30000 g\"")
		}
		return nil
	})
	var n int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM parcels.parcels WHERE reference = $1`, "PX-IMPORT-HEAVY-01").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("a parcel PX-IMPORT-HEAVY-01 was created")
	}
}
