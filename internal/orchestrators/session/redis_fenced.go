package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	goredis "github.com/redis/go-redis/v9"

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
// capability and its appending refinement; the SDK finds both by type
// assertion on Config.Encounters.
var (
	_ sdk.FencedEncounterRepository    = (*redisEncounterRepository)(nil)
	_ sdk.AppendingEncounterRepository = (*redisEncounterRepository)(nil)
)

// THE VERSION SHARES THE CHECKPOINT'S LIFETIME. Every script that writes the
// rest writes the version and the record list in the same call with the same
// TTL, so the three expire together and nothing deletes one without the
// others (no code deletes the rest today). Belt and braces: a version found
// with no rest beside it is treated as no checkpoint (0), so a lingering
// version key can never refuse a fresh Launch of the same id; a lingering
// list is dropped by the first save, which always appends from seq 1.
//
// NOTHING IS WRITTEN UNTIL EVERYTHING IS CHECKED. A Lua script is atomic
// against other clients but does not roll back a write when a later call in
// it errors, so the fence, the record's first seq, the stored segments' last
// seqs, the sheets' existence and their owners are all checked before the
// first write.

// saveFencedScript lands one fenced save: the rest replaced, one record
// segment appended, the segments wholly below the floor dropped, the floor
// kept beside the rest.
//
//	KEYS: rest, version, log, floor, [session], sheet..., seat...
//	ARGV: expected, ttlMillis, hasSession, nSheets, nSeats, restBlob, since,
//	      hasSegment, segmentElem, floor, [sessionBlob], sheetBlob...,
//	      sheetPlayer..., seatBlob...
//
// since 1 starts the record over: the list is dropped before the append. A
// first save (no checkpoint yet) must start it over. Each list element is
// "<last seq>\n<segment JSON>", so the floor drop reads the prefix alone; a
// stored segment whose last seq equals the floor is kept (it holds the floor
// itself), and an element whose prefix does not parse refuses the save.
//
// Replies {"ok", newVersion}, {"stale", storedVersion}, {"missing", key} or
// {"bad", reason}.
var saveFencedScript = goredis.NewScript(`
local expected = ARGV[1]
local ttl = tonumber(ARGV[2])
local hasSession = ARGV[3] == '1'
local nSheets = tonumber(ARGV[4])
local nSeats = tonumber(ARGV[5])
local since = tonumber(ARGV[7])
local hasSegment = ARGV[8] == '1'
local floor = tonumber(ARGV[10])

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
if cur == '0' and since ~= 1 then
  return {'bad', 'a first checkpoint must append the record from seq 1, not ' .. ARGV[7]}
end

-- The stored segments the floor drops, counted before any write. A save that
-- starts the record over drops them all anyway.
local drop = 0
if since ~= 1 and floor > 0 then
  local n = redis.call('LLEN', KEYS[3])
  while drop < n do
    local elem = redis.call('LINDEX', KEYS[3], drop)
    local prefix = string.match(elem, '^(%d+)\n')
    local last = nil
    if prefix then
      last = tonumber(prefix)
    end
    if last == nil then
      return {'bad', 'record segment ' .. drop .. ' has no last-seq prefix'}
    end
    if last >= floor then
      break
    end
    drop = drop + 1
  end
end

local k = 5
local a = 11
if hasSession then
  k = 6
  a = 12
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
local function expireWithTTL(key)
  if ttl > 0 then
    redis.call('PEXPIRE', key, ttl)
  else
    redis.call('PERSIST', key)
  end
end

setWithTTL(KEYS[1], ARGV[6])
if since == 1 then
  redis.call('DEL', KEYS[3])
end
if drop > 0 then
  redis.call('LTRIM', KEYS[3], drop, -1)
end
if hasSegment then
  redis.call('RPUSH', KEYS[3], ARGV[9])
end
expireWithTTL(KEYS[3])
setWithTTL(KEYS[4], ARGV[10])
if hasSession then
  setWithTTL(KEYS[5], ARGV[11])
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
expireWithTTL(KEYS[2])
return {'ok', nv}
`)

// savePlainScript is SaveEncounter: the rest written unconditionally with the
// whole record as its one segment, and the version advanced with them,
// because a plain write is a save that landed and a fenced writer that
// loaded before it must see the checkpoint move.
//
//	KEYS: rest, version, log, floor
//	ARGV: restBlob, ttlMillis, hasSegment, segmentElem, floor
var savePlainScript = goredis.NewScript(`
local ttl = tonumber(ARGV[2])
local had = redis.call('EXISTS', KEYS[1]) == 1
local function expireWithTTL(key)
  if ttl > 0 then
    redis.call('PEXPIRE', key, ttl)
  else
    redis.call('PERSIST', key)
  end
end
local function setWithTTL(key, value)
  if ttl > 0 then
    redis.call('SET', key, value, 'PX', ttl)
  else
    redis.call('SET', key, value)
  end
end
setWithTTL(KEYS[1], ARGV[1])
setWithTTL(KEYS[4], ARGV[5])
redis.call('DEL', KEYS[3])
if ARGV[3] == '1' then
  redis.call('RPUSH', KEYS[3], ARGV[4])
  expireWithTTL(KEYS[3])
end
if not had then
  redis.call('DEL', KEYS[2])
end
redis.call('INCR', KEYS[2])
expireWithTTL(KEYS[2])
return 1
`)

// GetFencedEncounter implements sdk.FencedEncounterRepository: the rest, its
// version and its record segments in one MULTI/EXEC (loadCheckpoint), so the
// version describes exactly the data returned, joined into one whole world. A
// rest with no version beside it fails closed.
func (r *redisEncounterRepository) GetFencedEncounter(
	ctx context.Context, in *sdk.GetFencedEncounterInput,
) (*sdk.GetFencedEncounterOutput, error) {
	if in == nil || in.Encounter == "" {
		return nil, errors.New("session: GetFencedEncounter encounter is required")
	}
	id := in.Encounter
	stored, err := r.loadCheckpoint(ctx, id)
	if err != nil {
		return nil, err
	}
	if stored.version == "" {
		return nil, fmt.Errorf("encounter %q is stored without a checkpoint version: %w", id, sdk.ErrBadRepository)
	}
	version, err := strconv.ParseUint(stored.version, 10, 64)
	if err != nil || version == 0 {
		return nil, fmt.Errorf("encounter %q has checkpoint version %q: %w", id, stored.version, sdk.ErrBadRepository)
	}
	return &sdk.GetFencedEncounterOutput{Data: stored.data, Version: version}, nil
}

// SaveFenced implements sdk.FencedEncounterRepository for a caller that hands
// the whole world: it is split here (splitWhole) and saved as the record from
// seq 1, so the store holds one shape whichever path wrote it.
func (r *redisEncounterRepository) SaveFenced(ctx context.Context, in *sdk.SaveFencedInput) (*sdk.SaveFencedOutput, error) {
	if in == nil || in.Encounter == "" {
		return nil, errors.New("session: SaveFenced encounter is required")
	}
	if in.Data == nil {
		return nil, errors.New("session: SaveFenced data is required")
	}
	rest, segment, floor, err := splitWhole(in.Data)
	if err != nil {
		return nil, fmt.Errorf("fenced save of encounter %q: %w", in.Encounter, err)
	}
	split := *in
	split.Data = &rest
	return r.saveFenced(ctx, &sdk.SaveAppendedInput{SaveFencedInput: split, Appended: segment, Since: 1, Floor: floor})
}

// SaveAppended implements sdk.AppendingEncounterRepository: the rest, the
// session record when given, the touched sheets and the written seats, under
// the keys their own repositories use, and the record segment appended, in
// one script fenced on in.Expected.
//
// The session record takes this repository's TTL; the orchestrator builds the
// session and encounter repositories with the same one (orchestrator.go).
// Sheets and seats carry no TTL, as their repositories write them.
func (r *redisEncounterRepository) SaveAppended(
	ctx context.Context, in *sdk.SaveAppendedInput,
) (*sdk.SaveFencedOutput, error) {
	if in == nil || in.Encounter == "" {
		return nil, errors.New("session: SaveAppended encounter is required")
	}
	if in.Data == nil {
		return nil, errors.New("session: SaveAppended data is required")
	}
	return r.saveFenced(ctx, in)
}

// saveFenced runs saveFencedScript for one split save.
func (r *redisEncounterRepository) saveFenced(
	ctx context.Context, in *sdk.SaveAppendedInput,
) (*sdk.SaveFencedOutput, error) {
	id := in.Encounter
	if n := len(in.Data.Log.Entries); n > 0 {
		return nil, fmt.Errorf("save of encounter %q: the rest carries %d record entries: %w", id, n, ErrRestCarriesRecord)
	}
	keys := []string{
		sessionEncKeyPrefix + id, sessionEncVerKeyPrefix + id, sessionEncLogKeyPrefix + id, sessionEncFloorKeyPrefix + id,
	}
	restBlob, err := json.Marshal(in.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal encounter %q: %w", id, err)
	}
	hasSegment, segmentElem := "0", []byte(nil)
	if len(in.Appended.Entries) > 0 {
		if segmentElem, err = encodeSegment(in.Appended); err != nil {
			return nil, fmt.Errorf("marshal encounter %q record segment: %w", id, err)
		}
		hasSegment = "1"
	}
	args := []interface{}{
		strconv.FormatUint(in.Expected, 10), r.ttl.Milliseconds(), "0",
		len(in.Characters), len(in.Seats), restBlob,
		strconv.FormatUint(in.Since, 10), hasSegment, segmentElem,
		strconv.FormatUint(in.Floor, 10),
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
