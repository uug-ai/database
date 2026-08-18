// Package tenancy resolves the immutable organisation and project that own a
// source device, and therefore own every resource a pipeline derives from that
// device's media.
//
// This lives in a shared module on purpose. Ownership resolution is not a
// per-service concern that each pipeline may answer its own way: the answer is
// stamped onto media, markers, counts, and notifications that a single Hub API
// reader later scopes with one selector. Two services that resolve the same
// device differently do not produce a visible error — they produce resources
// that are silently invisible to the tenant, or visible to the wrong one. The
// only way to prevent that across independently deployed services is to have
// exactly one implementation.
//
// Two rules govern the resolution, and they are deliberately asymmetric:
//
//   - The organisation is verified against persisted records. It is the tenant
//     boundary, a wrong value crosses tenants, and a populated organisation
//     collection exists to check against. Ambiguity fails closed.
//   - The project is assigned from a definition, never looked up. During the
//     hidden single-project rollout no authoritative project record exists, so
//     any query would let two services disagree. See models.ResolveProjectId.
package tenancy

import (
	"context"
	"errors"
	"fmt"

	"github.com/uug-ai/database/pkg/database"
	"github.com/uug-ai/models/pkg/models"
	"github.com/uug-ai/models/pkg/properties"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Collections names the database and collections the resolver reads. Services
// historically declared these constants themselves and do not all spell them
// the same way, so they are configuration rather than package constants.
type Collections struct {
	Database      string
	Devices       string
	Users         string
	Organisations string
}

// DefaultCollections is the Kerberos Hub layout every current pipeline service
// uses. Services with a different layout override the fields they need.
func DefaultCollections() Collections {
	return Collections{
		Database:      "Kerberos",
		Devices:       "devices",
		Users:         "users",
		Organisations: "organisation",
	}
}

func (c Collections) withDefaults() Collections {
	defaults := DefaultCollections()
	if c.Database == "" {
		c.Database = defaults.Database
	}
	if c.Devices == "" {
		c.Devices = defaults.Devices
	}
	if c.Users == "" {
		c.Users = defaults.Users
	}
	if c.Organisations == "" {
		c.Organisations = defaults.Organisations
	}
	return c
}

// Ownership is the resolved tenant placement of a device. Both fields are
// always set on success: ProjectId is never the zero value, because an
// unassigned device falls back to the organisation default rather than to
// "no project".
type Ownership struct {
	OrganisationId primitive.ObjectID
	ProjectId      primitive.ObjectID
}

// DeviceResolver resolves device ownership against one database.
type DeviceResolver struct {
	db          *database.Database
	collections Collections
}

// NewDeviceResolver binds a resolver to a database. Zero-valued Collections
// fields fall back to DefaultCollections.
func NewDeviceResolver(db *database.Database, collections Collections) *DeviceResolver {
	return &DeviceResolver{db: db, collections: collections.withDefaults()}
}

type persistedOrganisationOwnership struct {
	Id            primitive.ObjectID `bson:"_id"`
	OwnerId       primitive.ObjectID `bson:"ownerId"`
	LegacyOwnerId string             `bson:"owner_id"`
}

type persistedUserOwnership struct {
	Id            primitive.ObjectID `bson:"_id"`
	MasterAccount string             `bson:"user_id"`
}

// DeviceProjection lists the fields ResolveDevice needs. Callers that load
// devices themselves must include all of them: omitting the project field
// yields a nil value that is indistinguishable from "unassigned", so the
// resolver would silently default a device that actually has a project.
func DeviceProjection() *database.Projection {
	return database.NewProjection().Include(
		properties.DeviceId,
		properties.DeviceKey,
		properties.DeviceOrganisationId,
		properties.DeviceProjectId,
		properties.DeviceUserId,
	)
}

// LoadDevice reads the ownership fields of a source device by its key.
func (r *DeviceResolver) LoadDevice(ctx context.Context, deviceKey string) (models.Device, error) {
	if r == nil || r.db == nil || r.db.Client == nil {
		return models.Device{}, fmt.Errorf("source device %q lookup requires a database", deviceKey)
	}

	databaseCtx, cancel := context.WithTimeout(ctx, r.db.Client.GetTimeout())
	defer cancel()

	var device models.Device
	err := r.db.Client.FindOne(
		databaseCtx,
		r.collections.Database,
		r.collections.Devices,
		map[string]string{properties.DeviceKey: deviceKey},
		DeviceProjection(),
	).Into(&device)
	if err != nil {
		return models.Device{}, fmt.Errorf("find source device %q: %w", deviceKey, err)
	}
	if device.Id.IsZero() {
		return models.Device{}, fmt.Errorf("source device %q has no persisted identity", deviceKey)
	}
	return device, nil
}

// ResolveDeviceByKey loads a device and resolves its ownership in one step.
func (r *DeviceResolver) ResolveDeviceByKey(ctx context.Context, deviceKey string) (models.Device, Ownership, error) {
	device, err := r.LoadDevice(ctx, deviceKey)
	if err != nil {
		return models.Device{}, Ownership{}, err
	}
	ownership, err := r.ResolveDevice(ctx, device)
	if err != nil {
		return models.Device{}, Ownership{}, err
	}
	return device, ownership, nil
}

// ResolveDevice returns the organisation and project owning a device.
func (r *DeviceResolver) ResolveDevice(ctx context.Context, device models.Device) (Ownership, error) {
	organisationId, err := r.resolveOrganisation(ctx, device)
	if err != nil {
		return Ownership{}, err
	}
	return Ownership{
		OrganisationId: organisationId,
		ProjectId:      models.ResolveProjectId(organisationId, device.ProjectId),
	}, nil
}

func (r *DeviceResolver) resolveOrganisation(ctx context.Context, device models.Device) (primitive.ObjectID, error) {
	if device.OrganisationId != "" {
		organisationId, err := primitive.ObjectIDFromHex(device.OrganisationId)
		if err != nil {
			return primitive.NilObjectID, fmt.Errorf("source device %q has invalid canonical organisation ownership", device.Key)
		}
		return organisationId, nil
	}

	if device.UserId == "" {
		return primitive.NilObjectID, fmt.Errorf("source device %q has no canonical or legacy organisation ownership", device.Key)
	}
	legacyId, err := primitive.ObjectIDFromHex(device.UserId)
	if err != nil {
		return primitive.NilObjectID, fmt.Errorf("source device %q has no valid organisation ownership", device.Key)
	}
	if r == nil || r.db == nil || r.db.Client == nil {
		return primitive.NilObjectID, fmt.Errorf("source device %q legacy ownership requires database resolution", device.Key)
	}

	// Newer compatibility writers mirror the organisation id into user_id. An
	// exact organisation identity is therefore the strongest legacy candidate.
	if _, found, err := r.findOrganisationById(ctx, legacyId); err != nil {
		return primitive.NilObjectID, err
	} else if found {
		return legacyId, nil
	}

	// Older heartbeat documents may contain the concrete actor in user_id. Use
	// the stable master relationship, never the actor's mutable active selection.
	ownerId := legacyId
	user, userExists, err := r.findUserOwnership(ctx, legacyId)
	if err != nil {
		return primitive.NilObjectID, err
	}
	if userExists && user.MasterAccount != "" {
		ownerId, err = primitive.ObjectIDFromHex(user.MasterAccount)
		if err != nil {
			return primitive.NilObjectID, fmt.Errorf("source device %q legacy user has invalid master ownership", device.Key)
		}
		if _, found, err := r.findOrganisationById(ctx, ownerId); err != nil {
			return primitive.NilObjectID, err
		} else if found {
			return ownerId, nil
		}
	}

	organisations, err := r.findOrganisationsByOwner(ctx, ownerId)
	if err != nil {
		return primitive.NilObjectID, err
	}
	if len(organisations) == 1 {
		return organisations[0].Id, nil
	}
	if len(organisations) > 1 {
		return primitive.NilObjectID, fmt.Errorf("source device %q legacy owner resolves to %d organisations", device.Key, len(organisations))
	}

	// No organisation record exists at all, so organisations-bootstrap has not
	// run on this instance. A primary organisation deterministically reuses the
	// master user's id, so deriving it here produces exactly the value the
	// migration will later persist. Failing instead would drop every message on
	// an un-migrated deployment.
	//
	// The derivation is only meaningful if the owner has a user record, because
	// bootstrap mints one organisation per master user. An owner with no user
	// record is orphaned: deriving an id for it would stamp media with a tenant
	// the migration never creates, which no reader would ever select.
	ownerExists := userExists
	if ownerId != legacyId {
		// Ownership followed a master link, so userExists only proves the
		// sub-user is real. The derived id is the master's, and a dangling
		// user_id must not be turned into an organisation.
		if _, ownerExists, err = r.findUserOwnership(ctx, ownerId); err != nil {
			return primitive.NilObjectID, err
		}
	}
	if !ownerExists {
		return primitive.NilObjectID, fmt.Errorf("source device %q legacy owner %s has no user or organisation record", device.Key, ownerId.Hex())
	}
	return ownerId, nil
}

func (r *DeviceResolver) findOrganisationById(ctx context.Context, id primitive.ObjectID) (persistedOrganisationOwnership, bool, error) {
	databaseCtx, cancel := context.WithTimeout(ctx, r.db.Client.GetTimeout())
	defer cancel()

	var organisation persistedOrganisationOwnership
	err := r.db.Client.FindOne(databaseCtx, r.collections.Database, r.collections.Organisations, map[string]any{
		properties.OrganisationId: id,
	}).Into(&organisation)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return persistedOrganisationOwnership{}, false, nil
	}
	if err != nil {
		return persistedOrganisationOwnership{}, false, fmt.Errorf("find organisation %s: %w", id.Hex(), err)
	}
	if organisation.Id.IsZero() {
		return persistedOrganisationOwnership{}, false, fmt.Errorf("organisation %s has no persisted identity", id.Hex())
	}
	return organisation, true, nil
}

func (r *DeviceResolver) findUserOwnership(ctx context.Context, id primitive.ObjectID) (persistedUserOwnership, bool, error) {
	databaseCtx, cancel := context.WithTimeout(ctx, r.db.Client.GetTimeout())
	defer cancel()

	projection := database.NewProjection().Include(properties.UserId, properties.UserMasterAccount)
	var user persistedUserOwnership
	err := r.db.Client.FindOne(databaseCtx, r.collections.Database, r.collections.Users, map[string]any{
		properties.UserId: id,
	}, projection).Into(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return persistedUserOwnership{}, false, nil
	}
	if err != nil {
		return persistedUserOwnership{}, false, fmt.Errorf("find legacy device user %s: %w", id.Hex(), err)
	}
	return user, true, nil
}

func (r *DeviceResolver) findOrganisationsByOwner(ctx context.Context, ownerId primitive.ObjectID) ([]persistedOrganisationOwnership, error) {
	databaseCtx, cancel := context.WithTimeout(ctx, r.db.Client.GetTimeout())
	defer cancel()

	filter := map[string]any{"$or": []any{
		map[string]any{properties.OrganisationOwnerId: ownerId},
		map[string]any{"owner_id": ownerId.Hex()},
	}}
	var organisations []persistedOrganisationOwnership
	if err := r.db.Client.Find(databaseCtx, r.collections.Database, r.collections.Organisations, filter, options.Find().SetLimit(2)).All(&organisations); err != nil {
		return nil, fmt.Errorf("find organisations owned by %s: %w", ownerId.Hex(), err)
	}
	for _, organisation := range organisations {
		if organisation.Id.IsZero() {
			return nil, fmt.Errorf("organisation owned by %s has no persisted identity", ownerId.Hex())
		}
		if !organisation.OwnerId.IsZero() && organisation.OwnerId != ownerId {
			return nil, fmt.Errorf("organisation %s has conflicting canonical owner %s and legacy owner %s", organisation.Id.Hex(), organisation.OwnerId.Hex(), ownerId.Hex())
		}
		if organisation.LegacyOwnerId != "" && organisation.LegacyOwnerId != ownerId.Hex() {
			return nil, fmt.Errorf("organisation %s has conflicting legacy owner %s and resolved owner %s", organisation.Id.Hex(), organisation.LegacyOwnerId, ownerId.Hex())
		}
	}
	return organisations, nil
}
