package database

import (
	"reflect"
	"testing"

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

func TestProjectScopeUnset(t *testing.T) {
	if got := projectScope(primitive.NilObjectID); got != nil {
		t.Fatalf("projectScope with zero projectId should be nil, got: %#v", got)
	}
}

func TestProjectScopeSelectedProject(t *testing.T) {
	project := primitive.NewObjectID()
	got := projectScope(project)
	want := bson.M{"projectId": project}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectScope selected-project mismatch:\n got: %#v\nwant: %#v", got, want)
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

func TestScopeWithProjectUnsetMatchesOwnership(t *testing.T) {
	got := ScopeWithProject("org-1", "user_id", "legacy-1", primitive.NilObjectID)
	want := CanonicalFirstOwnership("org-1", "user_id", "legacy-1")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScopeWithProject with zero projectId must equal CanonicalFirstOwnership:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestScopeWithProjectSelectedProject(t *testing.T) {
	project := primitive.NewObjectID()
	got := ScopeWithProject("org-1", "user_id", "legacy-1", project)
	want := bson.M{"$and": []bson.M{
		CanonicalFirstOwnership("org-1", "user_id", "legacy-1"),
		{"projectId": project},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScopeWithProject selected-project mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestTenantFieldScopeWithProject(t *testing.T) {
	tenant := TenantField{Legacy: "owner_id"}
	project := primitive.NewObjectID()
	got := tenant.ScopeWithProject("org-9", "legacy-9", project)
	want := ScopeWithProject("org-9", "owner_id", "legacy-9", project)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TenantField.ScopeWithProject mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}
