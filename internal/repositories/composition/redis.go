package composition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	goredis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

const (
	compositionKeyPrefix = "composition:"

	sourceVersion = 1
	maxItems      = 200
	maxGroups     = 80
	maxNameLength = 120
	maxIDLength   = 120
	maxRefLength  = 160
	worldLimit    = 12.0
	maxHeight     = 8.0
	maxYaw        = math.Pi * 100

	itemKind  = "prop"
	groupKind = "group"
)

const createScriptOK int64 = 0

const (
	createScriptDefinitionCollision int64 = 1
	createScriptRevisionCollision   int64 = 2
	createScriptCorruptIndex        int64 = 3
)

const appendScriptOK int64 = 0

const (
	appendScriptDefinitionMissing   int64 = 1
	appendScriptCorruptDefinition   int64 = 2
	appendScriptStaleHead           int64 = 3
	appendScriptRevisionCollision   int64 = 4
	appendScriptCorruptIndex        int64 = 5
	appendScriptCorruptHeadRevision int64 = 6
)

var createDefinitionScript = goredis.NewScript(`
local definition_type = redis.call('TYPE', KEYS[1]).ok
if definition_type ~= 'none' then
  return 1
end
local revision_type = redis.call('TYPE', KEYS[2]).ok
if revision_type ~= 'none' then
  return 2
end
local index_type = redis.call('TYPE', KEYS[3]).ok
if index_type ~= 'none' and index_type ~= 'set' then
  return 3
end
redis.call('SET', KEYS[1], ARGV[2])
redis.call('SET', KEYS[2], ARGV[3])
redis.call('SADD', KEYS[3], ARGV[1])
return 0
`)

var appendRevisionScript = goredis.NewScript(`
local definition_type = redis.call('TYPE', KEYS[1]).ok
if definition_type == 'none' then
  return 1
end
if definition_type ~= 'string' then
  return 2
end

local revision_type = redis.call('TYPE', KEYS[2]).ok
if revision_type ~= 'none' then
  return 4
end

local index_type = redis.call('TYPE', KEYS[3]).ok
if index_type ~= 'set' or redis.call('SISMEMBER', KEYS[3], ARGV[2]) ~= 1 then
  return 5
end

local stored_definition = redis.call('GET', KEYS[1])
local definition_ok, definition = pcall(cjson.decode, stored_definition)
if not definition_ok or type(definition) ~= 'table' then
  return 2
end
if definition['guild_id'] ~= ARGV[1] or definition['id'] ~= ARGV[2] or type(definition['head_revision_id']) ~= 'string' then
  return 2
end
if definition['head_revision_id'] ~= ARGV[3] then
  return 3
end
if stored_definition ~= ARGV[6] then
  return 2
end

local head_type = redis.call('TYPE', KEYS[4]).ok
if head_type ~= 'string' then
  return 6
end
local stored_head = redis.call('GET', KEYS[4])
local head_ok, head = pcall(cjson.decode, stored_head)
if not head_ok or type(head) ~= 'table' then
  return 6
end
if head['guild_id'] ~= ARGV[1] or head['definition_id'] ~= ARGV[2] or head['id'] ~= ARGV[3] then
  return 6
end
if stored_head ~= ARGV[7] then
  return 6
end

redis.call('SET', KEYS[2], ARGV[5])
redis.call('SET', KEYS[1], ARGV[4])
return 0
`)

type redisRepository struct {
	client redisclient.Client
	clock  clock.Clock
	idGen  idgen.Generator
}

// RedisConfig contains all required Redis repository dependencies.
type RedisConfig struct {
	Client      redisclient.Client
	Clock       clock.Clock
	IDGenerator idgen.Generator
}

// Validate validates the Redis repository configuration.
func (cfg *RedisConfig) Validate() error {
	if cfg == nil {
		return apierr.InvalidArgument("config cannot be nil")
	}
	if cfg.Client == nil {
		return apierr.InvalidArgument("redis client is required")
	}
	if cfg.Clock == nil {
		return apierr.InvalidArgument("clock is required")
	}
	if cfg.IDGenerator == nil {
		return apierr.InvalidArgument("ID generator is required")
	}
	return nil
}

// NewRedis creates a Redis-backed composition repository.
func NewRedis(cfg *RedisConfig) (Repository, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &redisRepository{client: cfg.Client, clock: cfg.Clock, idGen: cfg.IDGenerator}, nil
}

func (r *redisRepository) CreateDefinition(
	ctx context.Context,
	input CreateDefinitionInput,
) (*CreateDefinitionOutput, error) {
	if err := validateOwnerAndSource(input.GuildID, input.CreatedByPlayerID, input.Source); err != nil {
		return nil, err
	}

	definitionID, err := r.generateID("definition")
	if err != nil {
		return nil, err
	}
	revisionID, err := r.generateID("revision")
	if err != nil {
		return nil, err
	}
	now := r.clock.Now()
	definition := &entities.CompositionDefinition{
		ID: definitionID, GuildID: input.GuildID,
		CreatedByPlayerID: input.CreatedByPlayerID, CreatedAt: now,
		HeadRevisionID: revisionID,
	}
	revision := &entities.CompositionRevision{
		ID: revisionID, DefinitionID: definitionID, GuildID: input.GuildID,
		CreatedByPlayerID: input.CreatedByPlayerID, CreatedAt: now, Source: input.Source,
	}
	definitionJSON, err := json.Marshal(definition)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to marshal composition definition")
	}
	revisionJSON, err := json.Marshal(revision)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to marshal composition revision")
	}

	status, err := createDefinitionScript.Run(ctx, r.client, []string{
		definitionKey(input.GuildID, definitionID),
		revisionKey(input.GuildID, revisionID),
		definitionIndexKey(input.GuildID),
	}, definitionID, definitionJSON, revisionJSON).Int64()
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to create composition definition")
	}
	switch status {
	case createScriptOK:
	case createScriptDefinitionCollision:
		return nil, apierr.AlreadyExistsf("composition definition ID %s already exists", definitionID)
	case createScriptRevisionCollision:
		return nil, apierr.AlreadyExistsf("composition revision ID %s already exists", revisionID)
	case createScriptCorruptIndex:
		return nil, apierr.Internal("composition definition index has an invalid Redis type")
	default:
		return nil, apierr.Internalf("composition create returned unknown status %d", status)
	}

	return decodeCreateOutput(definitionJSON, revisionJSON)
}

func (r *redisRepository) AppendRevision(
	ctx context.Context,
	input AppendRevisionInput,
) (*AppendRevisionOutput, error) {
	if err := validateIdentifier("guild ID", input.GuildID); err != nil {
		return nil, err
	}
	if err := validateIdentifier("definition ID", input.DefinitionID); err != nil {
		return nil, err
	}
	if err := validateIdentifier("expected head revision ID", input.ExpectedHeadRevisionID); err != nil {
		return nil, err
	}
	if err := validateIdentifier("creator player ID", input.CreatedByPlayerID); err != nil {
		return nil, err
	}
	if err := validateSource(input.Source); err != nil {
		return nil, err
	}

	revisionID, err := r.generateID("revision")
	if err != nil {
		return nil, err
	}
	now := r.clock.Now()
	definition := &entities.CompositionDefinition{
		ID: input.DefinitionID, GuildID: input.GuildID,
		// The script binds its writes to the exact definition/head bytes validated below.
		// Preserve the definition's immutable attribution and creation time in its new value.
		HeadRevisionID: revisionID,
	}

	existingJSON, err := r.client.Get(ctx, definitionKey(input.GuildID, input.DefinitionID)).Bytes()
	if err != nil {
		if err == goredis.Nil {
			return nil, apierr.NotFoundf("composition definition %s not found", input.DefinitionID)
		}
		return nil, apierr.Wrapf(err, "failed to read composition definition before append")
	}
	existing, err := decodeDefinition(existingJSON, input.GuildID, input.DefinitionID)
	if err != nil {
		return nil, err
	}
	if existing.HeadRevisionID != input.ExpectedHeadRevisionID {
		return nil, apierr.Aborted("composition head changed concurrently")
	}
	existingHeadJSON, err := r.client.Get(
		ctx,
		revisionKey(input.GuildID, input.ExpectedHeadRevisionID),
	).Bytes()
	if err != nil {
		return nil, apierr.WrapWithCodef(
			err,
			apierr.CodeInternal,
			"composition definition %s has a missing or unreadable head revision",
			input.DefinitionID,
		)
	}
	if _, err = decodeRevision(
		existingHeadJSON,
		input.GuildID,
		input.DefinitionID,
		input.ExpectedHeadRevisionID,
		false,
	); err != nil {
		return nil, apierr.WrapWithCode(err, apierr.CodeInternal, "composition head revision is corrupt")
	}
	definition.CreatedByPlayerID = existing.CreatedByPlayerID
	definition.CreatedAt = existing.CreatedAt

	revision := &entities.CompositionRevision{
		ID: revisionID, DefinitionID: input.DefinitionID, GuildID: input.GuildID,
		CreatedByPlayerID: input.CreatedByPlayerID, CreatedAt: now, Source: input.Source,
	}
	definitionJSON, err := json.Marshal(definition)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to marshal composition definition head")
	}
	revisionJSON, err := json.Marshal(revision)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to marshal composition revision")
	}

	status, err := appendRevisionScript.Run(ctx, r.client, []string{
		definitionKey(input.GuildID, input.DefinitionID),
		revisionKey(input.GuildID, revisionID),
		definitionIndexKey(input.GuildID),
		revisionKey(input.GuildID, input.ExpectedHeadRevisionID),
	}, input.GuildID, input.DefinitionID, input.ExpectedHeadRevisionID,
		definitionJSON, revisionJSON, existingJSON, existingHeadJSON).Int64()
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to append composition revision")
	}
	switch status {
	case appendScriptOK:
	case appendScriptDefinitionMissing:
		return nil, apierr.NotFoundf("composition definition %s not found", input.DefinitionID)
	case appendScriptCorruptDefinition:
		return nil, apierr.Internal("composition definition metadata is corrupt")
	case appendScriptStaleHead:
		return nil, apierr.Aborted("composition head changed concurrently")
	case appendScriptRevisionCollision:
		return nil, apierr.AlreadyExistsf("composition revision ID %s already exists", revisionID)
	case appendScriptCorruptIndex:
		return nil, apierr.Internal("composition definition index is corrupt")
	case appendScriptCorruptHeadRevision:
		return nil, apierr.Internal("composition head revision is missing or corrupt")
	default:
		return nil, apierr.Internalf("composition append returned unknown status %d", status)
	}

	return decodeAppendOutput(definitionJSON, revisionJSON)
}

func (r *redisRepository) GetRevision(
	ctx context.Context,
	input GetRevisionInput,
) (*GetRevisionOutput, error) {
	if err := validateIdentifier("guild ID", input.GuildID); err != nil {
		return nil, err
	}
	if err := validateIdentifier("definition ID", input.DefinitionID); err != nil {
		return nil, err
	}
	if err := validateIdentifier("revision ID", input.RevisionID); err != nil {
		return nil, err
	}

	stored, err := r.client.Get(ctx, revisionKey(input.GuildID, input.RevisionID)).Bytes()
	if err != nil {
		if err == goredis.Nil {
			return nil, apierr.NotFoundf("composition revision %s not found", input.RevisionID)
		}
		return nil, apierr.Wrapf(err, "failed to get composition revision")
	}
	revision, err := decodeRevision(stored, input.GuildID, input.DefinitionID, input.RevisionID, true)
	if err != nil {
		return nil, err
	}
	return &GetRevisionOutput{Revision: revision}, nil
}

func (r *redisRepository) ListDefinitions(
	ctx context.Context,
	input ListDefinitionsInput,
) (*ListDefinitionsOutput, error) {
	if err := validateIdentifier("guild ID", input.GuildID); err != nil {
		return nil, err
	}

	ids, err := r.client.SMembers(ctx, definitionIndexKey(input.GuildID)).Result()
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to list composition definition index")
	}
	sort.Strings(ids)
	output := &ListDefinitionsOutput{Definitions: make([]*CurrentDefinition, 0, len(ids))}
	for _, definitionID := range ids {
		storedDefinition, getErr := r.client.Get(ctx, definitionKey(input.GuildID, definitionID)).Bytes()
		if getErr != nil {
			return nil, apierr.WrapWithCodef(
				getErr, apierr.CodeInternal, "composition index points to missing definition %s", definitionID,
			)
		}
		definition, decodeErr := decodeDefinition(storedDefinition, input.GuildID, definitionID)
		if decodeErr != nil {
			return nil, decodeErr
		}
		storedRevision, getErr := r.client.Get(ctx, revisionKey(input.GuildID, definition.HeadRevisionID)).Bytes()
		if getErr != nil {
			return nil, apierr.WrapWithCodef(
				getErr, apierr.CodeInternal, "composition definition %s has a missing head", definitionID,
			)
		}
		revision, decodeErr := decodeRevision(
			storedRevision, input.GuildID, definitionID, definition.HeadRevisionID, false,
		)
		if decodeErr != nil {
			return nil, decodeErr
		}
		output.Definitions = append(output.Definitions, &CurrentDefinition{
			Definition: definition,
			Revision:   revision,
		})
	}
	return output, nil
}

func (r *redisRepository) generateID(kind string) (string, error) {
	id := r.idGen.Generate()
	if err := validateIdentifier(kind+" ID", id); err != nil {
		return "", apierr.Internalf("ID generator returned invalid %s ID", kind)
	}
	return id, nil
}

func validateOwnerAndSource(guildID, creatorID string, source entities.CompositionSource) error {
	if err := validateIdentifier("guild ID", guildID); err != nil {
		return err
	}
	if err := validateIdentifier("creator player ID", creatorID); err != nil {
		return err
	}
	return validateSource(source)
}

func validateSource(source entities.CompositionSource) error {
	if source.Version != sourceVersion {
		return apierr.InvalidArgumentf("composition source version must be %d", sourceVersion)
	}
	if err := validateBoundedText("composition name", source.Name, maxNameLength); err != nil {
		return err
	}
	if len(source.Items) == 0 {
		return apierr.InvalidArgument("composition source must contain at least one item")
	}
	if len(source.Items) > maxItems {
		return apierr.InvalidArgumentf("composition source cannot contain more than %d items", maxItems)
	}
	if len(source.Groups) > maxGroups {
		return apierr.InvalidArgumentf("composition source cannot contain more than %d groups", maxGroups)
	}

	allIDs := make(map[string]struct{}, len(source.Items)+len(source.Groups))
	itemIDs := make(map[string]struct{}, len(source.Items))
	groupIDs := make(map[string]struct{}, len(source.Groups))
	for index := range source.Items {
		item := source.Items[index]
		if item.Kind != itemKind {
			return apierr.InvalidArgumentf("items[%d].kind must be %q", index, itemKind)
		}
		if err := validateEntityID(fmt.Sprintf("items[%d].id", index), item.ID, allIDs); err != nil {
			return err
		}
		itemIDs[item.ID] = struct{}{}
		if err := validateAssetRef(fmt.Sprintf("items[%d].asset_ref", index), item.AssetRef); err != nil {
			return err
		}
		if err := validateBoundedText(fmt.Sprintf("items[%d].label", index), item.Label, maxNameLength); err != nil {
			return err
		}
		if err := validateTransform(fmt.Sprintf("items[%d].transform", index), item.Transform); err != nil {
			return err
		}
	}
	for index := range source.Groups {
		group := source.Groups[index]
		if group.Kind != groupKind {
			return apierr.InvalidArgumentf("groups[%d].kind must be %q", index, groupKind)
		}
		if err := validateEntityID(fmt.Sprintf("groups[%d].id", index), group.ID, allIDs); err != nil {
			return err
		}
		groupIDs[group.ID] = struct{}{}
		if err := validateBoundedText(fmt.Sprintf("groups[%d].label", index), group.Label, maxNameLength); err != nil {
			return err
		}
		if err := validateTransform(fmt.Sprintf("groups[%d].transform", index), group.Transform); err != nil {
			return err
		}
	}

	dependencies := make(map[string][]string, len(allIDs))
	for index := range source.Groups {
		group := source.Groups[index]
		if group.ParentID != "" {
			if _, ok := groupIDs[group.ParentID]; !ok {
				return apierr.InvalidArgumentf("groups[%d].parent_id must reference a group", index)
			}
			dependencies[group.ID] = append(dependencies[group.ID], group.ParentID)
		}
	}
	for index := range source.Items {
		item := source.Items[index]
		if item.ParentID != "" {
			if _, ok := groupIDs[item.ParentID]; !ok {
				return apierr.InvalidArgumentf("items[%d].parent_id must reference a group", index)
			}
			dependencies[item.ID] = append(dependencies[item.ID], item.ParentID)
		}
		if item.SupportID != "" {
			if _, ok := itemIDs[item.SupportID]; !ok {
				return apierr.InvalidArgumentf("items[%d].support_id must reference an item", index)
			}
			dependencies[item.ID] = append(dependencies[item.ID], item.SupportID)
		}
	}
	if hasRelationshipCycle(allIDs, dependencies) {
		return apierr.InvalidArgument("composition relationships must be acyclic")
	}
	return nil
}

func validateEntityID(field, id string, allIDs map[string]struct{}) error {
	if err := validateBoundedText(field, id, maxIDLength); err != nil {
		return err
	}
	if _, exists := allIDs[id]; exists {
		return apierr.InvalidArgumentf("composition contains duplicate identity %q", id)
	}
	allIDs[id] = struct{}{}
	return nil
}

func validateAssetRef(field, ref string) error {
	if err := validateBoundedText(field, ref, maxRefLength); err != nil {
		return err
	}
	parts := strings.Split(ref, ":")
	if len(parts) < 3 {
		return apierr.InvalidArgumentf("%s must contain at least module:type:tail", field)
	}
	for _, part := range parts {
		if part == "" || strings.TrimSpace(part) != part || strings.IndexFunc(part, unicode.IsControl) >= 0 {
			return apierr.InvalidArgumentf("%s contains an empty or invalid segment", field)
		}
	}
	return nil
}

func validateTransform(field string, transform entities.CompositionTransform) error {
	values := []struct {
		name    string
		value   float64
		minimum float64
		maximum float64
	}{
		{name: "x", value: transform.X, minimum: -worldLimit, maximum: worldLimit},
		{name: "y", value: transform.Y, minimum: 0, maximum: maxHeight},
		{name: "z", value: transform.Z, minimum: -worldLimit, maximum: worldLimit},
		{name: "rotation_y", value: transform.RotationY, minimum: -maxYaw, maximum: maxYaw},
	}
	for _, entry := range values {
		if math.IsNaN(entry.value) || math.IsInf(entry.value, 0) ||
			entry.value < entry.minimum || entry.value > entry.maximum {
			return apierr.InvalidArgumentf(
				"%s.%s must be finite and between %g and %g",
				field, entry.name, entry.minimum, entry.maximum,
			)
		}
	}
	return nil
}

func validateIdentifier(field, value string) error {
	return validateBoundedText(field, value, maxIDLength)
}

func validateBoundedText(field, value string, maximum int) error {
	if !isValidBoundedText(value, maximum) {
		return apierr.InvalidArgumentf("%s must be a non-empty string up to %d characters", field, maximum)
	}
	return nil
}

func isValidIdentifier(value string) bool {
	return isValidBoundedText(value, maxIDLength)
}

func isValidBoundedText(value string, maximum int) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= maximum &&
		strings.IndexFunc(value, unicode.IsControl) < 0
}

func hasRelationshipCycle(allIDs map[string]struct{}, dependencies map[string][]string) bool {
	const (
		unvisited uint8 = iota
		visiting
		visited
	)
	states := make(map[string]uint8, len(allIDs))
	var visit func(string) bool
	visit = func(id string) bool {
		switch states[id] {
		case visiting:
			return true
		case visited:
			return false
		}
		states[id] = visiting
		for _, dependency := range dependencies[id] {
			if visit(dependency) {
				return true
			}
		}
		states[id] = visited
		return false
	}
	for id := range allIDs {
		if visit(id) {
			return true
		}
	}
	return false
}

func decodeCreateOutput(definitionJSON, revisionJSON []byte) (*CreateDefinitionOutput, error) {
	definition, err := decodeDefinitionJSON(definitionJSON)
	if err != nil {
		return nil, err
	}
	revision, err := decodeRevisionJSON(revisionJSON)
	if err != nil {
		return nil, err
	}
	return &CreateDefinitionOutput{Definition: definition, Revision: revision}, nil
}

func decodeAppendOutput(definitionJSON, revisionJSON []byte) (*AppendRevisionOutput, error) {
	definition, err := decodeDefinitionJSON(definitionJSON)
	if err != nil {
		return nil, err
	}
	revision, err := decodeRevisionJSON(revisionJSON)
	if err != nil {
		return nil, err
	}
	return &AppendRevisionOutput{Definition: definition, Revision: revision}, nil
}

func decodeDefinition(stored []byte, guildID, definitionID string) (*entities.CompositionDefinition, error) {
	definition, err := decodeDefinitionJSON(stored)
	if err != nil {
		return nil, err
	}
	if !isValidIdentifier(definition.ID) || !isValidIdentifier(definition.GuildID) ||
		!isValidIdentifier(definition.CreatedByPlayerID) || !isValidIdentifier(definition.HeadRevisionID) ||
		definition.ID != definitionID || definition.GuildID != guildID || definition.CreatedAt.IsZero() {
		return nil, apierr.Internal("composition definition metadata is corrupt")
	}
	return definition, nil
}

func decodeRevision(
	stored []byte,
	guildID, definitionID, revisionID string,
	notFoundOnDefinitionMismatch bool,
) (*entities.CompositionRevision, error) {
	revision, err := decodeRevisionJSON(stored)
	if err != nil {
		return nil, err
	}
	if !isValidIdentifier(revision.ID) || !isValidIdentifier(revision.DefinitionID) ||
		!isValidIdentifier(revision.GuildID) || !isValidIdentifier(revision.CreatedByPlayerID) ||
		revision.ID != revisionID || revision.GuildID != guildID || revision.CreatedAt.IsZero() {
		return nil, apierr.Internal("composition revision metadata is corrupt")
	}
	if revision.DefinitionID != definitionID {
		if notFoundOnDefinitionMismatch {
			return nil, apierr.NotFoundf("composition revision %s not found in definition %s", revisionID, definitionID)
		}
		return nil, apierr.Internal("composition revision definition metadata is corrupt")
	}
	if validationErr := validateSource(revision.Source); validationErr != nil {
		return nil, apierr.WrapWithCode(validationErr, apierr.CodeInternal, "composition revision source is corrupt")
	}
	return revision, nil
}

func decodeDefinitionJSON(stored []byte) (*entities.CompositionDefinition, error) {
	var definition entities.CompositionDefinition
	if err := json.Unmarshal(stored, &definition); err != nil {
		return nil, apierr.Wrapf(err, "failed to unmarshal composition definition")
	}
	return &definition, nil
}

func decodeRevisionJSON(stored []byte) (*entities.CompositionRevision, error) {
	var revision entities.CompositionRevision
	if err := json.Unmarshal(stored, &revision); err != nil {
		return nil, apierr.Wrapf(err, "failed to unmarshal composition revision")
	}
	return &revision, nil
}

func definitionIndexKey(guildID string) string {
	return guildKeyPrefix(guildID) + "definitions"
}

func definitionKey(guildID, definitionID string) string {
	return guildKeyPrefix(guildID) + "definition:" + definitionID
}

func revisionKey(guildID, revisionID string) string {
	return guildKeyPrefix(guildID) + "revision:" + revisionID
}

func guildKeyPrefix(guildID string) string {
	digest := sha256.Sum256([]byte(guildID))
	return compositionKeyPrefix + "{" + hex.EncodeToString(digest[:]) + "}:"
}
