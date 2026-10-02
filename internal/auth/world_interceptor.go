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

var compositionWorldMethods = map[string]struct{}{
	"/api.composition.v1alpha1.CompositionService/CreateComposition": {},
	"/api.composition.v1alpha1.CompositionService/GetComposition":    {},
	"/api.composition.v1alpha1.CompositionService/ListCompositions":  {},
	"/api.composition.v1alpha1.CompositionService/DeleteComposition": {},
}

// UnaryWorldContextInterceptor installs trusted world context only for the four
// composition unary methods.
func UnaryWorldContextInterceptor(resolver WorldResolver) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if _, ok := compositionWorldMethods[info.FullMethod]; !ok {
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

// optionalGuildSelector reports the canonical guild selector a request
// carries, if any. Absence is not an error so a caller with a configured
// default can tell "no selector" apart from "an unusable selector"; a present
// but non-canonical or repeated value is always refused.
func optionalGuildSelector(ctx context.Context) (string, bool, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false, nil
	}
	values := md.Get(guildSelectorHeader)
	if len(values) == 0 {
		return "", false, nil
	}
	if len(values) != 1 || !isCanonicalUint64(values[0]) {
		return "", false, status.Error(codes.InvalidArgument, "guild selector must be one canonical non-zero uint64")
	}
	return values[0], true, nil
}

func guildSelector(ctx context.Context) (string, error) {
	selector, present, err := optionalGuildSelector(ctx)
	if err != nil {
		return "", err
	}
	if !present {
		return "", status.Error(codes.FailedPrecondition, "guild selector is required")
	}
	return selector, nil
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
