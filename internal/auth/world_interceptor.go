package auth

import (
	"context"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

const guildSelectorHeader = "x-rpg-guild-id"

// worldScopedMethods are the unary methods whose handlers read the caller's
// world: the composition verbs, and the dungeon verbs whose store is per
// world (rpg-project#585, design R21). Under world-access enforcement the
// role interceptor installs the world for every classified game RPC; without
// it, these are the methods that still get one.
var worldScopedMethods = map[string]struct{}{
	"/api.composition.v1alpha1.CompositionService/CreateComposition": {},
	"/api.composition.v1alpha1.CompositionService/GetComposition":    {},
	"/api.composition.v1alpha1.CompositionService/ListCompositions":  {},
	"/api.composition.v1alpha1.CompositionService/DeleteComposition": {},
	"/dnd5e.api.authoring.v1alpha1.AuthoringService/PutDungeon":      {},
	"/dnd5e.api.authoring.v1alpha1.AuthoringService/GetDungeon":      {},
	"/dnd5e.api.authoring.v1alpha1.AuthoringService/ListDungeons":    {},
	"/dnd5e.api.authoring.v1alpha1.AuthoringService/DeleteDungeon":   {},
	"/dnd5e.api.authoring.v1alpha1.AuthoringService/RenameDungeon":   {},
	"/dnd5e.api.lobby.v1alpha1.LobbyService/ListDungeons":            {},
	"/dnd5e.api.lobby.v1alpha1.LobbyService/StartEncounter":          {},
}

// UnaryWorldContextInterceptor installs trusted world context only for the
// worldScopedMethods.
func UnaryWorldContextInterceptor(resolver WorldResolver) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if _, ok := worldScopedMethods[info.FullMethod]; !ok {
			return handler(withoutRequestAuth(ctx), req)
		}
		if resolver == nil {
			return nil, status.Error(codes.Internal, "world resolver is not configured")
		}

		input := &ResolveWorldInput{}
		requestAuth, hasRequestAuth := getRequestAuth(ctx)
		if !hasRequestAuth || requestAuth.scheme != authSchemeDev {
			guildID, err := guildSelector(ctx)
			if err != nil {
				return nil, err
			}
			input.GuildID = guildID
		}
		output, err := resolver.Resolve(ctx, input)
		if err != nil {
			return nil, err
		}
		if output == nil || output.WorldID == "" {
			return nil, status.Error(codes.Internal, "world resolver returned no world")
		}

		handlerContext := worldcontext.With(withoutRequestAuth(ctx), worldcontext.Value{WorldID: output.WorldID})
		return handler(handlerContext, req)
	}
}

func guildSelector(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.FailedPrecondition, "guild selector is required")
	}
	values := md.Get(guildSelectorHeader)
	if len(values) == 0 {
		return "", status.Error(codes.FailedPrecondition, "guild selector is required")
	}
	if len(values) != 1 || !isCanonicalUint64(values[0]) {
		return "", status.Error(codes.InvalidArgument, "guild selector must be one canonical non-zero uint64")
	}
	return values[0], nil
}

func isCanonicalUint64(value string) bool {
	if value == "" || value[0] < '1' || value[0] > '9' {
		return false
	}
	for i := 1; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}
