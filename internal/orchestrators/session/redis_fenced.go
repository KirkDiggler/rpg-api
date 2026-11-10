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

// sessionEncVerHighKey is the highest checkpoint version the store has ever
// issued to any encounter (rpg-project#927): one key, never expiring, raised
// by every save. A launch seeds its first version above it.
const sessionEncVerHighKey = "session-enc-ver-high:v1alpha1"

// A VERSION NEVER REPEATS FOR AN ENCOUNTER ID (rpg-project#879 S6, R6). The
// resident copy a process keeps is checked by equality with the store's
// version (GetCheckpointVersion), so a version must name one landed state
// for the life of the id, across its keys expiring or being deleted and the
// session relaunched under the same id: otherwise a copy another process
// kept at version N could match a new world's N by coincidence. Within one
// checkpoint each save adds one; the FIRST save of a checkpoint (no head
// stored) starts one above the high-water key, which every save raises to
// its own version and which nothing expires. So every version a relaunched
// id issues is above every version the id ever had. Only a store that loses
// the high-water key with everything else (FLUSHALL, a fresh Redis) restarts
// the count, and a process holding copies across that must restart too: no
// production path flushes.

// The Redis encounter repository carries the session's optional fenced
// capability and its appending and pieces refinements; the SDK finds them by
// type assertion on Config.Encounters.
var (
	_ sdk.FencedEncounterRepository    = (*redisEncounterRepository)(nil)
	_ sdk.AppendingEncounterRepository = (*redisEncounterRepository)(nil)
	_ sdk.PiecesEncounterRepository    = (*redisEncounterRepository)(nil)
	_ sdk.CheckpointVersionReader      = (*redisEncounterRepository)(nil)
)

// THE VERSION SHARES THE CHECKPOINT'S LIFETIME. Every script that writes the
// head writes the version, the pieces and the record list in the same call
// with the same TTL, so they expire together and nothing deletes one without
// the others (no code deletes the head today). Belt and braces: a version
// found with no head beside it is treated as no checkpoint (0), so a
// lingering version key can never refuse a fresh Launch of the same id; a
// lingering list or hash is dropped by the first save, which always carries
// every piece and appends from seq 1.
//
// NOTHING IS WRITTEN UNTIL EVERYTHING IS CHECKED. A Lua script is atomic
// against other clients but does not roll back a write when a later call in
// it errors, so the fence, the save's wholeness, the record's first seq, the
// stored segments' last seqs, the sheets' existence and their owners are all
// checked before the first write.

// savePiecesScript lands one save of a split world (rpg-project#897 legs c
// and b): the head replaced, the roster and knowledge fields sent written
// (HSET) and the removed ones dropped (HDEL), the journal replaced when sent,
// one record segment appended, the segments wholly below the floor dropped,
// the floor kept beside the head, every piece key's TTL renewed together.
//
//	KEYS: head, version, log, floor, roster, know, journal, [session],
//	      sheet..., seat..., the version high-water (always last)
//	ARGV: 1 expected ("" for the unfenced plain write), 2 ttlMillis,
//	      3 hasSession, 4 nSheets, 5 nSeats, 6 headValue, 7 since,
//	      8 hasSegment, 9 segmentElem, 10 floor, 11 whole, 12 hasJournal,
//	      13 journalValue, 14 nRosterSet, 15 nRosterDel, 16 nKnowSet,
//	      17 nKnowDel, then the roster's field/value pairs, its removed
//	      fields, knowledge's pairs, its removed fields, then [sessionBlob],
//	      sheetBlob..., sheetPlayer..., seatBlob...
//
// A WHOLE save carries every piece the rest has: both hashes are replaced,
// not written into, so a member or observer the rest lost cannot linger. A
// first save (no checkpoint yet) and the plain write must be whole and must
// start the record over (since 1). Each piece value is "<mark>\n<JSON>" and
// each list element "<last seq>\n<segment JSON>", so the floor drop reads the
// prefix alone; a stored segment whose last seq equals the floor is kept (it
// holds the floor itself), and an element whose prefix does not parse
// refuses the save.
//
// Replies {"ok", newVersion}, {"stale", storedVersion}, {"missing", key} or
// {"bad", reason}.
var savePiecesScript = goredis.NewScript(`
local plain = ARGV[1] == ''
local expected = ARGV[1]
local ttl = tonumber(ARGV[2])
local hasSession = ARGV[3] == '1'
local nSheets = tonumber(ARGV[4])
local nSeats = tonumber(ARGV[5])
local since = tonumber(ARGV[7])
local hasSegment = ARGV[8] == '1'
local floor = tonumber(ARGV[10])
local whole = ARGV[11] == '1'
local hasJournal = ARGV[12] == '1'
local nRosterSet = tonumber(ARGV[14])
local nRosterDel = tonumber(ARGV[15])
local nKnowSet = tonumber(ARGV[16])
local nKnowDel = tonumber(ARGV[17])

local p = 18
local rosterSet = p
p = p + 2 * nRosterSet
local rosterDel = p
p = p + nRosterDel
local knowSet = p
p = p + 2 * nKnowSet
local knowDel = p
p = p + nKnowDel

local cur = '0'
if redis.call('EXISTS', KEYS[1]) == 1 then
  local v = redis.call('GET', KEYS[2])
  if v then
    cur = v
  elseif not plain then
    return {'bad', 'encounter stored without a checkpoint version'}
  end
end
if not plain and cur ~= expected then
  return {'stale', cur}
end
if plain or cur == '0' then
  if not whole then
    return {'bad', 'a first checkpoint must carry every piece'}
  end
  if since ~= 1 then
    return {'bad', 'a first checkpoint must append the record from seq 1, not ' .. ARGV[7]}
  end
end
if whole then
  if not hasJournal then
    return {'bad', 'a whole save carries no journal'}
  end
  local function hasShared(first, n)
    for i = 0, n - 1 do
      if ARGV[first + 2 * i] == '_' then
        return true
      end
    end
    return false
  end
  if not hasShared(rosterSet, nRosterSet) then
    return {'bad', 'a whole save carries no roster'}
  end
  if not hasShared(knowSet, nKnowSet) then
    return {'bad', 'a whole save carries no knowledge'}
  end
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

local k = 8
local a = p
if hasSession then
  k = 9
  a = p + 1
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
if whole then
  redis.call('DEL', KEYS[5], KEYS[6])
end
for i = 0, nRosterSet - 1 do
  redis.call('HSET', KEYS[5], ARGV[rosterSet + 2 * i], ARGV[rosterSet + 2 * i + 1])
end
for i = 0, nRosterDel - 1 do
  redis.call('HDEL', KEYS[5], ARGV[rosterDel + i])
end
for i = 0, nKnowSet - 1 do
  redis.call('HSET', KEYS[6], ARGV[knowSet + 2 * i], ARGV[knowSet + 2 * i + 1])
end
for i = 0, nKnowDel - 1 do
  redis.call('HDEL', KEYS[6], ARGV[knowDel + i])
end
expireWithTTL(KEYS[5])
expireWithTTL(KEYS[6])
if hasJournal then
  setWithTTL(KEYS[7], ARGV[13])
else
  expireWithTTL(KEYS[7])
end
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
  setWithTTL(KEYS[8], ARGV[p])
end
for i = 0, nSheets - 1 do
  redis.call('SET', KEYS[k + i], ARGV[a + i])
end
for j = 0, nSeats - 1 do
  redis.call('SET', KEYS[k + nSheets + j], ARGV[a + 2 * nSheets + j])
end

local high = KEYS[#KEYS]
local nv
if cur == '0' then
  nv = tonumber(redis.call('GET', high) or '0') + 1
  redis.call('SET', KEYS[2], string.format('%.0f', nv))
else
  nv = redis.call('INCR', KEYS[2])
end
expireWithTTL(KEYS[2])
if nv > tonumber(redis.call('GET', high) or '0') then
  redis.call('SET', high, string.format('%.0f', nv))
end
return {'ok', nv}
`)

// GetFencedEncounter implements sdk.FencedEncounterRepository: the pieces,
// their marks, the version and the record segments in one MULTI/EXEC
// (loadCheckpoint), so the version and the marks describe exactly the data
// returned, joined into one whole world. A head with no version beside it
// fails closed.
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
	return &sdk.GetFencedEncounterOutput{Data: stored.data, Version: version, Marks: stored.marks}, nil
}

// GetCheckpointVersion implements sdk.CheckpointVersionReader
// (rpg-project#879 S6): one GET of the version key and nothing else, so a
// write verb holding a resident copy checks it against the store for the
// price of one round trip. THE RESIDENT COPY RELIES ON THE VERSION NEVER
// REPEATING FOR AN ID (see sessionEncVerHighKey): equality with a copy's
// version is the whole check. An absent key is 0, which no kept copy holds (a
// landed save leaves the version at 1 or more), so the verb loads and the
// load answers as it always has. It does not look for the head beside the
// version: a version that outlived its head (they share one TTL, so only a
// hand-deleted head) still differs from any copy saved before, or the load
// that follows the miss fails closed.
func (r *redisEncounterRepository) GetCheckpointVersion(ctx context.Context, encounter string) (uint64, error) {
	if encounter == "" {
		return 0, errors.New("session: GetCheckpointVersion encounter is required")
	}
	raw, err := r.client.Get(ctx, sessionEncVerKeyPrefix+encounter).Result()
	if errors.Is(err, goredis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read checkpoint version of encounter %q: %w", encounter, err)
	}
	version, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("encounter %q has checkpoint version %q: %w", encounter, raw, sdk.ErrBadRepository)
	}
	return version, nil
}

// SaveFenced implements sdk.FencedEncounterRepository for a caller that hands
// the whole world: it is split here (splitWhole), its rest into every piece,
// and saved as the record from seq 1, so the store holds one shape whichever
// path wrote it.
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
	return r.SaveAppended(ctx, &sdk.SaveAppendedInput{SaveFencedInput: split, Appended: segment, Since: 1, Floor: floor})
}

// SaveAppended implements sdk.AppendingEncounterRepository for a caller that
// hands the rest whole: every piece of it is written (a whole save), with the
// segment appended, in the one fenced script.
func (r *redisEncounterRepository) SaveAppended(
	ctx context.Context, in *sdk.SaveAppendedInput,
) (*sdk.SaveFencedOutput, error) {
	if in == nil || in.Encounter == "" {
		return nil, errors.New("session: SaveAppended encounter is required")
	}
	if in.Data == nil {
		return nil, errors.New("session: SaveAppended data is required")
	}
	if n := len(in.Data.Log.Entries); n > 0 {
		return nil, fmt.Errorf("save of encounter %q: the rest carries %d record entries: %w",
			in.Encounter, n, ErrRestCarriesRecord)
	}
	write, err := splitAll(in.Data)
	if err != nil {
		return nil, fmt.Errorf("save of encounter %q: %w", in.Encounter, err)
	}
	return r.saveFenced(ctx, in, write)
}

// SavePieces implements sdk.PiecesEncounterRepository: the pieces whose mark
// moved, the removed ones dropped, the session record when given, the
// touched sheets and the written seats, under the keys their own
// repositories use, and the record segment appended, in one script fenced on
// in.Expected. A save that carries every piece its marks name is whole: the
// hashes are replaced rather than written into.
//
// The session record takes this repository's TTL; the orchestrator builds the
// session and encounter repositories with the same one (orchestrator.go).
// Sheets and seats carry no TTL, as their repositories write them.
func (r *redisEncounterRepository) SavePieces(
	ctx context.Context, in *sdk.SavePiecesInput,
) (*sdk.SaveFencedOutput, error) {
	if in == nil || in.Encounter == "" {
		return nil, errors.New("session: SavePieces encounter is required")
	}
	if len(in.Marks) == 0 {
		return nil, fmt.Errorf("save of encounter %q: SavePieces carries no marks: %w", in.Encounter, sdk.ErrBadRepository)
	}
	write, err := piecesWriteOf(in.Pieces, in.Removed, isWhole(in.Pieces, in.Marks))
	if err != nil {
		return nil, fmt.Errorf("save of encounter %q: %w: %w", in.Encounter, err, sdk.ErrBadRepository)
	}
	return r.saveFenced(ctx, &in.SaveAppendedInput, write)
}

// checkpointKeys is an encounter's checkpoint keys in the script's order:
// head, version, log, floor, roster, knowledge, journal.
func checkpointKeys(id string) []string {
	return []string{
		sessionEncKeyPrefix + id, sessionEncVerKeyPrefix + id, sessionEncLogKeyPrefix + id,
		sessionEncFloorKeyPrefix + id, sessionEncRosterKeyPrefix + id, sessionEncKnowKeyPrefix + id,
		sessionEncJournalKeyPrefix + id,
	}
}

// piecesArgs is the script's arguments from ARGV[1] through the pieces:
// expected ("" for the plain write), the TTL, the session/sheet/seat counts,
// the head, the record's since, segment and floor, then the pieces.
func (r *redisEncounterRepository) piecesArgs(
	expected string, in *sdk.SaveAppendedInput, write *piecesWrite,
) ([]interface{}, error) {
	hasSegment, segmentElem := "0", []byte(nil)
	if len(in.Appended.Entries) > 0 {
		var err error
		if segmentElem, err = encodeSegment(in.Appended); err != nil {
			return nil, fmt.Errorf("marshal encounter %q record segment: %w", in.Encounter, err)
		}
		hasSegment = "1"
	}
	whole, hasJournal := "0", "0"
	if write.whole {
		whole = "1"
	}
	if write.journal != nil {
		hasJournal = "1"
	}
	args := make([]interface{}, 0,
		17+len(write.rosterSet)+len(write.rosterDel)+len(write.knowSet)+len(write.knowDel))
	args = append(args,
		expected, r.ttl.Milliseconds(), "0", len(in.Characters), len(in.Seats), write.head,
		strconv.FormatUint(in.Since, 10), hasSegment, segmentElem, strconv.FormatUint(in.Floor, 10),
		whole, hasJournal, write.journal,
		len(write.rosterSet)/2, len(write.rosterDel), len(write.knowSet)/2, len(write.knowDel),
	)
	args = append(args, write.rosterSet...)
	args = append(args, write.rosterDel...)
	args = append(args, write.knowSet...)
	args = append(args, write.knowDel...)
	return args, nil
}

// saveFenced runs savePiecesScript for one fenced save.
func (r *redisEncounterRepository) saveFenced(
	ctx context.Context, in *sdk.SaveAppendedInput, write *piecesWrite,
) (*sdk.SaveFencedOutput, error) {
	id := in.Encounter
	keys := checkpointKeys(id)
	args, err := r.piecesArgs(strconv.FormatUint(in.Expected, 10), in, write)
	if err != nil {
		return nil, err
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

	keys = append(keys, sessionEncVerHighKey)
	reply, err := savePiecesScript.Run(ctx, r.client, keys, args...).Slice()
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
