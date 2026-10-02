// Package mutant lists the deliberate bugs the evals app (the parcels service)
// can be started with, and its correct variants. A task's verifier runs the
// agent's tests against the correct app, against each correct variant and
// against each mutant the task targets: a good acceptance test passes on the
// first two and fails on every targeted mutant.
//
// The app reads the active mutants from EVALS_MUTANT and the active variants
// from EVALS_VARIANT (both comma-separated). The variables are set only by the
// verifier, never in the agent's environment.
package mutant

import (
	"sort"
	"strings"
)

// EnvVar selects the active mutants.
const EnvVar = "EVALS_MUTANT"

// Mutant is one deliberate bug.
type Mutant struct {
	Name string
	// Area is the part of the service it breaks (for docs and reports).
	Area string
	// Bug describes what goes wrong, in product terms.
	Bug string
}

// All is every mutant the app implements.
var All = []Mutant{
	{"wrong-status-on-create", "rest", "registering a parcel answers 200 instead of 201"},
	{"wrong-status-on-duplicate", "rest", "registering an existing reference answers 200 with the existing parcel instead of 409"},
	{"update-not-persisted", "rest", "PATCH answers 200 with the changed parcel but does not store the change"},
	{"delete-not-removed", "rest", "DELETE answers 204 but the parcel can still be fetched"},
	{"list-ignores-sender-filter", "rest", "listing parcels by sender returns every sender's parcels"},
	{"no-weight-limit", "validation", "parcels heavier than 30000 g are accepted"},
	{"no-service-level-check", "validation", "unknown service levels are accepted"},
	{"skips-address-check", "address", "the address service is never called; the zone is UNKNOWN"},
	{"address-check-without-api-key", "address", "the address service is called without the X-Api-Key header"},
	{"ignores-undeliverable", "address", "parcels to undeliverable postcodes are registered anyway"},
	{"import-drops-postcode", "import", "imported parcels lose the recipient postcode"},
	{"import-leaves-line-pending", "import", "imported manifest lines stay PENDING"},
	{"import-wrong-source", "import", "imported parcels are recorded with source api instead of manifest"},
	{"import-accepts-overweight", "import", "overweight manifest lines are imported instead of rejected"},
	{"tracking-status-from-first-scan", "tracking", "the tracking view shows the earliest scan instead of the latest"},
	{"tracking-counts-duplicate-scans", "tracking", "duplicate scans (same scan id) are counted twice"},
	{"event-not-published", "events", "no ParcelRegistered event is published"},
	{"event-weight-in-kilograms", "events", "the event's weightGrams carries kilograms"},
}

// VariantEnvVar selects the active correct variants.
const VariantEnvVar = "EVALS_VARIANT"

// Variant is a correct build of the app that differs from the default the way
// real services do, within what the task's docs and contract promise. Tests
// that depend on what the contract leaves open (the order of JSON properties,
// how fast a background job is, how the broker stores events) fail on one.
type Variant struct {
	Name string
	// Area is the part of the service it changes (for docs and reports).
	Area string
	// Change describes the difference, in product terms.
	Change string
}

// Variants is every correct variant the app implements.
var Variants = []Variant{
	{"json-properties-reordered", "rest", "JSON response bodies list their properties in another order"},
	{"import-takes-seconds", "import", "manifest lines are imported 3 seconds after they arrive instead of at the next poll (the docs promise within a few seconds)"},
	{"tracking-takes-seconds", "tracking", "the tracking view is updated 3 seconds after a parcel's latest scan arrives (the docs promise within a few seconds)"},
	{"events-topic-gzip", "events", "the events topic is configured with compression.type=gzip, so the broker stores every event in a gzip-compressed batch"},
}

// KnownVariant reports whether name is a correct variant.
func KnownVariant(name string) bool {
	for _, v := range Variants {
		if v.Name == name {
			return true
		}
	}
	return false
}

// VariantChange describes a variant (empty for an unknown one).
func VariantChange(name string) string {
	for _, v := range Variants {
		if v.Name == name {
			return v.Change
		}
	}
	return ""
}

// Known reports whether name is a mutant.
func Known(name string) bool {
	for _, m := range All {
		if m.Name == name {
			return true
		}
	}
	return false
}

// Names returns every mutant name, sorted.
func Names() []string {
	out := make([]string, len(All))
	for i, m := range All {
		out[i] = m.Name
	}
	sort.Strings(out)
	return out
}

// Set is the set of active mutants (or variants).
type Set map[string]bool

// Parse reads a comma-separated EVALS_MUTANT value. Unknown names are
// returned separately so the app can refuse to start with a typo.
func Parse(v string) (Set, []string) { return parse(v, Known) }

// ParseVariants reads a comma-separated EVALS_VARIANT value, as Parse does.
func ParseVariants(v string) (Set, []string) { return parse(v, KnownVariant) }

func parse(v string, known func(string) bool) (Set, []string) {
	s := Set{}
	var unknown []string
	for _, n := range strings.Split(v, ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if !known(n) {
			unknown = append(unknown, n)
			continue
		}
		s[n] = true
	}
	return s, unknown
}

// On reports whether the named mutant (or variant) is active.
func (s Set) On(name string) bool { return s[name] }
