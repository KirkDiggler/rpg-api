package lobby_test

import (
	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
)

// TestListDungeons_ReturnsEmbeddedContentSpecs proves ListDungeons is wired
// end to end through the real orchestrator (s.handler, built in SetupTest
// against a real LoadContentRegistry() with no RPG_CONTENT_DIR override —
// just the embedded content/dungeons/*.yaml set) and, per design.md's
// post-approval correction, requires no special ungating: HandlerSuite
// never sets RPG_AUTHORING_ENABLED anywhere, and this RPC still works.
func (s *HandlerSuite) TestListDungeons_ReturnsEmbeddedContentSpecs() {
	resp, err := s.handler.ListDungeons(s.ctx, &lobbyv1alpha1.ListDungeonsRequest{})
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	byKey := make(map[string]string, len(resp.GetDungeons()))
	for _, d := range resp.GetDungeons() {
		byKey[d.GetKey()] = d.GetName()
	}
	s.Assert().Equal("The Tomb of the Captain", byKey["reference-tomb"])
	s.Assert().Equal("Fog Lab", byKey["fog-lab"])
}
