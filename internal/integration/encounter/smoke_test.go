// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package encounter_integration provides API-level integration tests.
package encounter_integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/metadata"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
)

// SmokeTestSuite validates the integration test harness with minimal tests.
type SmokeTestSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	server *harness.TestServer
}

func TestSmokeTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(SmokeTestSuite))
}

func (s *SmokeTestSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)

	var err error
	s.server, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")

	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  Smoke Test: Integration Harness Ready                           ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
}

func (s *SmokeTestSuite) TearDownSuite() {
	if s.server != nil {
		s.server.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *SmokeTestSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// TestCharacterDraftLifecycle validates the character draft API works through gRPC.
func (s *SmokeTestSuite) TestCharacterDraftLifecycle() {
	s.T().Log("Testing character draft lifecycle via gRPC...")

	ctx := s.authCtx("test-player-smoke-1")

	// Create a draft
	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err, "CreateDraft should succeed")
	s.Require().NotNil(createResp.GetDraft(), "draft should not be nil")
	s.Require().NotEmpty(createResp.GetDraft().GetId(), "draft ID should not be empty")

	draftID := createResp.GetDraft().GetId()
	s.T().Logf("  ✓ Created draft: %s", draftID)

	// Get the draft
	getResp, err := s.server.CharacterClient.GetDraft(ctx, &dnd5ev1alpha1.GetDraftRequest{
		DraftId: draftID,
	})
	s.Require().NoError(err, "GetDraft should succeed")
	s.Require().NotNil(getResp.GetDraft(), "draft should be returned")
	s.Assert().Equal(draftID, getResp.GetDraft().GetId(), "draft IDs should match")
	s.T().Log("  ✓ Retrieved draft")

	// List drafts
	listResp, err := s.server.CharacterClient.ListDrafts(ctx, &dnd5ev1alpha1.ListDraftsRequest{})
	s.Require().NoError(err, "ListDrafts should succeed")
	s.Assert().GreaterOrEqual(len(listResp.GetDrafts()), 1, "should have at least 1 draft")
	s.T().Logf("  ✓ Listed drafts: %d total", len(listResp.GetDrafts()))

	// Note: DeleteDraft is not implemented yet, skip that step
	// _, err = s.server.CharacterClient.DeleteDraft(ctx, &dnd5ev1alpha1.DeleteDraftRequest{
	// 	DraftId: draftID,
	// })

	s.T().Log("✓ Character draft lifecycle test passed")
}

// TestListRaces validates we can fetch reference data.
func (s *SmokeTestSuite) TestListRaces() {
	s.T().Log("Testing race listing via gRPC...")

	ctx := s.authCtx("test-player-smoke-2")

	resp, err := s.server.CharacterClient.ListRaces(ctx, &dnd5ev1alpha1.ListRacesRequest{})
	s.Require().NoError(err, "ListRaces should succeed")
	s.Assert().NotEmpty(resp.GetRaces(), "should return races")
	s.T().Logf("  ✓ Listed %d races", len(resp.GetRaces()))

	// Verify Human is in the list
	foundHuman := false
	for _, race := range resp.GetRaces() {
		if race.GetRaceId() == dnd5ev1alpha1.Race_RACE_HUMAN {
			foundHuman = true
			s.T().Logf("  ✓ Found Human race: %s", race.GetName())
			break
		}
	}
	s.Assert().True(foundHuman, "Human race should be in the list")
}

// TestListClasses validates we can fetch class data.
func (s *SmokeTestSuite) TestListClasses() {
	s.T().Log("Testing class listing via gRPC...")

	ctx := s.authCtx("test-player-smoke-3")

	resp, err := s.server.CharacterClient.ListClasses(ctx, &dnd5ev1alpha1.ListClassesRequest{})
	s.Require().NoError(err, "ListClasses should succeed")
	s.Assert().NotEmpty(resp.GetClasses(), "should return classes")
	s.T().Logf("  ✓ Listed %d classes", len(resp.GetClasses()))

	// Verify Barbarian is in the list
	foundBarbarian := false
	for _, class := range resp.GetClasses() {
		if class.GetClassId() == dnd5ev1alpha1.Class_CLASS_BARBARIAN {
			foundBarbarian = true
			s.T().Logf("  ✓ Found Barbarian class: %s", class.GetName())
			break
		}
	}
	s.Assert().True(foundBarbarian, "Barbarian class should be in the list")
}

// TestRedisIntegration validates Redis operations work through the harness.
func (s *SmokeTestSuite) TestRedisIntegration() {
	s.T().Log("Testing Redis integration...")

	// Create a draft (writes to Redis)
	ctx := s.authCtx("test-player-redis")
	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err)
	draftID := createResp.GetDraft().GetId()
	s.T().Logf("  ✓ Created draft in Redis: %s", draftID)

	// Flush Redis
	err = s.server.FlushRedis(s.ctx)
	s.Require().NoError(err, "FlushRedis should succeed")
	s.T().Log("  ✓ Flushed Redis")

	// Draft should be gone
	_, err = s.server.CharacterClient.GetDraft(ctx, &dnd5ev1alpha1.GetDraftRequest{
		DraftId: draftID,
	})
	s.Assert().Error(err, "draft should not exist after flush")
	s.T().Log("  ✓ Verified draft was deleted by flush")
}
