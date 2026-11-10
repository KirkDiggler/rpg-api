package session

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
)

// THE REST IS STORED AS PIECES (rpg-project#897 leg b). The encounter's
// SplitPieces groups the rest by rpg-project#770's shapes; this store keeps:
//
//   - the HEAD (clocks, log head, site hash; written every save) as a string
//     at sessionEncKeyPrefix, which is also the checkpoint's existence
//     sentinel;
//   - the ROSTER as one hash: field "_" the roster's shared part, field
//     "m:<member>" one per member;
//   - KNOWLEDGE as one hash: field "_" the shared part, field "o:<observer>"
//     one per observer (its holdings and its sight sweep);
//   - the JOURNAL as a string.
//
// Every value is "<mark>\n<piece JSON>": the piece's mark, decimal, then its
// bytes, as a record segment carries its last seq. A mark can therefore never
// disagree with its piece, and the load hands the marks back to the session,
// which splits its next save against them so only the pieces that moved are
// written. The record's segments, its floor and the version keep their keys
// (redis_record.go, redis_fenced.go).
//
// WHY A NEW PREFIX FOR EVERY PIECE. A leg (c) rest under session-enc:v1alpha2
// decodes as a whole world; a reader that found it beside pieces could load a
// stale whole and miss them. Nothing writes v1alpha2 any more, and a
// checkpoint found only there is refused by name ([ErrCheckpointBeforePieces]).
const (
	// sessionEncRosterKeyPrefix keys an encounter's roster hash.
	sessionEncRosterKeyPrefix = "session-enc-roster:v1alpha3:"
	// sessionEncKnowKeyPrefix keys an encounter's knowledge hash.
	sessionEncKnowKeyPrefix = "session-enc-know:v1alpha3:"
	// sessionEncJournalKeyPrefix keys an encounter's journal.
	sessionEncJournalKeyPrefix = "session-enc-journal:v1alpha3:"

	// appendedRestKeyPrefix is where a leg (c) checkpoint kept its whole
	// rest. Read only to refuse it by name.
	appendedRestKeyPrefix = "session-enc:v1alpha2:"

	// sharedField is the hash field of a shape's shared piece (the roster's
	// doors, reserve, outcome, pause and ever-members; knowledge's discovery
	// and sneak verdicts).
	sharedField = "_"
)

// encodePiece is one piece's stored value: its mark, a newline, its bytes.
func encodePiece(p tkencounter.Piece) []byte {
	out := make([]byte, 0, len(p.Bytes)+21)
	out = strconv.AppendUint(out, uint64(p.Mark), 10)
	out = append(out, '\n')
	return append(out, p.Bytes...)
}

// decodePiece reads one stored value back into its mark and bytes. A value
// without a mark prefix is malformed, never read as a bare piece.
func decodePiece(value string) (tkencounter.Mark, []byte, error) {
	prefix, body, ok := strings.Cut(value, "\n")
	if !ok {
		return 0, nil, errors.New("value has no mark prefix")
	}
	mark, err := strconv.ParseUint(prefix, 10, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("mark prefix %q: %v", prefix, err)
	}
	return tkencounter.Mark(mark), []byte(body), nil
}

// fieldOf is the hash field a roster or knowledge piece is stored under: "_"
// for the shape's shared piece, the key itself ("m:<id>", "o:<observer>")
// for the rest.
func fieldOf(key tkencounter.PieceKey) string {
	if key == tkencounter.PieceRoster || key == tkencounter.PieceKnowledge {
		return sharedField
	}
	return string(key)
}

// keyOfField is the piece key a stored hash field names within shape, or
// false for a field that names no piece of that shape.
func keyOfField(shape tkencounter.PieceShape, field string) (tkencounter.PieceKey, bool) {
	if field == sharedField {
		if shape == tkencounter.ShapeRoster {
			return tkencounter.PieceRoster, true
		}
		return tkencounter.PieceKnowledge, true
	}
	key := tkencounter.PieceKey(field)
	if key.Shape() != shape || key == tkencounter.PieceRoster || key == tkencounter.PieceKnowledge {
		return "", false
	}
	return key, true
}

// piecesWrite is one save's pieces, routed to the keys and fields the script
// writes, in key order so a save's arguments are deterministic.
type piecesWrite struct {
	head []byte
	// journal is nil when the journal did not move.
	journal []byte
	// rosterSet and knowSet are field, value pairs.
	rosterSet, knowSet []interface{}
	rosterDel, knowDel []interface{}
	// whole means the save carries every piece the rest has: the script
	// replaces both hashes rather than writing into them, so a field the
	// rest lost cannot linger.
	whole bool
	// count is the pieces written, for the error that names a bad save.
	count int
}

// piecesWriteOf routes a split's pieces and removals to the store's keys. A
// save must carry the head; a whole save must carry every fixed piece.
// Removing the head, the journal or a shared piece is refused: the rest
// always has them.
func piecesWriteOf(
	pieces map[tkencounter.PieceKey]tkencounter.Piece, removed []tkencounter.PieceKey, whole bool,
) (*piecesWrite, error) {
	head, ok := pieces[tkencounter.PieceHead]
	if !ok {
		return nil, fmt.Errorf("the save carries no %s piece", tkencounter.PieceHead)
	}
	out := &piecesWrite{head: encodePiece(head), whole: whole, count: len(pieces)}
	if whole {
		for _, fixed := range []tkencounter.PieceKey{
			tkencounter.PieceRoster, tkencounter.PieceKnowledge, tkencounter.PieceJournal,
		} {
			if _, ok := pieces[fixed]; !ok {
				return nil, fmt.Errorf("a whole save carries no %s piece", fixed)
			}
		}
	}
	keys := make([]string, 0, len(pieces))
	for key := range pieces {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	for _, k := range keys {
		key := tkencounter.PieceKey(k)
		value := encodePiece(pieces[key])
		switch key.Shape() {
		case tkencounter.ShapeHead:
		case tkencounter.ShapeJournal:
			out.journal = value
		case tkencounter.ShapeRoster:
			out.rosterSet = append(out.rosterSet, fieldOf(key), value)
		case tkencounter.ShapeKnowledge:
			out.knowSet = append(out.knowSet, fieldOf(key), value)
		default:
			return nil, fmt.Errorf("piece %q names no shape", key)
		}
	}
	for _, key := range removed {
		if fieldOf(key) == sharedField {
			return nil, fmt.Errorf("the save removes %s, which every rest has", key)
		}
		switch key.Shape() {
		case tkencounter.ShapeRoster:
			out.rosterDel = append(out.rosterDel, string(key))
		case tkencounter.ShapeKnowledge:
			out.knowDel = append(out.knowDel, string(key))
		default:
			return nil, fmt.Errorf("the save removes %q, which every rest has", key)
		}
	}
	return out, nil
}

// splitAll is every piece of a rest, for a save that hands the rest whole
// (SaveEncounter, SaveFenced, SaveAppended): the store holds one shape
// whichever path wrote it.
func splitAll(rest *tkencounter.EncounterData) (*piecesWrite, error) {
	split, err := tkencounter.SplitPieces(&tkencounter.SplitPiecesInput{Data: rest})
	if err != nil {
		return nil, err
	}
	return piecesWriteOf(split.Pieces, nil, true)
}

// isWhole reports whether a pieces save carries every piece its marks name,
// which is what a save split against no held marks does (a Launch, or a host
// that kept none).
func isWhole(pieces map[tkencounter.PieceKey]tkencounter.Piece, marks map[tkencounter.PieceKey]tkencounter.Mark) bool {
	for key := range marks {
		if _, ok := pieces[key]; !ok {
			return false
		}
	}
	return true
}
