package database

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestCanonicalFirstOwnership(t *testing.T) {
	got := CanonicalFirstOwnership("org-123", "master_user_id", "legacy-abc")
	want := bson.M{"$or": []bson.M{
		{"organisationId": "org-123"},
		{"$and": []bson.M{
			{"organisationId": bson.M{"$exists": false}},
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
	want := bson.M{"organisationId": bson.M{"$exists": false}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MissingCanonicalOrganisation mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}
