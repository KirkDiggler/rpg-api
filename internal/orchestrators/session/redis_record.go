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

// THE RECORD IS SEGMENTS BESIDE THE PIECES (rpg-project#897 leg c). The
// story record lives beside the rest's pieces (redis_pieces.go) as a Redis
// list of SEGMENTS at sessionEncLogKeyPrefix, one per save that wrote beats,
// each a record.LogData with its own audience table. A save appends one
// segment; a read joins them onto the head's log with record.JoinData. The
// version key (redis_fenced.go) is unchanged.
const (
	// sessionEncLogKeyPrefix keys an encounter's record segments: a list, oldest
	// first, each element "<last seq>\n<segment JSON>" so the save script can
	// drop segments below the retention floor without decoding them.
	sessionEncLogKeyPrefix = "session-enc-log:v1alpha2:"

	// sessionEncFloorKeyPrefix keys an encounter's retention floor: the seq
	// below which the store may hold nothing, as the last save named it,
	// written with the head under the same TTL. The read joins on it, so a
	// record the store lost at or above it is refused, never loaded short.
	sessionEncFloorKeyPrefix = "session-enc-floor:v1alpha2:"

	// legacyEncKeyPrefix is where a checkpoint saved before the appended
	// record kept its whole world in one blob. Read only to refuse it by name.
	legacyEncKeyPrefix = "session-enc:v1alpha1:"
)

// ErrCheckpointBeforePieces is a stored encounter found only under a key an
// earlier store shape wrote: leg (c)'s whole rest (session-enc:v1alpha2) or
// the single blob before it (session-enc:v1alpha1), with no head under the
// pieces' key (rpg-project#897 leg b). It is refused rather than migrated:
// pre-playtest, every live session relaunches. The session handlers map it
// to FAILED_PRECONDITION (sdkerr): the request is well formed and the session
// exists, but this server's stored state refuses the verb.
var ErrCheckpointBeforePieces = errors.New(
	"checkpoint saved before the rest was stored as pieces (rpg-project#897); relaunch the session")

// ErrCheckpointMismatch is a stored checkpoint whose keys disagree, named by
// what is wrong: a piece missing (the roster, knowledge or the journal beside
// a head, or pieces, a record list or a floor with no head), a piece or mark
// that does not decode, a hash field that names no piece, a malformed
// segment, or a record missing a seq at or above the floor. The read fails
// closed rather than load a world short a shape or a story. It is wrapped
// beside sdk.ErrBadRepository, and the session handlers map it to
// FAILED_PRECONDITION (sdkerr): the stored state refuses the verb until the
// session is relaunched, and retrying as is cannot help.
var ErrCheckpointMismatch = errors.New("stored checkpoint's pieces disagree")

// ErrRestCarriesRecord is a fenced save whose rest still carries record
// entries: the record travels only as the appended segment, so a rest with
// entries is a caller's bug and is refused before anything is written.
var ErrRestCarriesRecord = errors.New("the rest of an appended save carries record entries")

// mismatch wraps a disagreement between a checkpoint's pieces by name.
func mismatch(id, format string, args ...interface{}) error {
	return fmt.Errorf("encounter %q: %s: %w: %w", id, fmt.Sprintf(format, args...),
		ErrCheckpointMismatch, sdk.ErrBadRepository)
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
	// marks is the mark stored beside every piece.
	marks map[tkencounter.PieceKey]tkencounter.Mark
}

// loadCheckpoint reads the head, both hashes, the journal, the floor, the
// version and the segments in one MULTI/EXEC, so the version and the marks
// describe exactly the data read, joins the pieces (encounter.JoinPieces) and
// then the record on the stored floor into one whole world. An encounter
// stored only under an earlier shape's key is refused with
// ErrCheckpointBeforePieces; one stored nowhere is sdk.ErrNotFound; keys that
// disagree are ErrCheckpointMismatch, naming the piece.
func (r *redisEncounterRepository) loadCheckpoint(ctx context.Context, id string) (*storedCheckpoint, error) {
	var (
		headCmd    *goredis.StringCmd
		rosterCmd  *goredis.MapStringStringCmd
		knowCmd    *goredis.MapStringStringCmd
		journalCmd *goredis.StringCmd
		floorCmd   *goredis.StringCmd
		verCmd     *goredis.StringCmd
		logCmd     *goredis.StringSliceCmd
		earlierCmd *goredis.IntCmd
	)
	_, err := r.client.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
		headCmd = pipe.Get(ctx, sessionEncKeyPrefix+id)
		rosterCmd = pipe.HGetAll(ctx, sessionEncRosterKeyPrefix+id)
		knowCmd = pipe.HGetAll(ctx, sessionEncKnowKeyPrefix+id)
		journalCmd = pipe.Get(ctx, sessionEncJournalKeyPrefix+id)
		floorCmd = pipe.Get(ctx, sessionEncFloorKeyPrefix+id)
		verCmd = pipe.Get(ctx, sessionEncVerKeyPrefix+id)
		logCmd = pipe.LRange(ctx, sessionEncLogKeyPrefix+id, 0, -1)
		earlierCmd = pipe.Exists(ctx, appendedRestKeyPrefix+id, legacyEncKeyPrefix+id)
		return nil
	})
	if err != nil && !errors.Is(err, goredis.Nil) {
		return nil, fmt.Errorf("get encounter %q: %w", id, err)
	}

	elems, err := logCmd.Result()
	if err != nil {
		return nil, fmt.Errorf("get encounter %q record: %w", id, err)
	}
	roster, err := rosterCmd.Result()
	if err != nil {
		return nil, fmt.Errorf("get encounter %q roster: %w", id, err)
	}
	know, err := knowCmd.Result()
	if err != nil {
		return nil, fmt.Errorf("get encounter %q knowledge: %w", id, err)
	}
	rawJournal, journalErr := journalCmd.Result()
	if journalErr != nil && !errors.Is(journalErr, goredis.Nil) {
		return nil, fmt.Errorf("get encounter %q journal: %w", id, journalErr)
	}
	hasJournal := journalErr == nil
	rawFloor, floorErr := floorCmd.Result()
	if floorErr != nil && !errors.Is(floorErr, goredis.Nil) {
		return nil, fmt.Errorf("get encounter %q floor: %w", id, floorErr)
	}
	hasFloor := floorErr == nil

	rawHead, err := headCmd.Result()
	if errors.Is(err, goredis.Nil) {
		if len(elems) > 0 || hasFloor || len(roster) > 0 || len(know) > 0 || hasJournal {
			return nil, mismatch(id, "pieces or a record with no %s (roster %d, knowledge %d, journal %v, "+
				"%d segments, floor %v)", tkencounter.PieceHead, len(roster), len(know), hasJournal, len(elems), hasFloor)
		}
		if earlierCmd.Val() > 0 {
			return nil, fmt.Errorf("encounter %q: %w", id, ErrCheckpointBeforePieces)
		}
		return nil, fmt.Errorf("encounter %q: %w", id, sdk.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get encounter %q: %w", id, err)
	}

	pieces := map[tkencounter.PieceKey][]byte{}
	marks := map[tkencounter.PieceKey]tkencounter.Mark{}
	put := func(key tkencounter.PieceKey, value string) error {
		mark, body, decodeErr := decodePiece(value)
		if decodeErr != nil {
			return mismatch(id, "piece %s: %v", key, decodeErr)
		}
		pieces[key], marks[key] = body, mark
		return nil
	}
	if putErr := put(tkencounter.PieceHead, rawHead); putErr != nil {
		return nil, putErr
	}
	if hasJournal {
		if putErr := put(tkencounter.PieceJournal, rawJournal); putErr != nil {
			return nil, putErr
		}
	}
	for _, hash := range []struct {
		shape  tkencounter.PieceShape
		fields map[string]string
	}{{tkencounter.ShapeRoster, roster}, {tkencounter.ShapeKnowledge, know}} {
		for field, value := range hash.fields {
			key, ok := keyOfField(hash.shape, field)
			if !ok {
				return nil, mismatch(id, "%s field %q names no piece", hash.shape, field)
			}
			if putErr := put(key, value); putErr != nil {
				return nil, putErr
			}
		}
	}
	data, err := tkencounter.JoinPieces(&tkencounter.JoinPiecesInput{Pieces: pieces})
	if err != nil {
		// The join names the piece and keeps its cause (ErrMissingPiece,
		// ErrBadPiece and the decode error) in the chain.
		return nil, fmt.Errorf("encounter %q: %w: %w: %w", id, err, ErrCheckpointMismatch, sdk.ErrBadRepository)
	}

	if !hasFloor {
		return nil, mismatch(id, "a head with no stored floor")
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

	out := &storedCheckpoint{data: data, marks: marks}
	if v, verErr := verCmd.Result(); verErr == nil {
		out.version = v
	} else if !errors.Is(verErr, goredis.Nil) {
		return nil, fmt.Errorf("get encounter %q version: %w", id, verErr)
	}
	return out, nil
}
