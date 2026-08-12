package database

import "go.mongodb.org/mongo-driver/bson"

// CanonicalOrganisationField is the canonical, string-hex tenant ownership key
// written to every organisation-owned document during the organisationId
// migration. Reads scope on this field first and only fall back to a legacy
// owner field for documents that predate the migration.
const CanonicalOrganisationField = "organisationId"

// MissingCanonicalOrganisation matches documents that have not yet been stamped
// with the canonical organisationId. It is the predicate that distinguishes
// pre-migration (legacy-owned) documents from migrated ones, and is exported so
// index and backfill tooling can reuse the exact same shape.
func MissingCanonicalOrganisation() bson.M {
	return bson.M{CanonicalOrganisationField: bson.M{"$exists": false}}
}

// CanonicalFirstOwnership returns a tenant-isolation filter that matches:
//
//   - documents already owned by the canonical organisationId, OR
//   - legacy documents that predate the migration (no organisationId yet) whose
//     legacy owner field equals legacyValue.
//
// This is the single "dual-read" shape shared across services during the
// organisationId migration. Callers resolve organisationId and legacyValue with
// their own models helpers (e.g. models.GetOrganisationId /
// models.GetUserIdFromAccountOrMaster) and pass the resolved strings here, so
// this package stays a dependency-light leaf with no models import.
//
// Once every document has been backfilled with organisationId, the legacy
// branch can be dropped and this collapses to a single equality on
// organisationId.
func CanonicalFirstOwnership(organisationId, legacyField, legacyValue string) bson.M {
	return bson.M{"$or": []bson.M{
		{CanonicalOrganisationField: organisationId},
		{"$and": []bson.M{
			MissingCanonicalOrganisation(),
			{legacyField: legacyValue},
		}},
	}}
}

// TenantField is a per-collection descriptor: the name of the legacy owner field
// for one collection is declared once, then scopes are built from a resolved
// identity. It lets each service register its collections' legacy fields in a
// single place instead of re-hardcoding the field string at every read site.
type TenantField struct {
	// Legacy is the pre-migration owner field for the collection
	// (e.g. "user_id", "master_user_id", "userid", "owner_id").
	Legacy string
}

// Scope returns the canonical-first ownership filter for this collection from an
// already-resolved canonical organisationId and legacy owner value.
func (t TenantField) Scope(organisationId, legacyValue string) bson.M {
	return CanonicalFirstOwnership(organisationId, t.Legacy, legacyValue)
}
