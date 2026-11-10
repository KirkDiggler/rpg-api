package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	goredis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-toolkit/play/record"
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// THE CHECKPOINT IS TWO KEYS AND A VERSION (rpg-project#897 leg c). The
// encounter's REST, the world with its story log reduced to the head (NextSeq
// and LastID), lives at sessionEncKeyPrefix; the story record lives beside it
// as a Redis list of SEGMENTS at sessionEncLogKeyPrefix, one per save that
// wrote beats, each a record.LogData with its own audience table. A save
// replaces the rest and appends one segment; a read joins them with
// record.JoinData. The version key (redis_fenced.go) is unchanged.
//
// WHY A NEW PREFIX. A rest alone decodes as a legal trimmed-empty record, so a
// reader of the old single-blob key handed a rest would load an encounter with
// a silently empty story. The old prefix is never written again, and a
// checkpoint found only there is refused by name
// ([ErrCheckpointBeforeAppendedRecord]).
const (
	// sessionEncLogKeyPrefix keys an encounter's record segments: a list, oldest
	// first, each element "<last seq>\n<segment JSON>" so the save script can
	// drop segments below the retention floor without decoding them.
	sessionEncLogKeyPrefix = "session-enc-log:v1alpha2:"

	// sessionEncFloorKeyPrefix keys an encounter's retention floor: the seq
	// below which the store may hold nothing, as the last save named it,
	// written with the rest under the same TTL. The read joins on it, so a
	// record the store lost at or above it is refused, never loaded short.
	sessionEncFloorKeyPrefix = "session-enc-floor:v1alpha2:"

	// legacyEncKeyPrefix is where a checkpoint saved before the appended
	// record kept its whole world in one blob. Read only to refuse it by name.
	legacyEncKeyPrefix = "session-enc:v1alpha1:"
)

// ErrCheckpointBeforeAppendedRecord is a stored encounter found only under
// the single-blob key every checkpoint used before the appended record
// (rpg-project#897 leg c). It is refused rather than migrated: pre-playtest,
// every live session relaunches. The session handlers map it to
// FAILED_PRECONDITION (sdkerr): the request is well formed and the session
// exists, but this server's stored state refuses the verb.
var ErrCheckpointBeforeAppendedRecord = errors.New(
	"checkpoint saved before the appended record (rpg-project#897); relaunch the session")

// ErrCheckpointRecordMismatch is a stored checkpoint whose rest, record
// list and floor disagree: a record list or floor with no rest, a rest with
// no floor, a malformed segment, or a record missing a seq at or above the
// floor (which includes a rest whose list is gone while its next seq is above
// the floor). The read fails closed rather than load a short or empty story.
// It is wrapped beside sdk.ErrBadRepository.
var ErrCheckpointRecordMismatch = errors.New("stored checkpoint and its record disagree")

// ErrRestCarriesRecord is a fenced save whose rest still carries record
// entries: the record travels only as the appended segment, so a rest with
// entries is a caller's bug and is refused before anything is written.
var ErrRestCarriesRecord = errors.New("the rest of an appended save carries record entries")

// mismatch wraps a disagreement between a checkpoint's pieces by name.
func mismatch(id, format string, args ...interface{}) error {
	return fmt.Errorf("encounter %q: %s: %w: %w", id, fmt.Sprintf(format, args...),
		ErrCheckpointRecordMismatch, sdk.ErrBadRepository)
}

// encodeSegment is one segment as its list element: the last seq it holds, a
// newline, and its JSON. The caller never encodes an empty segment.
func encodeSegment(seg record.LogData) ([]byte, error) {
	b, err := json.Marshal(seg)
	if err != nil {
		return nil, err
	}
	last := seg.Entries[len(seg.Entries)-1].Seq
	out := make([]byte, 0, len(b)+21)
	out = strconv.AppendUint(out, last, 10)
	out = append(out, '\n')
	return append(out, b...), nil
}

// decodeSegment reads one list element back into its segment. The prefix
// must be a seq and must name the segment's own last entry: an element that
// fails either is malformed, never skipped.
func decodeSegment(elem string) (record.LogData, error) {
	prefix, body, ok := strings.Cut(elem, "\n")
	if !ok {
		return record.LogData{}, errors.New("segment has no last-seq prefix")
	}
	last, err := strconv.ParseUint(prefix, 10, 64)
	if err != nil {
		return record.LogData{}, fmt.Errorf("segment last-seq prefix %q: %v", prefix, err)
	}
	var seg record.LogData
	if err := json.Unmarshal([]byte(body), &seg); err != nil {
		return record.LogData{}, err
	}
	if len(seg.Entries) == 0 {
		return record.LogData{}, errors.New("segment holds no entries")
	}
	if got := seg.Entries[len(seg.Entries)-1].Seq; got != last {
		return record.LogData{}, fmt.Errorf("segment prefix names seq %d, its last entry is %d", last, got)
	}
	return seg, nil
}

// splitWhole splits a whole world the way the encounter's ToCheckpoint does,
// through the record's own functions: the rest with the log's head, and the
// whole record as one segment from seq 1 (empty for a record with no
// entries), and its floor: the oldest seq the record still holds, its next
// seq when it holds none, 0 for a fresh one (record.JoinDataInput.Floor). The
// plain SaveEncounter writes this, so both save paths leave one shape in the
// store.
func splitWhole(data *tkencounter.EncounterData) (tkencounter.EncounterData, record.LogData, uint64, error) {
	log, err := record.LoadLog(data.Log)
	if err != nil {
		return tkencounter.EncounterData{}, record.LogData{}, 0, fmt.Errorf("load the record to split it: %w", err)
	}
	next, err := log.NextSeq()
	if err != nil {
		return tkencounter.EncounterData{}, record.LogData{}, 0, err
	}
	head, err := log.DataFrom(&record.DataFromInput{Since: next})
	if err != nil {
		return tkencounter.EncounterData{}, record.LogData{}, 0, err
	}
	floor := data.Log.NextSeq
	if len(data.Log.Entries) > 0 {
		floor = data.Log.Entries[0].Seq
	}
	rest := *data
	rest.Log = head
	return rest, data.Log, floor, nil
}

// joinRecord is the record's join, the one call site of its shape.
func joinRecord(head record.LogData, floor uint64, segments []record.LogData) (record.LogData, error) {
	return record.JoinData(&record.JoinDataInput{Head: head, Floor: floor, Segments: segments})
}

// storedCheckpoint is one encounter as the store holds it, read together.
type storedCheckpoint struct {
	data *tkencounter.EncounterData
	// version is the raw version key, "" when absent.
	version string
}

// loadCheckpoint reads the rest, the version, the segments and the floor in
// one MULTI/EXEC, so the version describes exactly the data read, and joins
// them on the stored floor into one whole world. An encounter stored only
// under the legacy single-blob key is refused with
// ErrCheckpointBeforeAppendedRecord; one stored nowhere is sdk.ErrNotFound;
// pieces that disagree are ErrCheckpointRecordMismatch.
func (r *redisEncounterRepository) loadCheckpoint(ctx context.Context, id string) (*storedCheckpoint, error) {
	var (
		restCmd   *goredis.StringCmd
		verCmd    *goredis.StringCmd
		logCmd    *goredis.StringSliceCmd
		floorCmd  *goredis.StringCmd
		legacyCmd *goredis.IntCmd
	)
	_, err := r.client.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
		restCmd = pipe.Get(ctx, sessionEncKeyPrefix+id)
		verCmd = pipe.Get(ctx, sessionEncVerKeyPrefix+id)
		logCmd = pipe.LRange(ctx, sessionEncLogKeyPrefix+id, 0, -1)
		floorCmd = pipe.Get(ctx, sessionEncFloorKeyPrefix+id)
		legacyCmd = pipe.Exists(ctx, legacyEncKeyPrefix+id)
		return nil
	})
	if err != nil && !errors.Is(err, goredis.Nil) {
		return nil, fmt.Errorf("get encounter %q: %w", id, err)
	}

	elems, err := logCmd.Result()
	if err != nil {
		return nil, fmt.Errorf("get encounter %q record: %w", id, err)
	}
	rawFloor, floorErr := floorCmd.Result()
	if floorErr != nil && !errors.Is(floorErr, goredis.Nil) {
		return nil, fmt.Errorf("get encounter %q floor: %w", id, floorErr)
	}
	hasFloor := floorErr == nil

	rest, err := restCmd.Bytes()
	if errors.Is(err, goredis.Nil) {
		if len(elems) > 0 || hasFloor {
			return nil, mismatch(id, "a record (%d segments, floor stored %v) with no rest", len(elems), hasFloor)
		}
		if legacyCmd.Val() == 1 {
			return nil, fmt.Errorf("encounter %q: %w", id, ErrCheckpointBeforeAppendedRecord)
		}
		return nil, fmt.Errorf("encounter %q: %w", id, sdk.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get encounter %q: %w", id, err)
	}
	var data tkencounter.EncounterData
	if err = json.Unmarshal(rest, &data); err != nil {
		return nil, fmt.Errorf("unmarshal stored encounter %q: %w", id, err)
	}

	if !hasFloor {
		return nil, mismatch(id, "a rest with no stored floor")
	}
	floor, err := strconv.ParseUint(rawFloor, 10, 64)
	if err != nil {
		return nil, mismatch(id, "stored floor %q: %v", rawFloor, err)
	}
	segments := make([]record.LogData, 0, len(elems))
	for i, elem := range elems {
		seg, decodeErr := decodeSegment(elem)
		if decodeErr != nil {
			return nil, mismatch(id, "record segment %d: %v", i, decodeErr)
		}
		segments = append(segments, seg)
	}
	joined, err := joinRecord(data.Log, floor, segments)
	if err != nil {
		return nil, mismatch(id, "record at floor %d: %v", floor, err)
	}
	data.Log = joined

	out := &storedCheckpoint{data: &data}
	if v, verErr := verCmd.Result(); verErr == nil {
		out.version = v
	} else if !errors.Is(verErr, goredis.Nil) {
		return nil, fmt.Errorf("get encounter %q version: %w", id, verErr)
	}
	return out, nil
}
