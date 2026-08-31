package database

import (
	"reflect"
	"testing"

	"github.com/uug-ai/models/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestParseTenancyMode(t *testing.T) {
	for input, want := range map[string]TenancyMode{
		"":                TenancyModeCompatibility,
		"  ":              TenancyModeCompatibility,
		"legacy":          TenancyModeLegacy,
		" COMPATIBILITY ": TenancyModeCompatibility,
		"Canonical":       TenancyModeCanonical,
	} {
		got, err := ParseTenancyMode(input)
		if err != nil {
			t.Fatalf("ParseTenancyMode(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseTenancyMode(%q) = %q, want %q", input, got, want)
		}
	}

	if _, err := ParseTenancyMode("mixed"); err == nil {
		t.Fatal("ParseTenancyMode must reject an unknown mode")
	}
}

func TestOwnershipScopeForMode(t *testing.T) {
	organisationId := primitive.NewObjectID().Hex()
	tests := []struct {
		name string
		mode TenancyMode
		want bson.M
	}{
		{name: "legacy", mode: TenancyModeLegacy, want: bson.M{"user_id": "legacy-1"}},
		{
			name: "compatibility",
			mode: TenancyModeCompatibility,
			want: bson.M{"$or": []bson.M{
				{"organisationId": organisationId},
				{"user_id": "legacy-1", "organisationId": bson.M{"$in": bson.A{nil, ""}}},
			}},
		},
		{name: "canonical", mode: TenancyModeCanonical, want: bson.M{"organisationId": organisationId}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := OwnershipScopeForMode(test.mode, organisationId, "user_id", "legacy-1")
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("OwnershipScopeForMode() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestScopeWithProjectForMode(t *testing.T) {
	organisation := primitive.NewObjectID()
	organisationId := organisation.Hex()
	projectId := models.DefaultProjectId(organisation)

	tests := []struct {
		name string
		mode TenancyMode
		want bson.M
	}{
		{name: "legacy", mode: TenancyModeLegacy, want: bson.M{"user_id": "legacy-1"}},
		{
			name: "compatibility",
			mode: TenancyModeCompatibility,
			want: bson.M{"$or": []bson.M{
				{
					"organisationId": organisationId,
					"projectId":      bson.M{"$in": bson.A{projectId, nil}},
				},
				{
					"user_id":        "legacy-1",
					"organisationId": bson.M{"$in": bson.A{nil, ""}},
					"projectId":      bson.M{"$in": bson.A{projectId, nil}},
				},
			}},
		},
		{
			name: "canonical",
			mode: TenancyModeCanonical,
			want: bson.M{"organisationId": organisationId, "projectId": projectId},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ScopeWithProjectForMode(test.mode, organisationId, "user_id", "legacy-1", projectId)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("ScopeWithProjectForMode() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestCanonicalFirstOwnership(t *testing.T) {
	got := CanonicalFirstOwnership("org-123", "master_user_id", "legacy-abc")
	want := bson.M{"$or": []bson.M{
		{"organisationId": "org-123"},
		{
			"master_user_id": "legacy-abc",
			"organisationId": bson.M{"$in": bson.A{nil, ""}},
		},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CanonicalFirstOwnership mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

// A collection that declares the canonical field as its own legacy owner field
// has two tests for one key. Merging them would write one over the other and
// widen the tenant scope, so both must survive as an explicit conjunction.
func TestCanonicalFirstOwnershipKeepsCollidingLegacyFieldSeparate(t *testing.T) {
	got := CanonicalFirstOwnership("org-123", CanonicalOrganisationField, "legacy-abc")
	want := bson.M{"$or": []bson.M{
		{"organisationId": "org-123"},
		{"$and": []bson.M{
			MissingCanonicalOrganisation(),
			{CanonicalOrganisationField: "legacy-abc"},
		}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("colliding legacy field mismatch:\n got: %#v\nwant: %#v", got, want)
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
	want := bson.M{"organisationId": bson.M{"$in": bson.A{nil, ""}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MissingCanonicalOrganisation mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

// The migration predicate must stay a single key holding point-matchable
// values. An $exists arm selects the same documents but cannot be resolved from
// an index, which costs every reader that merges the ownership arms an index
// scan and hands it a collection scan instead.
func TestMissingCanonicalOrganisationIsIndexBounded(t *testing.T) {
	got := MissingCanonicalOrganisation()
	if len(got) != 1 {
		t.Fatalf("predicate must be a single key, got %#v", got)
	}
	candidates, ok := got[CanonicalOrganisationField].(bson.M)["$in"].(bson.A)
	if !ok {
		t.Fatalf("predicate must narrow %s with $in, got %#v", CanonicalOrganisationField, got)
	}
	for _, candidate := range candidates {
		if _, isOperator := candidate.(bson.M); isOperator {
			t.Fatalf("$in must hold plain values, got operator %#v", candidate)
		}
	}
}

func TestDefaultCompatibleProjectScope(t *testing.T) {
	organisationId := primitive.NewObjectID()
	defaultProjectId := organisationId
	wantDefault := bson.M{CanonicalProjectField: bson.M{"$in": bson.A{defaultProjectId, nil}}}
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
	want := bson.M{"$or": []bson.M{
		{
			"organisationId": organisationId,
			"projectId":      project,
		},
		{
			"user_id":        "legacy-1",
			"organisationId": bson.M{"$in": bson.A{nil, ""}},
			"projectId":      project,
		},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScopeWithProject selected-project mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

// The tenant-safety property behind the distributed shape: no ownership arm may
// escape project narrowing. An unnarrowed legacy arm would let a stale owner id
// widen the read across every project in the organisation — the exact leak the
// project clause exists to prevent.
func TestScopeWithProjectNarrowsEveryOwnershipArm(t *testing.T) {
	organisation := primitive.NewObjectID()

	for name, project := range map[string]primitive.ObjectID{
		"default project": models.DefaultProjectId(organisation),
		"real project":    primitive.NewObjectID(),
	} {
		got := ScopeWithProject(organisation.Hex(), "user_id", "legacy-1", project)

		arms, ok := got["$or"].([]bson.M)
		if !ok {
			t.Fatalf("%s: expected a top-level $or, got %#v", name, got)
		}
		if len(arms) != 2 {
			t.Fatalf("%s: expected both ownership arms, got %#v", name, arms)
		}
		for _, arm := range arms {
			if _, narrowed := arm[CanonicalProjectField]; !narrowed {
				t.Fatalf("%s: ownership arm escapes project narrowing: %#v", name, arm)
			}
		}
	}
}

// The distributed form must select exactly what the conjunctive form selected.
// It is the distributive law over an "$or", not a policy change, so every arm
// has to carry the unmodified project clause and nothing else may move.
func TestScopeWithProjectDistributesTheConjunctiveForm(t *testing.T) {
	organisation := primitive.NewObjectID()
	organisationId := organisation.Hex()
	project := models.DefaultProjectId(organisation)

	ownership := CanonicalFirstOwnership(organisationId, "user_id", "legacy-1")
	projectClause := models.ProjectScopeFilter(organisationId, project)

	got := ScopeWithProject(organisationId, "user_id", "legacy-1", project)

	want := bson.M{"$or": []bson.M{}}
	for _, arm := range ownership["$or"].([]bson.M) {
		combined := bson.M{}
		for field, test := range arm {
			combined[field] = test
		}
		for field, test := range projectClause {
			combined[field] = test
		}
		want["$or"] = append(want["$or"].([]bson.M), combined)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScopeWithProject is not the distributed conjunction:\n got: %#v\nwant: %#v", got, want)
	}
}

// The regression this whole helper exists to prevent. Scoping strictly on the
// organisation's default project excludes every document written before the
// project field existed, and does it silently — the query returns zero
// documents, not an error. The default project must therefore also match a
// missing and a null projectId, on every ownership arm.
func TestScopeWithProjectDefaultProjectToleratesUnstampedDocuments(t *testing.T) {
	organisation := primitive.NewObjectID()
	organisationId := organisation.Hex()
	defaultProject := models.DefaultProjectId(organisation)

	got := ScopeWithProject(organisationId, "user_id", "legacy-1", defaultProject)

	arms, ok := got["$or"].([]bson.M)
	if !ok {
		t.Fatalf("expected a top-level $or, got %#v", got)
	}
	for _, arm := range arms {
		candidates, ok := arm[CanonicalProjectField].(bson.M)["$in"].(bson.A)
		if !ok {
			t.Fatalf("arm does not tolerate an unstamped projectId: %#v", arm)
		}

		var tolerated bool
		for _, candidate := range candidates {
			// null matches both an explicit null and a missing field.
			if candidate == nil {
				tolerated = true
			}
		}
		if !tolerated {
			t.Fatalf("arm excludes documents written before the project axis existed: %#v", arm)
		}
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
