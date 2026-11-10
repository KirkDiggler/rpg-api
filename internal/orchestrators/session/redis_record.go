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

// decodeSegment reads one list element back into its segment.
func decodeSegment(elem string) (record.LogData, error) {
	_, body, ok := strings.Cut(elem, "\n")
	if !ok {
		return record.LogData{}, errors.New("segment has no last-seq prefix")
	}
	var seg record.LogData
	if err := json.Unmarshal([]byte(body), &seg); err != nil {
		return record.LogData{}, err
	}
	return seg, nil
}

// splitWhole splits a whole world the way the encounter's ToCheckpoint does,
// through the record's own functions: the rest with the log's head, and the
// whole record as one segment from seq 1 (empty for a record with no
// entries). The plain SaveEncounter writes this, so both save paths leave one
// shape in the store.
func splitWhole(data *tkencounter.EncounterData) (tkencounter.EncounterData, record.LogData, error) {
	log, err := record.LoadLog(data.Log)
	if err != nil {
		return tkencounter.EncounterData{}, record.LogData{}, fmt.Errorf("load the record to split it: %w", err)
	}
	next, err := log.NextSeq()
	if err != nil {
		return tkencounter.EncounterData{}, record.LogData{}, err
	}
	head, err := log.DataFrom(&record.DataFromInput{Since: next})
	if err != nil {
		return tkencounter.EncounterData{}, record.LogData{}, err
	}
	rest := *data
	rest.Log = head
	return rest, data.Log, nil
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

// loadCheckpoint reads the rest, the version and the segments in one
// MULTI/EXEC, so the version describes exactly the data read, and joins them
// into one whole world. An encounter stored only under the legacy single-blob
// key is refused with ErrCheckpointBeforeAppendedRecord; one stored nowhere is
// sdk.ErrNotFound.
func (r *redisEncounterRepository) loadCheckpoint(ctx context.Context, id string) (*storedCheckpoint, error) {
	var (
		restCmd   *goredis.StringCmd
		verCmd    *goredis.StringCmd
		logCmd    *goredis.StringSliceCmd
		legacyCmd *goredis.IntCmd
	)
	_, err := r.client.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
		restCmd = pipe.Get(ctx, sessionEncKeyPrefix+id)
		verCmd = pipe.Get(ctx, sessionEncVerKeyPrefix+id)
		logCmd = pipe.LRange(ctx, sessionEncLogKeyPrefix+id, 0, -1)
		legacyCmd = pipe.Exists(ctx, legacyEncKeyPrefix+id)
		return nil
	})
	if err != nil && !errors.Is(err, goredis.Nil) {
		return nil, fmt.Errorf("get encounter %q: %w", id, err)
	}

	rest, err := restCmd.Bytes()
	if errors.Is(err, goredis.Nil) {
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

	elems, err := logCmd.Result()
	if err != nil {
		return nil, fmt.Errorf("get encounter %q record: %w", id, err)
	}
	segments := make([]record.LogData, 0, len(elems))
	for i, elem := range elems {
		seg, decodeErr := decodeSegment(elem)
		if decodeErr != nil {
			return nil, fmt.Errorf("encounter %q record segment %d: %v: %w", id, i, decodeErr, sdk.ErrBadRepository)
		}
		segments = append(segments, seg)
	}
	joined, err := joinRecord(data.Log, storedFloor(&data, segments), segments)
	if err != nil {
		return nil, fmt.Errorf("encounter %q record: %v: %w", id, err, sdk.ErrBadRepository)
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

// storedFloor is the seq below which the store may hold nothing. An unbounded
// record keeps everything, so every seq from 1 must be there (0). A bounded
// one may have lost what the save scripts dropped below its floor, so the
// oldest seq still held is the floor; the next load's retention trims the
// rest of the straddling segment in memory.
func storedFloor(data *tkencounter.EncounterData, segments []record.LogData) uint64 {
	if data.Retention < 0 {
		return 0
	}
	for _, seg := range segments {
		if len(seg.Entries) > 0 {
			return seg.Entries[0].Seq
		}
	}
	return data.Log.NextSeq
}
