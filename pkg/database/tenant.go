package database

import (
	"fmt"
	"strings"

	"github.com/uug-ai/models/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TenancyMode selects the persisted ownership contract used by readers during
// the organisation/project migration.
type TenancyMode string

const (
	TenancyModeLegacy        TenancyMode = "legacy"
	TenancyModeCompatibility TenancyMode = "compatibility"
	TenancyModeCanonical     TenancyMode = "canonical"
)

// ParseTenancyMode parses deployment configuration. An empty value preserves
// migration-safe compatibility reads; unknown values are rejected so a typo
// cannot silently select a different ownership contract.
func ParseTenancyMode(value string) (TenancyMode, error) {
	mode := TenancyMode(strings.ToLower(strings.TrimSpace(value)))
	if mode == "" {
		return TenancyModeCompatibility, nil
	}
	if !mode.IsValid() {
		return "", fmt.Errorf("invalid tenancy mode %q", value)
	}
	return mode, nil
}

func (m TenancyMode) IsValid() bool {
	return m == TenancyModeLegacy || m == TenancyModeCompatibility || m == TenancyModeCanonical
}

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
//
// It is a single-key $in rather than an $or over an $exists, an empty-string and
// a null arm. The two select the same documents — a null test already matches an
// absent field — but only the $in is answerable from an index, because a missing
// field and an explicit null are stored as the same index entry and no planner
// can tell them apart without fetching the document. Keeping this shape point-
// matchable is what allows the ownership branches to be scanned from an index
// rather than resolved by a collection scan.
func MissingCanonicalOrganisation() bson.M {
	return bson.M{CanonicalOrganisationField: bson.M{"$in": bson.A{nil, ""}}}
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
//
// Each arm is a flat conjunction rather than a nested "$and". That is not
// cosmetic: an index can only serve an "$or" when every arm applies all of its
// bounds at index level, so keeping the arms flat and point-matchable is what
// lets a reader merge them from an index instead of scanning the collection.
func CanonicalFirstOwnership(organisationId, legacyField, legacyValue string) bson.M {
	return OwnershipScopeForMode(TenancyModeCompatibility, organisationId, legacyField, legacyValue)
}

// OwnershipScopeForMode returns the organisation ownership predicate for the
// selected migration contract. Invalid modes degrade to compatibility so a
// directly constructed value cannot accidentally disable migration fallbacks;
// deployment configuration should still be validated with
// ParseTenancyMode at startup.
func OwnershipScopeForMode(mode TenancyMode, organisationId, legacyField, legacyValue string) bson.M {
	if !mode.IsValid() {
		mode = TenancyModeCompatibility
	}
	switch mode {
	case TenancyModeLegacy:
		return bson.M{legacyField: legacyValue}
	case TenancyModeCanonical:
		return bson.M{CanonicalOrganisationField: organisationId}
	}

	legacy := bson.M{legacyField: legacyValue}
	for field, test := range MissingCanonicalOrganisation() {
		if _, collides := legacy[field]; collides {
			// The collection declares the canonical field as its own legacy
			// owner field. Merging would write one test over the other and
			// widen the tenant scope, so keep both as an explicit conjunction.
			legacy = bson.M{"$and": []bson.M{
				MissingCanonicalOrganisation(),
				{legacyField: legacyValue},
			}}
			break
		}
		legacy[field] = test
	}

	return bson.M{"$or": []bson.M{
		{CanonicalOrganisationField: organisationId},
		legacy,
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

// ScopeForMode returns this collection's organisation ownership predicate for
// the selected migration contract.
func (t TenantField) ScopeForMode(mode TenancyMode, organisationId, legacyValue string) bson.M {
	return OwnershipScopeForMode(mode, organisationId, t.Legacy, legacyValue)
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

// ScopeWithProject returns a tenant-isolation filter that combines
// canonical-first organisation ownership with an optional project narrowing.
//
// When projectId is zero the result is exactly CanonicalFirstOwnership, so call
// sites can adopt this helper without changing behaviour and only start
// narrowing once a project is selected. When projectId is set, the project
// predicate is applied to every ownership arm, so a stale legacy owner can never
// widen a document across projects.
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
// The project clause is pushed into each ownership arm instead of being ANDed
// on top of them, and that shape is load-bearing. An index can serve an "$or"
// only when every arm applies all of its bounds at index level; a clause left
// outside the "$or" is a residual the planner must resolve per candidate
// document, which collapses an indexed read into a full scan of the tenant's
// collection followed by a blocking sort. Distributing it keeps every arm
// index-resolvable, so a paged, sorted read merges a few index entries rather
// than examining the whole tenant. The two shapes select identical documents —
// this is the distributive law, not a policy change.
//
// An organisationId that is not valid hex yields no project narrowing at all
// (models.ProjectScopeFilter degrades to nil). The read stays bounded by the
// ownership clause, so this fails organisation-wide rather than blanking a
// tenant's screen over a resolution glitch.
//
// Note for callers that add their own clauses to the returned map: this helper
// now always returns a top-level "$or", with or without a project. Adding a
// sibling key (match["deviceId"] = ...) is safe and ANDs as expected, but
// assigning to "$or" would overwrite the tenant boundary. Wrap the result in an
// "$and" instead of writing into it. Be aware that any clause added outside the
// "$or" is the residual described above: it is correct, but it gives up the
// index merge, so prefer narrowing on a field the arms already constrain.
func ScopeWithProject(organisationId, legacyField, legacyValue string, projectId primitive.ObjectID) bson.M {
	return ScopeWithProjectForMode(TenancyModeCompatibility, organisationId, legacyField, legacyValue, projectId)
}

// ScopeWithProjectForMode combines the organisation and project predicates for
// the selected migration contract. Canonical mode collapses to flat exact
// equalities, while compatibility mode retains the guarded, index-mergeable
// ownership branches and default-project tolerance.
func ScopeWithProjectForMode(mode TenancyMode, organisationId, legacyField, legacyValue string, projectId primitive.ObjectID) bson.M {
	if !mode.IsValid() {
		mode = TenancyModeCompatibility
	}
	ownership := OwnershipScopeForMode(mode, organisationId, legacyField, legacyValue)

	var project bson.M
	switch mode {
	case TenancyModeLegacy:
		project = nil
	case TenancyModeCanonical:
		if !projectId.IsZero() {
			project = bson.M{CanonicalProjectField: projectId}
		}
	default:
		project = models.ProjectScopeFilter(organisationId, projectId)
	}
	if project == nil {
		return ownership
	}

	arms, ok := ownership["$or"].([]bson.M)
	if !ok {
		return mergeScopes(ownership, project)
	}

	narrowed := make([]bson.M, 0, len(arms))
	for _, arm := range arms {
		combined := mergeScopes(arm, project)
		if _, conjunctive := combined["$and"]; conjunctive {
			return bson.M{"$and": []bson.M{ownership, project}}
		}
		narrowed = append(narrowed, combined)
	}

	return bson.M{"$or": narrowed}
}

func mergeScopes(first, second bson.M) bson.M {
	combined := bson.M{}
	for field, test := range first {
		combined[field] = test
	}
	for field, test := range second {
		if _, collides := combined[field]; collides {
			return bson.M{"$and": []bson.M{first, second}}
		}
		combined[field] = test
	}
	return combined
}

// ScopeWithProject returns the canonical-first ownership filter for this
// collection, optionally narrowed to a project. It mirrors Scope but threads the
// selected project id through the shared project predicate.
func (t TenantField) ScopeWithProject(organisationId, legacyValue string, projectId primitive.ObjectID) bson.M {
	return ScopeWithProject(organisationId, t.Legacy, legacyValue, projectId)
}

// ScopeWithProjectForMode returns this collection's organisation/project
// predicate for the selected migration contract.
func (t TenantField) ScopeWithProjectForMode(mode TenancyMode, organisationId, legacyValue string, projectId primitive.ObjectID) bson.M {
	return ScopeWithProjectForMode(mode, organisationId, t.Legacy, legacyValue, projectId)
}
