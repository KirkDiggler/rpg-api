package dungeons_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
)

// THE CONTENT CHECK ON THE SAVE (rpg-project#1024): a status a line can give
// that the rules refuse is refused when the author checks or saves the site,
// at its place under `statuses:`, and nothing is stored; the castle's own
// status passes.

// kitchenGivingAStatus is the author's kitchen (the castle kitchen under key)
// with the castle's `statuses:` section, its one change word swapped for
// changeWord, and a plain line on the first cook that gives it.
func (s *StoresSuite) kitchenGivingAStatus(key, changeWord string) []byte {
	castle, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(s.T()), "the-mad-king.yaml"))
	s.Require().NoError(err)
	start := bytes.Index(castle, []byte("\nstatuses:\n"))
	s.Require().Positive(start, "the castle writes a statuses section")
	end := bytes.Index(castle[start:], []byte("\nexits:\n"))
	s.Require().Positive(end, "the statuses section ends at exits")
	statuses := bytes.Clone(castle[start+1 : start+end+1])
	good := []byte("      - initiative: 1\n")
	s.Require().Equal(1, bytes.Count(statuses, good), "precondition: the castle's warmed-up change is written once")
	statuses = bytes.Replace(statuses, good, []byte("      - "+changeWord+": 1\n"), 1)

	raw := s.file(key)
	room := []byte("\nroom:\n")
	s.Require().Equal(1, bytes.Count(raw, room))
	raw = bytes.Replace(raw, room, append(append([]byte("\n"), statuses...), room[1:]...), 1)
	cook := []byte("      cook-1: {faction: kitchen}\n")
	s.Require().Equal(1, bytes.Count(raw, cook))
	return bytes.Replace(raw, cook, []byte(`      cook-1:
        faction: kitchen
        options:
          - id: help
            say: (lend a hand with the pies)
            success:
              - say: Bless you. Here, eat.
                apply_status: { status: warmed-up, to: speaker }
`), 1)
}

func (s *StoresSuite) TestPutRefusesAStatusTheRulesRefuseAndStoresNothing() {
	r := s.registry(true)
	good, err := r.Put(s.ctx, &dungeons.PutInput{
		Caller: alice, Key: storeMyCastle, YAML: s.kitchenGivingAStatus(storeMyCastle, "initiative"), ValidateOnly: true,
	})
	s.Require().NoError(err)
	s.Require().Empty(good.Errors, "control: the castle's own status, given by a line, is taken")

	raw := s.kitchenGivingAStatus(storeMyCastle, "luckier")
	for _, validateOnly := range []bool{true, false} {
		out, putErr := r.Put(s.ctx, &dungeons.PutInput{Caller: alice, Key: storeMyCastle, YAML: raw, ValidateOnly: validateOnly})
		s.Require().NoErrorf(putErr, "validate-only %v: a refusal is an answer, not an error", validateOnly)
		s.Require().Lenf(out.Errors, 1, "validate-only %v", validateOnly)
		s.Equal("statuses.warmed-up", out.Errors[0].Path, "at the author's place")
		s.Contains(out.Errors[0].Message, "changes[0].luckier", "the message names the place inside the status")
		s.Nil(out.Entry)
	}
	_, err = s.world.Get(s.ctx, &worlddungeons.GetInput{WorldID: alice.WorldID, Key: storeMyCastle})
	s.ErrorIs(err, worlddungeons.ErrNotFound, "nothing was stored")
}

// TestTheShippedCastlesStatusPasses: the castle on the shelf, whose cook gives
// warmed-up and feasted, compiled through the same check (the shelf's own compile).
func (s *StoresSuite) TestTheShippedCastlesStatusPasses() {
	entry, err := s.registry(true).Get(s.ctx, alice, "the-mad-king")
	s.Require().NoError(err)
	s.Contains(string(entry.YAML), "apply_status: { status: warmed-up, to: speaker }", "precondition: the castle gives the status")
}

// kitchenGivingAStatusUntil is the author's kitchen giving warmed-up with the
// given `until` written on the line (untilYAML is the flow-map body, such as
// "rounds: 3").
func (s *StoresSuite) kitchenGivingAStatusUntil(key, untilYAML string) []byte {
	raw := s.kitchenGivingAStatus(key, "initiative")
	line := []byte("apply_status: { status: warmed-up, to: speaker }")
	s.Require().Equal(1, bytes.Count(raw, line), "precondition: the kitchen line gives the status once")
	return bytes.Replace(raw, line, []byte("apply_status: { status: warmed-up, to: speaker, until: { "+untilYAML+" } }"), 1)
}

// putRefusal validates raw and returns the one refusal it must carry; nothing
// is stored.
func (s *StoresSuite) putRefusal(raw []byte) dungeons.FieldError {
	out, err := s.registry(true).Put(s.ctx, &dungeons.PutInput{Caller: alice, Key: storeMyCastle, YAML: raw, ValidateOnly: true})
	s.Require().NoError(err, "a refusal is an answer, not an error")
	s.Require().Len(out.Errors, 1)
	s.Nil(out.Entry)
	_, getErr := s.world.Get(s.ctx, &worlddungeons.GetInput{WorldID: alice.WorldID, Key: storeMyCastle})
	s.ErrorIs(getErr, worlddungeons.ErrNotFound, "nothing was stored")
	return out.Errors[0]
}

func (s *StoresSuite) TestPutRefusesAStatusGivenForNoRounds() {
	got := s.putRefusal(s.kitchenGivingAStatusUntil(storeMyCastle, "rounds: 0"))
	s.True(strings.HasSuffix(got.Path, ".apply_status.until.rounds"), "path %q", got.Path)
	s.Contains(got.Message, "rounds must be a whole number of one or more, at most 1000, such as 3")
}

func (s *StoresSuite) TestPutRefusesAnUntilThatIsNotRounds() {
	got := s.putRefusal(s.kitchenGivingAStatusUntil(storeMyCastle, "turns: 2"))
	s.True(strings.HasSuffix(got.Path, ".apply_status.until.turns"), "path %q", got.Path)
	s.True(strings.HasPrefix(got.Message, "until takes `rounds`, not `turns`"), "message %q", got.Message)
}

func (s *StoresSuite) TestPutAcceptsAStatusGivenForThreeRounds() {
	out, err := s.registry(true).Put(s.ctx, &dungeons.PutInput{
		Caller: alice, Key: storeMyCastle, YAML: s.kitchenGivingAStatusUntil(storeMyCastle, "rounds: 3"), ValidateOnly: true,
	})
	s.Require().NoError(err)
	s.Empty(out.Errors)
}

// TestTheShippedCastlesPrayerPasses: the castle on the shelf (compiled by the
// shelf's own compile when read) gives the status prayed at the altar.
func (s *StoresSuite) TestTheShippedCastlesPrayerPasses() {
	entry, err := s.registry(true).Get(s.ctx, alice, "the-mad-king")
	s.Require().NoError(err, "the shipped castle compiles")
	s.Contains(string(entry.YAML), "apply_status: { status: prayed, to: speaker }", "the altar's prayer gives the status")
}
