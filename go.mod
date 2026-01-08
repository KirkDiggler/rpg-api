module github.com/KirkDiggler/rpg-api

go 1.24.1

require (
	github.com/KirkDiggler/rpg-api-protos/gen/go v0.0.0-20260107092240-e203fd19fa12
	github.com/KirkDiggler/rpg-toolkit/core v0.10.0
	github.com/KirkDiggler/rpg-toolkit/dice v0.3.2
	github.com/KirkDiggler/rpg-toolkit/events v0.6.2
	github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e v0.42.0
	github.com/KirkDiggler/rpg-toolkit/tools/environments v0.3.0
	github.com/KirkDiggler/rpg-toolkit/tools/spatial v0.4.0
	github.com/alicebob/miniredis/v2 v2.35.0
	github.com/google/uuid v1.6.0
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.2
	github.com/oklog/ulid/v2 v2.1.1
	github.com/redis/go-redis/v9 v9.11.0
	github.com/spf13/cobra v1.9.1
	github.com/stretchr/testify v1.10.0
	go.uber.org/mock v0.6.0
	google.golang.org/grpc v1.78.0
)

require (
	github.com/KirkDiggler/rpg-toolkit/game v0.1.0 // indirect
	github.com/KirkDiggler/rpg-toolkit/mechanics/resources v0.3.1 // indirect
	github.com/KirkDiggler/rpg-toolkit/rpgerr v0.1.2 // indirect
	github.com/KirkDiggler/rpg-toolkit/tools/selectables v0.1.2 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251029180050-ab9386a59fda // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// Local development - remove before committing
replace github.com/KirkDiggler/rpg-toolkit/tools/environments => ../rpg-toolkit/tools/environments
