package database

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CanonicalOrganisationField is the canonical, string-hex tenant ownership key
// written to every organisation-owned document during the organisationId
// migration. Reads scope on this field first and only fall back to a legacy
// owner field for documents that predate the migration.
const CanonicalOrganisationField = "organisationId"

// MissingCanonicalOrganisation matches documents that have not yet been stamped
// with the canonical organisationId. A document counts as missing when the field
// is absent, an empty string, or null, so partially-written or cleared legacy
// documents still fall back to the legacy owner clause during the migration. It
// is the predicate that distinguishes pre-migration (legacy-owned) documents
// from migrated ones, and is exported so index and backfill tooling can reuse
// the exact same shape.
func MissingCanonicalOrganisation() bson.M {
	return bson.M{"$or": []bson.M{
		{CanonicalOrganisationField: bson.M{"$exists": false}},
		{CanonicalOrganisationField: ""},
		{CanonicalOrganisationField: nil},
	}}
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

// CanonicalProjectField is the optional owning-project key on an
// organisation-owned document (models.<Resource>.ProjectId). A project is a
// narrowing axis applied on top of organisation ownership, never a replacement
// for it: a project always lives inside exactly one organisation and project
// scope is ANDed with the canonical-first organisation filter. An absent value
// means the resource is organisation-wide (it belongs to no specific project),
// so it is only returned by organisation-scoped reads, not by a project-scoped
// read.
const CanonicalProjectField = "projectId"

// projectScope returns the project-narrowing predicate for a selected project.
// It is an internal building block for ScopeWithProject, not a tenant filter on
// its own: it carries no organisation ownership, so it must never be used as a
// standalone read filter.
//
// It has two modes:
//
//   - a zero projectId — no narrowing. Returns nil so callers scope by
//     organisation only. This keeps every reader byte-for-byte identical to the
//     pre-projects behaviour until a project is actually selected, and is also
//     the organisation-wide view (which includes resources that belong to no
//     project).
//   - a set projectId — a single equality on the resource's owning projectId.
//     Organisation-wide resources (no owning project) are not matched, matching
//     the model contract where a resource is organisation-wide until it is
//     assigned a project.
//
// Projects are net-new, so projectId is always a real ObjectID with no legacy
// string-hex history to fall back to; a single typed equality is sufficient.
// Cross-project sharing is intentionally not modelled here — if a
// sharedWithProjectIds axis is ever added, this predicate grows an $or.
func projectScope(projectId primitive.ObjectID) bson.M {
	if projectId.IsZero() {
		return nil
	}
	return bson.M{CanonicalProjectField: projectId}
}

// ScopeWithProject returns a tenant-isolation filter that ANDs canonical-first
// organisation ownership with an optional project narrowing.
//
// When projectId is zero the result is exactly CanonicalFirstOwnership, so
// call sites can adopt this helper without changing behaviour and only start
// narrowing once a project is selected. When projectId is set, the project
// predicate (see projectScope) is ANDed on top of organisation ownership so a
// stale legacy owner can never widen a document across projects.
//
// Note for callers that already inject a top-level "$and" into the returned map
// (e.g. name lookups during the organisation migration): when a project is
// selected this helper returns a top-level "$and", so append to that slice
// rather than overwriting the key.
func ScopeWithProject(organisationId, legacyField, legacyValue string, projectId primitive.ObjectID) bson.M {
	ownership := CanonicalFirstOwnership(organisationId, legacyField, legacyValue)
	project := projectScope(projectId)
	if project == nil {
		return ownership
	}
	return bson.M{"$and": []bson.M{ownership, project}}
}

// ScopeWithProject returns the canonical-first ownership filter for this
// collection, optionally narrowed to a project. It mirrors Scope but threads the
// selected project id through projectScope.
func (t TenantField) ScopeWithProject(organisationId, legacyValue string, projectId primitive.ObjectID) bson.M {
	return ScopeWithProject(organisationId, t.Legacy, legacyValue, projectId)
}
