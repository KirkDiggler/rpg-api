package auth_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"

	compositionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/composition/v1alpha1"
	apipb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	lobbypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	presentationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/v1alpha1"
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	characterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	"github.com/KirkDiggler/rpg-api/internal/auth"
)

type PolicySuite struct{ suite.Suite }

func TestPolicySuite(t *testing.T) { suite.Run(t, new(PolicySuite)) }

func (s *PolicySuite) TestEveryGameRPCIsExplicitlyClassified() {
	for _, descriptor := range []*grpc.ServiceDesc{
		&compositionpb.CompositionService_ServiceDesc, &apipb.DiceService_ServiceDesc,
		&authoringpb.AuthoringService_ServiceDesc, &lobbypb.LobbyService_ServiceDesc,
		&presentationpb.SessionPresentationService_ServiceDesc, &sessionpb.SessionService_ServiceDesc,
		&characterpb.CharacterService_ServiceDesc, &characterv2pb.CharacterService_ServiceDesc,
	} {
		check := func(name string) {
			method := "/" + descriptor.ServiceName + "/" + name
			permission, known := auth.GameMethodPermission(method)
			s.True(known, method+" must have an explicit access policy")
			s.NotZero(permission, method)
		}
		for _, method := range descriptor.Methods {
			check(method.MethodName)
		}
		for _, stream := range descriptor.Streams {
			check(stream.StreamName)
		}
	}
}

func (s *PolicySuite) TestWritesRequireBuildAndRenderingReadsRequirePlay() {
	for method, want := range map[string]auth.Permissions{
		"/api.composition.v1alpha1.CompositionService/CreateComposition": auth.PermissionBuild,
		"/api.composition.v1alpha1.CompositionService/DeleteComposition": auth.PermissionBuild,
		"/api.composition.v1alpha1.CompositionService/GetComposition":    auth.PermissionPlay,
		"/dnd5e.api.authoring.v1alpha1.AuthoringService/PutDungeon":      auth.PermissionBuild,
		"/dnd5e.api.authoring.v1alpha1.AuthoringService/GetDungeon":      auth.PermissionBuild,
	} {
		permission, known := auth.GameMethodPermission(method)
		s.True(known)
		s.Equal(want, permission)
	}
	for _, method := range []string{"/new.Service/Play", "/dnd5e.api.session.v1alpha1.SessionService/NewVerb", "malformed"} {
		_, known := auth.GameMethodPermission(method)
		s.False(known, method)
	}
}
