---
name: add a handler/orchestrator method
description: Outside-in steps for adding a new gRPC RPC to rpg-api
updated: 2026-05-02
---

# How to add a new handler/orchestrator method

rpg-api uses outside-in development. Start at the handler (return `codes.Unimplemented`), define the service interface and Input/Output types, write tests with mocks, then implement.

## Step 1 — Update the proto (rpg-api-protos)

Add the RPC to the appropriate `.proto` file in `rpg-api-protos`. Follow proto3 conventions. Run `buf generate` and push to the `generated` branch.

Then pull the new compiled code into rpg-api:
```bash
GOPROXY=direct go get github.com/KirkDiggler/rpg-api-protos/gen/go@generated
go mod tidy
```

## Step 2 — Add the handler stub

In the appropriate handler file (e.g., `internal/handlers/dnd5e/v1alpha1/encounter/handler.go`):

```go
func (h *Handler) MyNewRPC(ctx context.Context, req *pb.MyNewRequest) (*pb.MyNewResponse, error) {
    return nil, status.Error(codes.Unimplemented, "not yet implemented")
}
```

Verify the server compiles and starts. This validates the proto definitions work.

## Step 3 — Define the service interface method and types

In `internal/orchestrators/<domain>/service.go`, add the method to the `Service` interface and define Input/Output types:

```go
// Service interface
MyNewMethod(ctx context.Context, input *MyNewMethodInput) (*MyNewMethodOutput, error)

// Input/Output types — ALWAYS define these, even for simple operations
type MyNewMethodInput struct {
    EncounterID string
    PlayerID    string
    // ... domain-typed fields only, no proto types
}

type MyNewMethodOutput struct {
    // ... domain-typed fields only, no proto types
}
```

Rules for Input/Output types:
- Use `string` IDs, not proto enums
- Use entity types, not proto types
- Use toolkit types where appropriate (these are domain types in this codebase)
- Never use `pb.` or `dnd5ev1alpha1.` in Input/Output types

## Step 4 — Regenerate mocks

```bash
cd internal/orchestrators/<domain>
go generate ./...
```

This regenerates `mock/mock_service.go` using gomock.

## Step 5 — Write handler tests

In a `*_test.go` file in the handler package:

```go
type HandlerTestSuite struct {
    suite.Suite
    mockService *encountermock.MockService
    handler     *Handler
}

func (s *HandlerTestSuite) SetupTest() {
    ctrl := gomock.NewController(s.T())
    s.mockService = encountermock.NewMockService(ctrl)
    s.handler = NewHandler(s.mockService, ...)
}

func (s *HandlerTestSuite) TestMyNewRPC_Success() {
    // Arrange
    s.mockService.EXPECT().
        MyNewMethod(gomock.Any(), &encounter.MyNewMethodInput{...}).
        Return(&encounter.MyNewMethodOutput{...}, nil)

    // Act
    resp, err := s.handler.MyNewRPC(context.Background(), &pb.MyNewRequest{...})

    // Assert
    s.NoError(err)
    s.NotNil(resp)
    // verify response fields
}
```

## Step 6 — Implement the handler fully

Replace the `Unimplemented` stub:

```go
func (h *Handler) MyNewRPC(ctx context.Context, req *pb.MyNewRequest) (*pb.MyNewResponse, error) {
    // 1. Validate input (proto-layer validation)
    if req.GetEncounterId() == "" {
        return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
    }

    // 2. Extract auth
    playerID := auth.PlayerIDFromContext(ctx)

    // 3. Build input — convert proto to domain
    input := &encounter.MyNewMethodInput{
        EncounterID: req.GetEncounterId(),
        PlayerID:    playerID,
        // ... convert proto fields to domain types
    }

    // 4. Call orchestrator
    output, err := h.service.MyNewMethod(ctx, input)
    if err != nil {
        return nil, apierr.ToGRPC(err)
    }

    // 5. Convert output — domain to proto
    return &pb.MyNewResponse{
        // ... convert output fields to proto types
    }, nil
}
```

## Step 7 — Add orchestrator method to orchestrator.go (or new file)

Add the method to the orchestrator struct:

```go
func (o *Orchestrator) MyNewMethod(ctx context.Context, input *MyNewMethodInput) (*MyNewMethodOutput, error) {
    if input == nil {
        return nil, apierr.InvalidArgument("input is required")
    }

    // Load what you need
    encounterOut, err := o.encounterRepo.Get(ctx, &encounterrepo.GetInput{EncounterID: input.EncounterID})
    if err != nil {
        return nil, fmt.Errorf("get encounter: %w", err)
    }

    // Call toolkit if needed
    // Persist changes
    // Emit events via o.eventProcessor.Process(ctx, ...)

    return &MyNewMethodOutput{...}, nil
}
```

## Step 8 — Run pre-commit

```bash
make pre-commit
```

Never skip this. Fix any lint or test failures before pushing.

## Key rules to remember

- No proto types in orchestrator Input/Output or implementation
- No game logic in the handler or orchestrator — if you need to check weapon properties, file an issue in rpg-toolkit
- Input/Output types on every function at every layer
- Never return `(nil, nil)`
- Context flows through all layers
