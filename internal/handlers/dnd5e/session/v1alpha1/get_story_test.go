package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestGetStory_Unauthenticated_Errors(t *testing.T) {
	h := &Handler{}
	_, err := h.GetStory(context.Background(), &sessionpb.GetStoryRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestGetStory_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Story(gomock.Any(), &sdk.StoryInput{Session: "sess-1", Member: "char-1", FromSeq: 5}).Return(
		[]sdk.StoryEntry{{Seq: 5}, {Seq: 6}}, nil,
	)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.GetStory(ctx, &sessionpb.GetStoryRequest{Session: "sess-1", Member: "char-1", FromSeq: 5})
	require.NoError(t, err)
	require.Len(t, resp.GetEntries(), 2)
}

func TestGetStory_Trimmed_ReturnsOutOfRange(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Story(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrStoryTrimmed)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetStory(ctx, &sessionpb.GetStoryRequest{Session: "sess-1", Member: "char-1", FromSeq: 1})
	requireCode(t, err, codes.OutOfRange)
}
