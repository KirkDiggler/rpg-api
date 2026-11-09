package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	goredis "github.com/redis/go-redis/v9"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// sessionEncVerKeyPrefix keys an encounter's checkpoint version: an integer
// the store increments on every encounter save that lands, absent while no
// save has (rpg-project#879 S12a). It lives beside the blob, never inside
// it, so the fenced save can compare it without decoding the world.
const sessionEncVerKeyPrefix = "session-enc-ver:v1alpha1:"

// The Redis encounter repository carries the session's optional fenced
// capability; the SDK finds it by type assertion on Config.Encounters.
var _ sdk.FencedEncounterRepository = (*redisEncounterRepository)(nil)

// THE VERSION SHARES THE BLOB'S LIFETIME. Every script that writes the blob
// writes the version in the same call with the same TTL, so the two expire
// together and nothing deletes one without the other (no code deletes the
// blob today). Belt and braces: a version found with no blob beside it is
// treated as no checkpoint (0), so a lingering version key can never refuse
// a fresh Launch of the same id.
//
// NOTHING IS WRITTEN UNTIL EVERYTHING IS CHECKED. A Lua script is atomic
// against other clients but does not roll back a write when a later call in
// it errors, so the fence, the sheets' existence and their owners are all
// checked before the first SET.

// saveFencedScript lands one fenced save.
//
//	KEYS: encounter, version, [session], sheet..., seat...
//	ARGV: expected, ttlMillis, hasSession, nSheets, nSeats, encounterBlob,
//	      [sessionBlob], sheetBlob..., sheetPlayer..., seatBlob...
//
// Replies {"ok", newVersion}, {"stale", storedVersion}, {"missing", key} or
// {"bad", reason}.
var saveFencedScript = goredis.NewScript(`
local expected = ARGV[1]
local ttl = tonumber(ARGV[2])
local hasSession = ARGV[3] == '1'
local nSheets = tonumber(ARGV[4])
local nSeats = tonumber(ARGV[5])

local cur = '0'
if redis.call('EXISTS', KEYS[1]) == 1 then
  local v = redis.call('GET', KEYS[2])
  if not v then
    return {'bad', 'encounter stored without a checkpoint version'}
  end
  cur = v
end
if cur ~= expected then
  return {'stale', cur}
end

local k = 3
local a = 7
if hasSession then
  k = 4
  a = 8
end

for i = 0, nSheets - 1 do
  local key = KEYS[k + i]
  local existing = redis.call('GET', key)
  if not existing then
    return {'missing', key}
  end
  local ok, decoded = pcall(cjson.decode, existing)
  if not ok or type(decoded) ~= 'table' or type(decoded.data) ~= 'table' then
    return {'bad', 'undecodable stored sheet ' .. key}
  end
  local owner = decoded.data.player_id
  if type(owner) ~= 'string' then
    owner = ''
  end
  if owner ~= ARGV[a + nSheets + i] then
    return {'bad', 'sheet ' .. key .. ' would change player; the fenced save never moves a sheet'}
  end
end

local function setWithTTL(key, value)
  if ttl > 0 then
    redis.call('SET', key, value, 'PX', ttl)
  else
    redis.call('SET', key, value)
  end
end

setWithTTL(KEYS[1], ARGV[6])
if hasSession then
  setWithTTL(KEYS[3], ARGV[7])
end
for i = 0, nSheets - 1 do
  redis.call('SET', KEYS[k + i], ARGV[a + i])
end
for j = 0, nSeats - 1 do
  redis.call('SET', KEYS[k + nSheets + j], ARGV[a + 2 * nSheets + j])
end

if cur == '0' then
  redis.call('DEL', KEYS[2])
end
local nv = redis.call('INCR', KEYS[2])
if ttl > 0 then
  redis.call('PEXPIRE', KEYS[2], ttl)
else
  redis.call('PERSIST', KEYS[2])
end
return {'ok', nv}
`)

// savePlainScript is SaveEncounter: the blob written unconditionally, and the
// version advanced with it, because a plain write is a save that landed and a
// fenced writer that loaded before it must see the checkpoint move.
//
//	KEYS: encounter, version
//	ARGV: encounterBlob, ttlMillis
var savePlainScript = goredis.NewScript(`
local ttl = tonumber(ARGV[2])
local had = redis.call('EXISTS', KEYS[1]) == 1
if ttl > 0 then
  redis.call('SET', KEYS[1], ARGV[1], 'PX', ttl)
else
  redis.call('SET', KEYS[1], ARGV[1])
end
if not had then
  redis.call('DEL', KEYS[2])
end
redis.call('INCR', KEYS[2])
if ttl > 0 then
  redis.call('PEXPIRE', KEYS[2], ttl)
else
  redis.call('PERSIST', KEYS[2])
end
return 1
`)

// GetFencedEncounter implements sdk.FencedEncounterRepository: the blob and
// its version in one MGET, so the version describes exactly the data
// returned. A blob with no version beside it fails closed.
func (r *redisEncounterRepository) GetFencedEncounter(
	ctx context.Context, in *sdk.GetFencedEncounterInput,
) (*sdk.GetFencedEncounterOutput, error) {
	if in == nil || in.Encounter == "" {
		return nil, errors.New("session: GetFencedEncounter encounter is required")
	}
	id := in.Encounter
	vals, err := r.client.MGet(ctx, sessionEncKeyPrefix+id, sessionEncVerKeyPrefix+id).Result()
	if err != nil {
		return nil, fmt.Errorf("get fenced encounter %q: %w", id, err)
	}
	if len(vals) != 2 {
		return nil, fmt.Errorf("get fenced encounter %q: MGET returned %d values: %w", id, len(vals), sdk.ErrBadRepository)
	}
	blob, ok := vals[0].(string)
	if !ok {
		return nil, fmt.Errorf("encounter %q: %w", id, sdk.ErrNotFound)
	}
	rawVersion, ok := vals[1].(string)
	if !ok {
		return nil, fmt.Errorf("encounter %q is stored without a checkpoint version: %w", id, sdk.ErrBadRepository)
	}
	version, err := strconv.ParseUint(rawVersion, 10, 64)
	if err != nil || version == 0 {
		return nil, fmt.Errorf("encounter %q has checkpoint version %q: %w", id, rawVersion, sdk.ErrBadRepository)
	}
	var data tkencounter.EncounterData
	if err := json.Unmarshal([]byte(blob), &data); err != nil {
		return nil, fmt.Errorf("unmarshal stored encounter %q: %w", id, err)
	}
	return &sdk.GetFencedEncounterOutput{Data: &data, Version: version}, nil
}

// SaveFenced implements sdk.FencedEncounterRepository: the world, the session
// record when given, the touched sheets and the written seats, under the keys
// their own repositories use, in one script fenced on in.Expected.
//
// The session record takes this repository's TTL; the orchestrator builds the
// session and encounter repositories with the same one (orchestrator.go).
// Sheets and seats carry no TTL, as their repositories write them.
func (r *redisEncounterRepository) SaveFenced(ctx context.Context, in *sdk.SaveFencedInput) (*sdk.SaveFencedOutput, error) {
	if in == nil || in.Encounter == "" {
		return nil, errors.New("session: SaveFenced encounter is required")
	}
	if in.Data == nil {
		return nil, errors.New("session: SaveFenced data is required")
	}
	id := in.Encounter

	keys := []string{sessionEncKeyPrefix + id, sessionEncVerKeyPrefix + id}
	encBlob, err := json.Marshal(in.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal encounter %q: %w", id, err)
	}
	args := []interface{}{
		strconv.FormatUint(in.Expected, 10), r.ttl.Milliseconds(), "0",
		len(in.Characters), len(in.Seats), encBlob,
	}
	if in.Session != nil {
		if in.Session.ID == "" {
			return nil, errors.New("session: SaveFenced session ID is required")
		}
		b, marshalErr := json.Marshal(in.Session)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal session %q: %w", in.Session.ID, marshalErr)
		}
		args[2] = "1"
		keys = append(keys, sessionKeyPrefix+in.Session.ID)
		args = append(args, b)
	}

	owners := make([]interface{}, 0, len(in.Characters))
	for _, sheet := range in.Characters {
		if sheet == nil || sheet.ID == "" {
			return nil, errors.New("session: SaveFenced character data.ID is required")
		}
		// The character repository's stored shape: entities.Character wraps
		// the SDK's sheet (internal/entities/character.go).
		b, marshalErr := json.Marshal(&entities.Character{Data: sheet})
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal character %q: %w", sheet.ID, marshalErr)
		}
		keys = append(keys, characterrepo.Key(sheet.ID))
		args = append(args, b)
		owners = append(owners, sheet.PlayerID)
	}
	args = append(args, owners...)
	for _, seat := range in.Seats {
		if seat == nil || seat.Character == "" {
			return nil, errors.New("session: SaveFenced seat Character is required")
		}
		b, marshalErr := json.Marshal(seat)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal seat %q: %w", seat.Character, marshalErr)
		}
		keys = append(keys, seatKeyPrefix+seat.Character)
		args = append(args, b)
	}

	reply, err := saveFencedScript.Run(ctx, r.client, keys, args...).Slice()
	if err != nil {
		return nil, fmt.Errorf("fenced save of encounter %q: %w", id, err)
	}
	if len(reply) != 2 {
		return nil, fmt.Errorf("fenced save of encounter %q: reply %v: %w", id, reply, sdk.ErrBadRepository)
	}
	marker, _ := reply[0].(string)
	switch marker {
	case "ok":
		version, ok := reply[1].(int64)
		if !ok || version <= 0 {
			return nil, fmt.Errorf("fenced save of encounter %q: version %v: %w", id, reply[1], sdk.ErrBadRepository)
		}
		return &sdk.SaveFencedOutput{Version: uint64(version)}, nil
	case "stale":
		return nil, fmt.Errorf("encounter %q: store is at checkpoint %v, save loaded %d: %w",
			id, reply[1], in.Expected, sdk.ErrStaleCheckpoint)
	case "missing":
		return nil, fmt.Errorf("fenced save of encounter %q: %v: %w", id, reply[1], sdk.ErrNotFound)
	default:
		return nil, fmt.Errorf("fenced save of encounter %q: %v: %w", id, reply[1], sdk.ErrBadRepository)
	}
}
