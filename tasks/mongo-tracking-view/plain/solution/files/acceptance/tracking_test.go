package acceptance

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Depot scanners record scans; each parcel's tracking view shows where it is now.

const mongoURI = "mongodb://parcels:parcels@mongo:27017/parcels?authSource=admin"

type scan struct {
	ScanID    string    `bson:"scanId"`
	ParcelRef string    `bson:"parcelRef"`
	Status    string    `bson:"status"`
	Location  string    `bson:"location"`
	ScannedAt time.Time `bson:"scannedAt"`
}

type tracking struct {
	Status       string `bson:"status"`
	LastLocation string `bson:"lastLocation"`
	ScanCount    int    `bson:"scanCount"`
	Delivered    bool   `bson:"delivered"`
}

func database(t *testing.T) *mongo.Database {
	t.Helper()
	client, err := mongo.Connect(options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	return client.Database("parcels")
}

// record writes scans as the depot scanners do.
func record(t *testing.T, db *mongo.Database, scans ...scan) {
	t.Helper()
	docs := make([]any, len(scans))
	for i, s := range scans {
		docs[i] = s
	}
	if _, err := db.Collection("scans").InsertMany(context.Background(), docs); err != nil {
		t.Fatal(err)
	}
}

// view waits until a parcel's tracking view is the wanted one, and still is a second later
// (the projection runs every few seconds).
func view(t *testing.T, db *mongo.Database, ref string, want tracking) {
	t.Helper()
	read := func() (tracking, error) {
		var v tracking
		err := db.Collection("tracking").FindOne(context.Background(), bson.D{{Key: "_id", Value: ref}}).Decode(&v)
		return v, err
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		v, err := read()
		if err == nil && v == want {
			time.Sleep(time.Second)
			if v, err = read(); err == nil && v == want {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("tracking view of %s: %+v (%v), want %+v", ref, v, err, want)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func at(day, clock string) time.Time {
	ts, err := time.Parse(time.RFC3339, fmt.Sprintf("2026-09-%sT%s:00Z", day, clock))
	if err != nil {
		panic(err)
	}
	return ts
}

func TestTrackingShowsTheMostRecentScanEvenWhenScansArriveOutOfOrder(t *testing.T) {
	db := database(t)
	record(t, db,
		scan{"SC-TRK-ORDER-1", "PX-TRK-ORDER-01", "PICKED_UP", "Shop Mitte", at("01", "08:00")},
		scan{"SC-TRK-ORDER-3", "PX-TRK-ORDER-01", "OUT_FOR_DELIVERY", "Leipzig depot", at("01", "12:00")},
		scan{"SC-TRK-ORDER-2", "PX-TRK-ORDER-01", "IN_TRANSIT", "Halle hub", at("01", "10:00")},
	)
	view(t, db, "PX-TRK-ORDER-01", tracking{Status: "OUT_FOR_DELIVERY", LastLocation: "Leipzig depot", ScanCount: 3})
}

func TestAScanSentTwiceIsCountedOnce(t *testing.T) {
	db := database(t)
	record(t, db,
		scan{"SC-TRK-RESENT-1", "PX-TRK-RESENT-01", "PICKED_UP", "Shop Nord", at("02", "08:00")},
		scan{"SC-TRK-RESENT-2", "PX-TRK-RESENT-01", "IN_TRANSIT", "Kiel hub", at("02", "09:30")},
		scan{"SC-TRK-RESENT-2", "PX-TRK-RESENT-01", "IN_TRANSIT", "Kiel hub", at("02", "09:30")},
	)
	view(t, db, "PX-TRK-RESENT-01", tracking{Status: "IN_TRANSIT", LastLocation: "Kiel hub", ScanCount: 2})
}

func TestADeliveredParcelIsShownAsDelivered(t *testing.T) {
	db := database(t)
	record(t, db,
		scan{"SC-TRK-DELIVERED-1", "PX-TRK-DELIVERED-01", "PICKED_UP", "Shop Sued", at("03", "08:00")},
		scan{"SC-TRK-DELIVERED-2", "PX-TRK-DELIVERED-01", "DELIVERED", "Front door", at("03", "16:45")},
	)
	view(t, db, "PX-TRK-DELIVERED-01", tracking{Status: "DELIVERED", LastLocation: "Front door", ScanCount: 2, Delivered: true})
}
