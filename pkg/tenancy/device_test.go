package tenancy

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/uug-ai/database/pkg/database"
	"github.com/uug-ai/models/pkg/models"
	"github.com/uug-ai/models/pkg/properties"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func testResolver(collections Collections) (*DeviceResolver, *database.MockDatabase) {
	mock := database.NewMockDatabase()
	return NewDeviceResolver(&database.Database{Client: mock}, collections), mock
}

func assertOwnership(t *testing.T, resolver *DeviceResolver, device models.Device, wantOrganisationId, wantProjectId primitive.ObjectID) {
	t.Helper()

	ownership, err := resolver.ResolveDevice(context.Background(), device)
	if err != nil {
		t.Fatalf("ResolveDevice() error = %v", err)
	}
	if ownership.OrganisationId != wantOrganisationId {
		t.Fatalf("organisationId = %s, want %s", ownership.OrganisationId.Hex(), wantOrganisationId.Hex())
	}
	if ownership.ProjectId != wantProjectId {
		t.Fatalf("projectId = %s, want %s", ownership.ProjectId.Hex(), wantProjectId.Hex())
	}
}

func assertFailsClosed(t *testing.T, resolver *DeviceResolver, device models.Device, why string) {
	t.Helper()

	if _, err := resolver.ResolveDevice(context.Background(), device); err == nil {
		t.Fatalf("%s must fail closed", why)
	}
}

func TestDefaultCollectionsFillMissingFields(t *testing.T) {
	// A service that overrides only its database name must still read the
	// standard collections rather than empty strings.
	collections := Collections{Database: "Custom"}.withDefaults()
	want := DefaultCollections()
	want.Database = "Custom"
	if collections != want {
		t.Fatalf("withDefaults() = %+v, want %+v", collections, want)
	}
}

func TestResolveDeviceUsesCanonicalSnapshotWithoutLegacyQueries(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	organisationId := primitive.NewObjectID()
	projectId := primitive.NewObjectID()

	assertOwnership(t, resolver, models.Device{
		Key:            "device-1",
		OrganisationId: organisationId.Hex(),
		ProjectId:      &projectId,
		UserId:         primitive.NewObjectID().Hex(),
	}, organisationId, projectId)

	if len(mock.FindOneCalls) != 0 || len(mock.FindCalls) != 0 {
		t.Fatal("canonical device ownership must not consult mutable legacy records")
	}
}

func TestResolveDeviceDefaultsProjectToOrganisationWithoutQueries(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	organisationId := primitive.NewObjectID()

	assertOwnership(t, resolver, models.Device{
		Key:            "device-1",
		OrganisationId: organisationId.Hex(),
	}, organisationId, organisationId)

	// The hidden default is a definition, not a lookup. Consulting a project
	// record here would let two services disagree for the same organisation.
	if len(mock.FindOneCalls) != 0 || len(mock.FindCalls) != 0 {
		t.Fatal("default project resolution must not query persisted records")
	}
}

func TestResolveDeviceIgnoresZeroStoredProject(t *testing.T) {
	resolver, _ := testResolver(Collections{})
	organisationId := primitive.NewObjectID()
	zero := primitive.NilObjectID

	// A stored zero means "unassigned", not "assigned to nothing"; stamping it
	// would hide every derived resource from project-scoped reads.
	assertOwnership(t, resolver, models.Device{
		Key:            "device-1",
		OrganisationId: organisationId.Hex(),
		ProjectId:      &zero,
	}, organisationId, organisationId)
}

func TestResolveDeviceTreatsLegacyOrganisationIdAsCanonicalCandidate(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	organisationId := primitive.NewObjectID()
	mock.QueueFindOne(map[string]any{"_id": organisationId}, nil)

	assertOwnership(t, resolver, models.Device{
		Key:    "device-1",
		UserId: organisationId.Hex(),
	}, organisationId, organisationId)
}

func TestResolveDeviceResolvesLegacySubUserThroughMaster(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	userId := primitive.NewObjectID()
	masterId := primitive.NewObjectID()
	mock.QueueFindOne(nil, mongo.ErrNoDocuments)
	mock.QueueFindOne(map[string]any{"_id": userId, "user_id": masterId.Hex()}, nil)
	mock.QueueFindOne(map[string]any{"_id": masterId, "ownerId": masterId}, nil)

	// A sub-user's own id is never a tenant identity; ownership follows the
	// stable master relationship, not the actor's mutable active selection.
	assertOwnership(t, resolver, models.Device{
		Key:    "device-1",
		UserId: userId.Hex(),
	}, masterId, masterId)
}

func TestResolveDeviceResolvesLegacyOwnerToSingleSecondaryOrganisation(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	ownerId := primitive.NewObjectID()
	organisationId := primitive.NewObjectID()
	mock.QueueFindOne(nil, mongo.ErrNoDocuments)
	mock.QueueFindOne(map[string]any{"_id": ownerId}, nil)
	mock.QueueFind([]persistedOrganisationOwnership{{Id: organisationId, OwnerId: ownerId}}, nil)

	// A secondary organisation has an independent id, so the project default
	// follows the organisation rather than the owning user.
	assertOwnership(t, resolver, models.Device{
		Key:    "device-1",
		UserId: ownerId.Hex(),
	}, organisationId, organisationId)
}

func TestResolveDeviceDerivesPrimaryOrganisationBeforeBootstrap(t *testing.T) {
	// An instance that has never run organisations-bootstrap has an empty
	// organisation collection. A primary organisation reuses the master user's
	// id, so deriving it produces the value the migration will later persist.
	// Failing here would drop every message on an un-migrated deployment.
	resolver, mock := testResolver(Collections{})
	masterId := primitive.NewObjectID()
	mock.QueueFindOne(nil, mongo.ErrNoDocuments)
	mock.QueueFindOne(map[string]any{"_id": masterId}, nil)
	mock.QueueFind([]persistedOrganisationOwnership{}, nil)

	assertOwnership(t, resolver, models.Device{
		Key:    "device-1",
		UserId: masterId.Hex(),
	}, masterId, masterId)
}

func TestResolveDeviceDerivesSubUserMasterOrganisationBeforeBootstrap(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	userId := primitive.NewObjectID()
	masterId := primitive.NewObjectID()
	mock.QueueFindOne(nil, mongo.ErrNoDocuments)
	mock.QueueFindOne(map[string]any{"_id": userId, "user_id": masterId.Hex()}, nil)
	mock.QueueFindOne(nil, mongo.ErrNoDocuments)
	mock.QueueFindOne(map[string]any{"_id": masterId}, nil)
	mock.QueueFind([]persistedOrganisationOwnership{}, nil)

	assertOwnership(t, resolver, models.Device{
		Key:    "device-1",
		UserId: userId.Hex(),
	}, masterId, masterId)
}

func TestResolveDeviceRejectsDanglingMasterBeforeBootstrap(t *testing.T) {
	// The sub-user is real but its user_id points at a master that no longer
	// exists. Bootstrap mints one organisation per master user, so deriving an
	// id here would stamp media with a tenant the migration never creates —
	// invisible to every reader rather than merely wrong.
	resolver, mock := testResolver(Collections{})
	userId := primitive.NewObjectID()
	masterId := primitive.NewObjectID()
	mock.QueueFindOne(nil, mongo.ErrNoDocuments)
	mock.QueueFindOne(map[string]any{"_id": userId, "user_id": masterId.Hex()}, nil)
	mock.QueueFindOne(nil, mongo.ErrNoDocuments)
	mock.QueueFindOne(nil, mongo.ErrNoDocuments)
	mock.QueueFind([]persistedOrganisationOwnership{}, nil)

	assertFailsClosed(t, resolver, models.Device{
		Key:    "device-1",
		UserId: userId.Hex(),
	}, "a legacy sub-user whose master account no longer exists")
}

func TestResolveDeviceDoesNotRecheckDirectOwnerBeforeBootstrap(t *testing.T) {
	// The owner is the queried user itself, so its existence is already proven.
	// A second lookup would be a wasted round trip on every un-migrated message.
	resolver, mock := testResolver(Collections{})
	masterId := primitive.NewObjectID()
	mock.QueueFindOne(nil, mongo.ErrNoDocuments)
	mock.QueueFindOne(map[string]any{"_id": masterId}, nil)
	mock.QueueFind([]persistedOrganisationOwnership{}, nil)

	assertOwnership(t, resolver, models.Device{
		Key:    "device-1",
		UserId: masterId.Hex(),
	}, masterId, masterId)

	if len(mock.FindOneCalls) != 2 {
		t.Fatalf("FindOne calls = %d, want 2", len(mock.FindOneCalls))
	}
}

func TestResolveDeviceFailsOnAmbiguousLegacyOwner(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	ownerId := primitive.NewObjectID()
	mock.QueueFindOne(nil, mongo.ErrNoDocuments)
	mock.QueueFindOne(map[string]any{"_id": ownerId}, nil)
	mock.QueueFind([]persistedOrganisationOwnership{
		{Id: primitive.NewObjectID(), OwnerId: ownerId},
		{Id: primitive.NewObjectID(), OwnerId: ownerId},
	}, nil)

	assertFailsClosed(t, resolver, models.Device{Key: "device-1", UserId: ownerId.Hex()}, "ambiguous legacy ownership")
}

func TestResolveDeviceRejectsOrphanedLegacyOwner(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	mock.QueueFindOne(nil, mongo.ErrNoDocuments)
	mock.QueueFindOne(nil, mongo.ErrNoDocuments)
	mock.QueueFind([]persistedOrganisationOwnership{}, nil)

	assertFailsClosed(t, resolver, models.Device{
		Key:    "device-1",
		UserId: primitive.NewObjectID().Hex(),
	}, "an owner with neither a user nor an organisation record")
}

func TestResolveDeviceRejectsInvalidMasterRelationship(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	userId := primitive.NewObjectID()
	mock.QueueFindOne(nil, mongo.ErrNoDocuments)
	mock.QueueFindOne(map[string]any{"_id": userId, "user_id": "invalid"}, nil)

	assertFailsClosed(t, resolver, models.Device{Key: "device-1", UserId: userId.Hex()}, "invalid master ownership")
}

// The organisation lookup must stay a plain canonical equality. An ownership
// $or here would be unreachable — organisations never stored a snake_case owner
// — and would cost the ownerId_1 index, because an $or is only index-served
// when every arm is index-bounded and no owner_id index exists.
func TestResolveDeviceLooksUpOrganisationOwnerByCanonicalFieldOnly(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	ownerId := primitive.NewObjectID()
	organisationId := primitive.NewObjectID()
	mock.QueueFindOne(nil, mongo.ErrNoDocuments)
	mock.QueueFindOne(map[string]any{"_id": ownerId}, nil)
	mock.QueueFind([]persistedOrganisationOwnership{{Id: organisationId, OwnerId: ownerId}}, nil)

	ownership, err := resolver.ResolveDevice(context.Background(), models.Device{Key: "device-1", UserId: ownerId.Hex()})
	if err != nil {
		t.Fatalf("resolve device: %v", err)
	}
	if ownership.OrganisationId != organisationId {
		t.Fatalf("organisation = %s, want %s", ownership.OrganisationId.Hex(), organisationId.Hex())
	}

	if len(mock.FindCalls) != 1 {
		t.Fatalf("Find calls = %d, want 1", len(mock.FindCalls))
	}
	want := map[string]any{properties.OrganisationOwnerId: ownerId}
	if !reflect.DeepEqual(mock.FindCalls[0].Filter, want) {
		t.Fatalf("owner filter = %#v, want %#v", mock.FindCalls[0].Filter, want)
	}
}

func TestResolveDeviceRejectsMissingOrInvalidOwnership(t *testing.T) {
	resolver, _ := testResolver(Collections{})

	for _, device := range []models.Device{
		{Key: "missing"},
		{Key: "invalid-organisation", OrganisationId: "invalid", UserId: primitive.NewObjectID().Hex()},
		{Key: "invalid-user", UserId: "invalid"},
	} {
		assertFailsClosed(t, resolver, device, "device "+device.Key)
	}
}

func TestResolveDeviceRequiresDatabaseForLegacyOwnership(t *testing.T) {
	resolver := NewDeviceResolver(nil, Collections{})

	assertFailsClosed(t, resolver, models.Device{
		Key:    "device-1",
		UserId: primitive.NewObjectID().Hex(),
	}, "legacy ownership without a database")
}

func TestLoadDeviceReadsConfiguredCollection(t *testing.T) {
	resolver, mock := testResolver(Collections{Database: "Custom", Devices: "sources"})
	deviceId := primitive.NewObjectID()
	organisationId := primitive.NewObjectID()
	mock.QueueFind([]models.Device{{
		Id:             deviceId,
		Key:            "device-1",
		OrganisationId: organisationId.Hex(),
	}}, nil)

	device, err := resolver.LoadDevice(context.Background(), "device-1")
	if err != nil {
		t.Fatalf("LoadDevice() error = %v", err)
	}
	if device.Id != deviceId {
		t.Fatalf("device id = %s, want %s", device.Id.Hex(), deviceId.Hex())
	}
	if len(mock.FindCalls) != 1 {
		t.Fatalf("Find calls = %d, want 1", len(mock.FindCalls))
	}
	if mock.FindCalls[0].Db != "Custom" || mock.FindCalls[0].Collection != "sources" {
		t.Fatalf("read %s/%s, want Custom/sources", mock.FindCalls[0].Db, mock.FindCalls[0].Collection)
	}
	var findOptions *options.FindOptions
	for _, option := range mock.FindCalls[0].Opts {
		if typed, ok := option.(*options.FindOptions); ok {
			findOptions = typed
		}
	}
	if findOptions == nil || findOptions.Limit == nil || *findOptions.Limit != 2 {
		t.Fatalf("find options = %#v, want limit 2", mock.FindCalls[0].Opts)
	}
}

func TestLoadDeviceRejectsDocumentWithoutIdentity(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	mock.QueueFind([]models.Device{{Key: "device-1"}}, nil)

	if _, err := resolver.LoadDevice(context.Background(), "device-1"); err == nil {
		t.Fatal("a device document with no persisted identity must fail closed")
	}
}

func TestLoadDeviceRejectsMissingDevice(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	mock.QueueFind([]models.Device{}, nil)

	if _, err := resolver.LoadDevice(context.Background(), "device-1"); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("LoadDevice() error = %v, want mongo.ErrNoDocuments", err)
	}
}

func TestLoadDeviceRejectsAmbiguousKey(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	mock.QueueFind([]models.Device{
		{Id: primitive.NewObjectID(), Key: "device-1"},
		{Id: primitive.NewObjectID(), Key: "device-1"},
	}, nil)

	if _, err := resolver.LoadDevice(context.Background(), "device-1"); err == nil {
		t.Fatal("a device key resolving to multiple documents must fail closed")
	}
}

func TestLoadScopedDeviceAllowsSameKeyInDifferentProjects(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	organisationId := primitive.NewObjectID()
	projectId := primitive.NewObjectID()
	deviceId := primitive.NewObjectID()
	mock.QueueFind([]models.Device{{
		Id:             deviceId,
		Key:            "device-1",
		OrganisationId: organisationId.Hex(),
		ProjectId:      &projectId,
	}}, nil)

	device, err := resolver.LoadScopedDevice(context.Background(), DeviceLookup{
		DeviceKey:      "device-1",
		OrganisationId: organisationId,
		ProjectId:      projectId,
	})
	if err != nil {
		t.Fatalf("LoadScopedDevice() error = %v", err)
	}
	if device.Id != deviceId {
		t.Fatalf("device id = %s, want %s", device.Id.Hex(), deviceId.Hex())
	}

	filter := mock.FindCalls[0].Filter.(bson.M)
	for _, arm := range filter["$or"].([]bson.M) {
		if arm[properties.DeviceKey] != "device-1" || arm[properties.DeviceProjectId] != projectId {
			t.Fatalf("project-scoped arm = %#v", arm)
		}
	}
}

func TestLoadScopedDeviceRequiresTrustedScope(t *testing.T) {
	resolver, _ := testResolver(Collections{})

	_, err := resolver.LoadScopedDevice(context.Background(), DeviceLookup{DeviceKey: "device-1"})
	if !errors.Is(err, ErrDeviceScopeRequired) {
		t.Fatalf("LoadScopedDevice() error = %v, want ErrDeviceScopeRequired", err)
	}
}

func TestLoadScopedDeviceScopesCanonicalAndLegacyOrganisationOwnership(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	organisationId := primitive.NewObjectID()
	legacyOwnerId := primitive.NewObjectID()
	deviceId := primitive.NewObjectID()
	mock.QueueFind([]models.Device{{Id: deviceId, Key: "device-1"}}, nil)

	_, err := resolver.LoadScopedDevice(context.Background(), DeviceLookup{
		DeviceKey:      "device-1",
		OrganisationId: organisationId,
		LegacyOwnerId:  legacyOwnerId,
	})
	if err != nil {
		t.Fatalf("LoadScopedDevice() error = %v", err)
	}

	want := bson.M{"$or": []bson.M{
		{properties.DeviceOrganisationId: organisationId.Hex(), properties.DeviceKey: "device-1"},
		{
			properties.DeviceOrganisationId: bson.M{"$in": bson.A{nil, ""}},
			properties.DeviceUserId:         legacyOwnerId.Hex(),
			properties.DeviceKey:            "device-1",
		},
	}}
	if !reflect.DeepEqual(mock.FindCalls[0].Filter, want) {
		t.Fatalf("filter = %#v, want %#v", mock.FindCalls[0].Filter, want)
	}
}

func TestLoadScopedDeviceRejectsDuplicateInsideTrustedProject(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	organisationId := primitive.NewObjectID()
	projectId := primitive.NewObjectID()
	mock.QueueFind([]models.Device{
		{Id: primitive.NewObjectID(), Key: "device-1"},
		{Id: primitive.NewObjectID(), Key: "device-1"},
	}, nil)

	_, err := resolver.LoadScopedDevice(context.Background(), DeviceLookup{
		DeviceKey:      "device-1",
		OrganisationId: organisationId,
		ProjectId:      projectId,
	})
	if err == nil {
		t.Fatal("duplicate devices inside one trusted project must fail closed")
	}
}

func TestLoadScopedDeviceRejectsProjectWithoutOrganisation(t *testing.T) {
	resolver, _ := testResolver(Collections{})

	_, err := resolver.LoadScopedDevice(context.Background(), DeviceLookup{
		DeviceKey: "device-1",
		ProjectId: primitive.NewObjectID(),
	})
	if !errors.Is(err, ErrDeviceScopeRequired) {
		t.Fatalf("LoadScopedDevice() error = %v, want ErrDeviceScopeRequired", err)
	}
}

func TestLoadScopedDeviceUsesOwnerOnlyForCanonicalMissingDevices(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	ownerId := primitive.NewObjectID()
	mock.QueueFind([]models.Device{{Id: primitive.NewObjectID(), Key: "device-1"}}, nil)

	_, err := resolver.LoadScopedDevice(context.Background(), DeviceLookup{
		DeviceKey:     "device-1",
		LegacyOwnerId: ownerId,
	})
	if err != nil {
		t.Fatalf("LoadScopedDevice() error = %v", err)
	}
	want := bson.M{
		properties.DeviceKey:            "device-1",
		properties.DeviceUserId:         ownerId.Hex(),
		properties.DeviceOrganisationId: bson.M{"$in": bson.A{nil, ""}},
	}
	if !reflect.DeepEqual(mock.FindCalls[0].Filter, want) {
		t.Fatalf("filter = %#v, want %#v", mock.FindCalls[0].Filter, want)
	}
}

func TestLoadScopedDeviceUsesIdBeforeOwnershipScope(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	deviceId := primitive.NewObjectID()
	mock.QueueFind([]models.Device{{Id: deviceId, Key: "device-1"}}, nil)

	_, err := resolver.LoadScopedDevice(context.Background(), DeviceLookup{
		DeviceId:       deviceId,
		DeviceKey:      "device-1",
		OrganisationId: primitive.NewObjectID(),
		ProjectId:      primitive.NewObjectID(),
	})
	if err != nil {
		t.Fatalf("LoadScopedDevice() error = %v", err)
	}
	want := bson.M{properties.DeviceId: deviceId, properties.DeviceKey: "device-1"}
	if !reflect.DeepEqual(mock.FindCalls[0].Filter, want) {
		t.Fatalf("filter = %#v, want %#v", mock.FindCalls[0].Filter, want)
	}
}

func TestResolveDeviceByKeyReturnsDeviceAndOwnership(t *testing.T) {
	resolver, mock := testResolver(Collections{})
	deviceId := primitive.NewObjectID()
	organisationId := primitive.NewObjectID()
	mock.QueueFind([]models.Device{{
		Id:             deviceId,
		Key:            "device-1",
		OrganisationId: organisationId.Hex(),
	}}, nil)

	device, ownership, err := resolver.ResolveDeviceByKey(context.Background(), "device-1")
	if err != nil {
		t.Fatalf("ResolveDeviceByKey() error = %v", err)
	}
	if device.Id != deviceId {
		t.Fatalf("device id = %s, want %s", device.Id.Hex(), deviceId.Hex())
	}
	if ownership.OrganisationId != organisationId || ownership.ProjectId != organisationId {
		t.Fatalf("ownership = %+v, want organisation and project %s", ownership, organisationId.Hex())
	}
}
