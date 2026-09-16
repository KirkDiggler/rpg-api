// Copyright (C) 2026 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package character_integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	"github.com/KirkDiggler/rpg-api/internal/sandboxseed"
)

// LevelUpClassesSuite runs the per-class fixture set against the real
// CharacterService and real Redis.
//
// This is the test that answers the question the set exists for: can every
// class be created from its own catalog entry, with no per-class code, and be
// standing at the level-2 threshold afterwards. A fake catalog could only prove
// the loop iterates.
type LevelUpClassesSuite struct {
	suite.Suite
	ctx     context.Context
	cancel  context.CancelFunc
	server  *harness.TestServer
	release func()
}

func TestLevelUpClassesSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(LevelUpClassesSuite))
}

func (s *LevelUpClassesSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Minute)
	s.release = sharedRedis.Lease()

	var err error
	s.server, err = harness.NewWithRedis(s.ctx, nil, sharedRedis.Addr)
	s.Require().NoError(err, "failed to create test server")
	s.Require().NoError(s.server.FlushRedis(s.ctx), "failed to flush redis")
}

func (s *LevelUpClassesSuite) TearDownTest() {
	if s.server != nil {
		s.server.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.release != nil {
		s.release()
	}
}

func (s *LevelUpClassesSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

func (s *LevelUpClassesSuite) listExactlyOne(identity string) *dnd5ev1alpha1.Character {
	s.T().Helper()
	response, err := s.server.CharacterClient.ListCharacters(
		s.authCtx(identity), &dnd5ev1alpha1.ListCharactersRequest{PageSize: 100})
	require.NoError(s.T(), err)
	require.Len(s.T(), response.GetCharacters(), 1, "identity %s", identity)
	return response.GetCharacters()[0]
}

// blockedClasses are the classes the generic path CANNOT create, each with the
// defect that blocks it.
//
// EMPTY, and that is the finding. It held "ranger" until rpg-toolkit#1781's
// last commit: getClassSubmissions built the fighting-style submission with the
// FIGHTER's choice id, its own comment reading "Would need mapping for other
// classes", so a ranger's style was recorded under ranger-fighting-style while
// the submission claimed fighter-fighting-style. The validator never saw it
// answered, Draft.IsClassComplete was false forever, and a ranger could not be
// created by any client with any choices. The fix routes the draft's choices
// through choices.SubmissionsFrom, so each submission carries the requirement's
// own id.
//
// This map is the record and the police. The assertions below fail in BOTH
// directions: a class that starts working must be removed from here — which is
// exactly how the ranger entry left — and a class that stops working is not
// absorbed silently. Nothing in rpg-api may be added here as a workaround;
// entries name rules-engine defects, and working around one in the API would be
// the "game logic in the API" smell this repo refuses.
var blockedClasses = map[string]string{}

// TestEveryClassIsCreatedFromItsOwnCatalogEntry is the whole claim. Every class
// the catalog offers is created by answering the choices that class itself
// declares, finalized, and left holding the experience for level 2.
//
// A class the generic path cannot finalize is named, never skipped: that is a
// finding about the engine's content, and the run continues so one defect
// cannot hide eleven answers.
func (s *LevelUpClassesSuite) TestEveryClassIsCreatedFromItsOwnCatalogEntry() {
	out, err := sandboxseed.SeedLevelUpClasses(s.ctx, &sandboxseed.SeedLevelUpClassesInput{
		Client: s.server.CharacterClient,
		Store:  s.server.CharacterRepo,
	})
	s.Require().NotNil(out, "a partial run still reports what it seeded")

	seeded := map[string]bool{}
	for _, class := range out.Classes {
		seeded[class.Class] = true
	}

	if len(blockedClasses) == 0 {
		s.Require().NoError(err, "no class is blocked, so every class must seed")
	} else {
		s.Require().Error(err,
			"every class seeded, so nothing is blocked any more: DELETE the entries from "+
				"blockedClasses above. This assertion is how a fixed engine defect gets "+
				"noticed instead of being carried forever.")
		for class, why := range blockedClasses {
			s.Contains(err.Error(), "level-up-"+class,
				"blockedClasses says %s is blocked by %q, but the run did not fail on it. "+
					"If that defect is fixed, delete the entry; if a different class failed, "+
					"that is a NEW finding and belongs in its own entry.", class, why)
			s.False(seeded[class], "%s is blocked and must not report as seeded", class)
		}
		s.Contains(err.Error(), fmt.Sprintf("%d of 12", len(blockedClasses)),
			"exactly %d class(es) must fail. A different count means a class outside "+
				"blockedClasses broke, and it must not hide behind the known ones.",
			len(blockedClasses))
	}

	s.Len(out.Classes, 12-len(blockedClasses),
		"every class that is not blocked must seed")

	for _, class := range out.Classes {
		s.Run(class.Class, func() {
			s.Require().NotEmpty(class.CharacterID)
			s.Equal("level-up-"+class.Class, class.Identity)
			s.NotEmpty(class.ChoiceIDs, "%s answered no choices at all", class.Class)

			character := s.listExactlyOne(class.Identity)
			s.Equal(int32(1), character.GetLevel(), "seeded at level 1, holding the experience for 2")
			s.Equal(int32(300), character.GetExperiencePoints())
			s.Equal(int32(2), character.GetEntitledLevel())
			s.Equal(int32(900), character.GetNextLevelThreshold())
		})
	}
}

// classesWithNoOfferableLevelTwo are the classes whose level 2 the engine
// REFUSES to describe, with the reason.
//
// This is not a bug and not a gap in this service. Druid and wizard both take
// their subclass at level 2, and a subclass is the one requirement the engine
// can ask for and cannot yet receive (rpg-toolkit#1767, named in the design's
// §9 as a known blocker gating level 3 for everyone else). The SDK refuses the
// whole level rather than handing back a form with a hole in it, which is the
// honest answer: an offered level whose real question is missing would be a
// screen that cannot be completed.
//
// Same both-directions property as blockedClasses: when #1767 lands these two
// start answering and the assertions below demand this map be emptied.
var classesWithNoOfferableLevelTwo = map[string]string{
	"druid":  "takes its Circle at level 2; subclass-as-a-choice is rpg-toolkit#1767",
	"wizard": "takes its Tradition at level 2; subclass-as-a-choice is rpg-toolkit#1767",
}

// TestEveryClassCanBeAskedForItsNextLevel walks the read half for all twelve:
// the level-up screen has to be able to describe level 2 for every class, not
// only the two the wave was designed around. A class whose level 2 asks nothing
// and grants nothing still answers, with its hit die.
func (s *LevelUpClassesSuite) TestEveryClassCanBeAskedForItsNextLevel() {
	// The error is deliberately not asserted here: which classes fail to be
	// CREATED is the other test's claim, and this one is about whether the
	// ones that exist can be asked what their next level brings.
	out, _ := sandboxseed.SeedLevelUpClasses(s.ctx, &sandboxseed.SeedLevelUpClassesInput{
		Client: s.server.CharacterClient,
		Store:  s.server.CharacterRepo,
	})
	s.Require().NotNil(out)
	s.Require().NotEmpty(out.Classes)

	for _, seeded := range out.Classes {
		s.Run(seeded.Class, func() {
			character := s.listExactlyOne(seeded.Identity)
			next, nextErr := s.server.CharacterClient.GetNextLevel(
				s.authCtx(seeded.Identity),
				&dnd5ev1alpha1.GetNextLevelRequest{CharacterId: character.GetId()},
			)
			if why, blocked := classesWithNoOfferableLevelTwo[seeded.Class]; blocked {
				require.Error(s.T(), nextErr,
					"%s answered, so its level 2 IS offerable now (%s): remove it from "+
						"classesWithNoOfferableLevelTwo", seeded.Class, why)
				s.Equal(codes.FailedPrecondition, status.Code(nextErr),
					"a level this build cannot describe is the character's situation, "+
						"not a malformed request")
				s.T().Logf("%s level 2: refused, %s", seeded.Class, why)
				return
			}

			require.NoError(s.T(), nextErr, "%s cannot be asked what level 2 brings", seeded.Class)
			s.Equal(int32(2), next.GetLevel(), "the level being taken, not the one held")
			s.Positive(next.GetHitDie(), "a class with no hit die cannot roll for hit points")

			// Report shape, not a pass/fail: what each class's level 2
			// actually asks and grants is the per-class answer the walk wants.
			ids := make([]string, 0, len(next.GetChoices()))
			for _, choice := range next.GetChoices() {
				ids = append(ids, choice.GetId())
			}
			features := make([]string, 0, len(next.GetFeatures()))
			for _, feature := range next.GetFeatures() {
				features = append(features, feature.GetName())
			}
			s.T().Logf("%s level 2: choices=%v features=%v hit_die=d%d",
				seeded.Class, ids, features, next.GetHitDie())
		})
	}
}
