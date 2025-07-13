# Mocks

This directory contains generated mocks for testing.

## Structure

```
mocks/
├── proto/          # Client mocks from generated proto code (for Discord bot)
├── services/       # Service interface mocks (for handler tests)
└── repositories/   # Repository interface mocks (for service tests)
```

## Generating Mocks

### Proto client mocks (for Discord bot to use):
```bash
make proto-mocks
```

### All mocks (uses go generate):
```bash
make mocks
```

## Usage

```go
// In Discord bot tests
import "github.com/KirkDiggler/rpg-api/mocks/proto"

mockClient := protomocks.NewMockCharacterAPIClient(ctrl)
mockClient.EXPECT().CreateDraft(gomock.Any(), gomock.Any()).Return(&pb.CreateDraftResponse{...}, nil)
```
