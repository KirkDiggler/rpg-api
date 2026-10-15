package authoringv1alpha1_test

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
)

// The sentences an author reads; the wire carries them verbatim (S7-Q2).
const (
	referenceSentence   = "that key is a reference; save it under your own key"
	readOnlySentence    = "that key is a reference; references cannot be renamed or deleted"
	otherMemberSentence = "that key belongs to another member of this world"
	keyExistsSentence   = "new key already names one of your encounters"
)

func (s *HandlerSuite) requireStatus(err error, code codes.Code, message string) {
	s.T().Helper()
	st, ok := status.FromError(err)
	s.Require().True(ok, "not a status error: %v", err)
	s.Equal(code, st.Code(), st.Message())
	s.Equal(message, st.Message())
}

func (s *HandlerSuite) TestPutDungeon_UnderAReferenceIsPermissionDenied() {
	for _, validateOnly := range []bool{true, false} {
		s.registry.EXPECT().
			Put(gomock.Any(), &dungeons.PutInput{Caller: testCaller, Key: "castle-kitchen", YAML: []byte("x"), ValidateOnly: validateOnly}).
			Return(nil, dungeons.ErrReferenceKey)
		_, err := s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{Key: "castle-kitchen", Yaml: "x", ValidateOnly: validateOnly})
		s.requireStatus(err, codes.PermissionDenied, referenceSentence)
	}
}

// TestAStoreErrorKeepsItsCode: a write that lost its transaction too many
// times is the world store's Aborted, and reaches the wire as Aborted rather
// than Internal (apierr.ToGRPCError, as the world and dice handlers map it).
func (s *HandlerSuite) TestAStoreErrorKeepsItsCode() {
	aborted := fmt.Errorf("dungeon %q in world %q: %w", "my-castle", "w1",
		apierr.Abortedf("world %q key %q changed concurrently %d times", "w1", "my-castle", 4))
	s.registry.EXPECT().Put(gomock.Any(), gomock.Any()).Return(nil, aborted)
	_, err := s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{Key: "my-castle", Yaml: "x"})
	s.requireCode(err, codes.Aborted)

	s.registry.EXPECT().Delete(gomock.Any(), testCaller, "my-castle").Return(errors.New("disk on fire"))
	_, err = s.handler.DeleteDungeon(s.ctx, &authoringpb.DeleteDungeonRequest{Key: "my-castle"})
	s.requireCode(err, codes.Internal)
}

func (s *HandlerSuite) TestPutDungeon_UnderAnotherOwnersKeyIsPermissionDenied() {
	s.registry.EXPECT().Put(gomock.Any(), gomock.Any()).Return(nil, dungeons.ErrOwnedByOther)
	_, err := s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{Key: "my-castle", Yaml: "x"})
	s.requireStatus(err, codes.PermissionDenied, otherMemberSentence)
}

// TestADungeonRPCWithoutAWorldIsFailedPrecondition: a signed-in player with
// no trusted world is refused on every dungeon RPC, before the registry.
func (s *HandlerSuite) TestADungeonRPCWithoutAWorldIsFailedPrecondition() {
	ctx := auth.WithPlayerID(context.Background(), "alice")
	calls := map[string]func() error{
		"PutDungeon": func() error {
			_, err := s.handler.PutDungeon(ctx, &authoringpb.PutDungeonRequest{Key: "my-castle", Yaml: "x"})
			return err
		},
		"GetDungeon": func() error {
			_, err := s.handler.GetDungeon(ctx, &authoringpb.GetDungeonRequest{Key: "my-castle"})
			return err
		},
		"ListDungeons": func() error {
			_, err := s.handler.ListDungeons(ctx, &authoringpb.ListDungeonsRequest{})
			return err
		},
		"DeleteDungeon": func() error {
			_, err := s.handler.DeleteDungeon(ctx, &authoringpb.DeleteDungeonRequest{Key: "my-castle"})
			return err
		},
		"RenameDungeon": func() error {
			_, err := s.handler.RenameDungeon(ctx, &authoringpb.RenameDungeonRequest{Key: "my-castle", NewKey: "my-keep"})
			return err
		},
	}
	for name, call := range calls {
		s.Run(name, func() {
			s.requireStatus(call(), codes.FailedPrecondition, "trusted world context is required")
		})
	}
}

func (s *HandlerSuite) TestGetDungeon_PassesTheCaller() {
	s.registry.EXPECT().Get(gomock.Any(), testCaller, "my-castle").
		Return(&dungeons.Entry{Key: "my-castle", YAML: []byte("bytes"), Source: dungeons.SourceWorld, Owner: "bob"}, nil)
	resp, err := s.handler.GetDungeon(s.ctx, &authoringpb.GetDungeonRequest{Key: "my-castle"})
	s.Require().NoError(err)
	s.Equal("bytes", resp.GetYaml())
}

func (s *HandlerSuite) TestGetDungeon_EmptyKeyIsStillInvalidArgument() {
	_, err := s.handler.GetDungeon(s.ctx, &authoringpb.GetDungeonRequest{})
	s.requireStatus(err, codes.InvalidArgument, "key is required")
}

func (s *HandlerSuite) TestListDungeons_ReturnsBothSourcesInOrder() {
	s.registry.EXPECT().List(gomock.Any(), testCaller).Return([]dungeons.Summary{
		{Key: "bob-keep", Name: "Bob's Keep", Owner: "bob", Source: dungeons.SourceWorld},
		{Key: "my-castle", Name: "My Castle", Owner: "alice", Source: dungeons.SourceWorld},
		{Key: "castle-kitchen", Name: "The Castle Kitchen", Source: dungeons.SourceShipped},
	}, nil)

	resp, err := s.handler.ListDungeons(s.ctx, &authoringpb.ListDungeonsRequest{})
	s.Require().NoError(err)
	s.Require().Len(resp.GetDungeons(), 3)
	want := []*lobbyv1alpha1.DungeonSummary{
		{Key: "bob-keep", Name: "Bob's Keep", Owner: "bob", Source: lobbyv1alpha1.DungeonSource_DUNGEON_SOURCE_WORLD},
		{Key: "my-castle", Name: "My Castle", Owner: "alice", Source: lobbyv1alpha1.DungeonSource_DUNGEON_SOURCE_WORLD},
		{Key: "castle-kitchen", Name: "The Castle Kitchen", Source: lobbyv1alpha1.DungeonSource_DUNGEON_SOURCE_SHIPPED},
	}
	for i, d := range resp.GetDungeons() {
		s.Equal(want[i].GetKey(), d.GetKey())
		s.Equal(want[i].GetName(), d.GetName())
		s.Equal(want[i].GetOwner(), d.GetOwner())
		s.Equal(want[i].GetSource(), d.GetSource())
	}
}

func (s *HandlerSuite) TestRenameDungeon_Table() {
	cases := []struct {
		name    string
		req     *authoringpb.RenameDungeonRequest
		regErr  error
		callReg bool
		code    codes.Code
		message string
	}{
		{"ok", &authoringpb.RenameDungeonRequest{Key: "my-castle", NewKey: "my-keep"}, nil, true, codes.OK, ""},
		{"own new key", &authoringpb.RenameDungeonRequest{Key: "my-castle", NewKey: "my-keep"}, dungeons.ErrKeyExists, true, codes.AlreadyExists, keyExistsSentence},
		{"another member's", &authoringpb.RenameDungeonRequest{Key: "bob-keep", NewKey: "my-keep"}, dungeons.ErrOwnedByOther, true, codes.PermissionDenied, otherMemberSentence},
		{"shipped new key", &authoringpb.RenameDungeonRequest{Key: "my-castle", NewKey: "castle-kitchen"}, dungeons.ErrReferenceKey, true, codes.PermissionDenied, referenceSentence},
		{"shipped source", &authoringpb.RenameDungeonRequest{Key: "castle-kitchen", NewKey: "my-keep"}, dungeons.ErrReferenceReadOnly, true, codes.PermissionDenied, readOnlySentence},
		{"absent", &authoringpb.RenameDungeonRequest{Key: "absent", NewKey: "my-keep"}, dungeons.ErrNotFound, true, codes.NotFound, "dungeon not found"},
		{"invalid new key", &authoringpb.RenameDungeonRequest{Key: "my-castle", NewKey: "My Keep"}, dungeons.ErrInvalidKey, true, codes.InvalidArgument, "key must match [a-z0-9-]+"},
		{"empty key", &authoringpb.RenameDungeonRequest{NewKey: "my-keep"}, nil, false, codes.InvalidArgument, "key is required"},
		{"empty new key", &authoringpb.RenameDungeonRequest{Key: "my-castle"}, nil, false, codes.InvalidArgument, "new_key is required"},
		{"authoring off", &authoringpb.RenameDungeonRequest{Key: "my-castle", NewKey: "my-keep"}, dungeons.ErrAuthoringDisabled, true, codes.FailedPrecondition, "authoring is disabled"},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			if tc.callReg {
				s.registry.EXPECT().
					Rename(gomock.Any(), &dungeons.RenameInput{Caller: testCaller, Key: tc.req.GetKey(), NewKey: tc.req.GetNewKey()}).
					Return(tc.regErr)
			}
			_, err := s.handler.RenameDungeon(s.ctx, tc.req)
			if tc.code == codes.OK {
				s.Require().NoError(err)
				return
			}
			s.requireStatus(err, tc.code, tc.message)
		})
	}
}

func (s *HandlerSuite) TestDeleteDungeon_Table() {
	cases := []struct {
		name    string
		key     string
		regErr  error
		code    codes.Code
		message string
	}{
		{"ok", "my-castle", nil, codes.OK, ""},
		{"another member's", "bob-keep", dungeons.ErrOwnedByOther, codes.PermissionDenied, otherMemberSentence},
		{"a reference", "castle-kitchen", dungeons.ErrReferenceReadOnly, codes.PermissionDenied, readOnlySentence},
		{"absent", "absent", dungeons.ErrNotFound, codes.NotFound, "dungeon not found"},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			s.registry.EXPECT().Delete(gomock.Any(), testCaller, tc.key).Return(tc.regErr)
			_, err := s.handler.DeleteDungeon(s.ctx, &authoringpb.DeleteDungeonRequest{Key: tc.key})
			if tc.code == codes.OK {
				s.Require().NoError(err)
				return
			}
			s.requireStatus(err, tc.code, tc.message)
		})
	}

	_, err := s.handler.DeleteDungeon(s.ctx, &authoringpb.DeleteDungeonRequest{})
	s.requireStatus(err, codes.InvalidArgument, "key is required")
}
