package database

import (
	"reflect"
	"testing"

	"github.com/uug-ai/models/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestCanonicalFirstOwnership(t *testing.T) {
	got := CanonicalFirstOwnership("org-123", "master_user_id", "legacy-abc")
	want := bson.M{"$or": []bson.M{
		{"organisationId": "org-123"},
		{"$and": []bson.M{
			MissingCanonicalOrganisation(),
			{"master_user_id": "legacy-abc"},
		}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CanonicalFirstOwnership mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestTenantFieldScope(t *testing.T) {
	tenant := TenantField{Legacy: "user_id"}
	got := tenant.Scope("org-9", "legacy-9")
	want := CanonicalFirstOwnership("org-9", "user_id", "legacy-9")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TenantField.Scope mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestMissingCanonicalOrganisation(t *testing.T) {
	got := MissingCanonicalOrganisation()
	want := bson.M{"$or": []bson.M{
		{"organisationId": bson.M{"$exists": false}},
		{"organisationId": ""},
		{"organisationId": nil},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MissingCanonicalOrganisation mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestDefaultCompatibleProjectScope(t *testing.T) {
	organisationId := primitive.NewObjectID()
	defaultProjectId := organisationId
	wantDefault := bson.M{"$or": []bson.M{
		{CanonicalProjectField: defaultProjectId},
		{CanonicalProjectField: bson.M{"$exists": false}},
		{CanonicalProjectField: nil},
	}}
	if got := DefaultCompatibleProjectScope(organisationId, defaultProjectId); !reflect.DeepEqual(got, wantDefault) {
		t.Fatalf("default-compatible scope = %#v, want %#v", got, wantDefault)
	}

	nonDefaultProjectId := primitive.NewObjectID()
	wantNonDefault := bson.M{CanonicalProjectField: nonDefaultProjectId}
	if got := DefaultCompatibleProjectScope(organisationId, nonDefaultProjectId); !reflect.DeepEqual(got, wantNonDefault) {
		t.Fatalf("non-default scope = %#v, want %#v", got, wantNonDefault)
	}

	if got := DefaultCompatibleProjectScope(organisationId, primitive.NilObjectID); got != nil {
		t.Fatalf("zero project scope = %#v, want nil", got)
	}
}

// The adapter must not develop semantics of its own: whatever the shared rule
// says for a given organisation/project pair is what this returns.
func TestDefaultCompatibleProjectScopeMatchesSharedPredicate(t *testing.T) {
	organisationId := primitive.NewObjectID()
	for name, projectId := range map[string]primitive.ObjectID{
		"default":     organisationId,
		"non-default": primitive.NewObjectID(),
		"zero":        primitive.NilObjectID,
	} {
		got := DefaultCompatibleProjectScope(organisationId, projectId)
		want := models.ProjectScopeFilter(organisationId.Hex(), projectId)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s project: adapter = %#v, shared predicate = %#v", name, got, want)
		}
	}
}

func TestScopeWithProjectUnsetMatchesOwnership(t *testing.T) {
	got := ScopeWithProject("org-1", "user_id", "legacy-1", primitive.NilObjectID)
	want := CanonicalFirstOwnership("org-1", "user_id", "legacy-1")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScopeWithProject with zero projectId must equal CanonicalFirstOwnership:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestScopeWithProjectSelectedProject(t *testing.T) {
	organisationId := primitive.NewObjectID().Hex()
	project := primitive.NewObjectID()
	got := ScopeWithProject(organisationId, "user_id", "legacy-1", project)
	want := bson.M{"$and": []bson.M{
		CanonicalFirstOwnership(organisationId, "user_id", "legacy-1"),
		{"projectId": project},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScopeWithProject selected-project mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

// The regression this whole helper exists to prevent. Scoping strictly on the
// organisation's default project excludes every document written before the
// project field existed, and does it silently — the query returns zero
// documents, not an error. The default project must therefore also match a
// missing and a null projectId.
func TestScopeWithProjectDefaultProjectToleratesUnstampedDocuments(t *testing.T) {
	organisation := primitive.NewObjectID()
	organisationId := organisation.Hex()
	defaultProject := models.DefaultProjectId(organisation)

	got := ScopeWithProject(organisationId, "user_id", "legacy-1", defaultProject)
	want := bson.M{"$and": []bson.M{
		CanonicalFirstOwnership(organisationId, "user_id", "legacy-1"),
		{"$or": []bson.M{
			{"projectId": defaultProject},
			{"projectId": bson.M{"$exists": false}},
			{"projectId": nil},
		}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScopeWithProject default-project mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

// An organisation id that is not an ObjectID cannot be checked against the
// default project, so the project axis drops out entirely and the read stays
// bounded by ownership alone. Degrading to organisation-wide is deliberate:
// the alternative, an unmatchable predicate, blanks a tenant's screen over a
// resolution glitch.
func TestScopeWithProjectNonHexOrganisationDegradesToOwnership(t *testing.T) {
	got := ScopeWithProject("legacy-org", "user_id", "legacy-1", primitive.NewObjectID())
	want := CanonicalFirstOwnership("legacy-org", "user_id", "legacy-1")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("non-hex organisation must not narrow by project:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestTenantFieldScopeWithProject(t *testing.T) {
	tenant := TenantField{Legacy: "owner_id"}
	organisationId := primitive.NewObjectID().Hex()
	project := primitive.NewObjectID()
	got := tenant.ScopeWithProject(organisationId, "legacy-9", project)
	want := ScopeWithProject(organisationId, "owner_id", "legacy-9", project)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TenantField.ScopeWithProject mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}
