package database

import (
	"github.com/uug-ai/models/pkg/models"
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
// means the resource belongs to no specific project and is therefore owned by
// its organisation's default project, which is why the project predicate
// tolerates a missing field for that project and only for it.
//
// The field name is declared here for index and backfill tooling. The predicate
// itself is not: it lives in models.ProjectScopeFilter, because the writer that
// stamps the field (ingest, via models.ResolveProjectId) must not depend on this
// module. Keep this constant in step with the key that helper matches on.
const CanonicalProjectField = "projectId"

// DefaultCompatibleProjectScope returns the project-narrowing predicate for a
// selected project: exact equality for a real project, and for the
// organisation's default project an $or that also matches documents written
// before the project axis existed. A zero project returns nil, meaning "do not
// narrow", which leaves the query organisation-wide.
//
// Deprecated: this is now a thin adapter over models.ProjectScopeFilter, which
// is the single definition of the rule and is what the ingest writer's stamping
// is paired with. Prefer calling that helper directly, or ScopeWithProject when
// the organisation clause is wanted too. This wrapper exists only because it
// takes the organisation as an ObjectID rather than a hex string.
func DefaultCompatibleProjectScope(organisationId, projectId primitive.ObjectID) bson.M {
	return models.ProjectScopeFilter(organisationId.Hex(), projectId)
}

// ScopeWithProject returns a tenant-isolation filter that ANDs canonical-first
// organisation ownership with an optional project narrowing.
//
// When projectId is zero the result is exactly CanonicalFirstOwnership, so call
// sites can adopt this helper without changing behaviour and only start
// narrowing once a project is selected. When projectId is set, the project
// predicate is ANDed on top of organisation ownership so a stale legacy owner
// can never widen a document across projects.
//
// The project half is models.ProjectScopeFilter, not a local equality. That
// distinction is the whole point of routing through it: a caller who resolves
// the hidden default project and then scopes strictly on it excludes every
// document written before the field existed — an organisation's entire
// pre-rollout history — and does so silently, because a predicate that stops
// matching returns zero documents rather than an error. The tolerance is
// conditional on the default project for the mirror-image reason: relaxing it
// unconditionally reads identically today and becomes a cross-project leak the
// day a second project exists.
//
// An organisationId that is not valid hex yields no project narrowing at all
// (models.ProjectScopeFilter degrades to nil). The read stays bounded by the
// ownership clause, so this fails organisation-wide rather than blanking a
// tenant's screen over a resolution glitch.
//
// Note for callers that already inject a top-level "$and" into the returned map
// (e.g. name lookups during the organisation migration): when a project is
// selected this helper returns a top-level "$and", so append to that slice
// rather than overwriting the key.
func ScopeWithProject(organisationId, legacyField, legacyValue string, projectId primitive.ObjectID) bson.M {
	ownership := CanonicalFirstOwnership(organisationId, legacyField, legacyValue)
	project := models.ProjectScopeFilter(organisationId, projectId)
	if project == nil {
		return ownership
	}
	return bson.M{"$and": []bson.M{ownership, project}}
}

// ScopeWithProject returns the canonical-first ownership filter for this
// collection, optionally narrowed to a project. It mirrors Scope but threads the
// selected project id through the shared project predicate.
func (t TenantField) ScopeWithProject(organisationId, legacyValue string, projectId primitive.ObjectID) bson.M {
	return ScopeWithProject(organisationId, t.Legacy, legacyValue, projectId)
}
