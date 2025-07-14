package redis

import (
	"github.com/redis/go-redis/v9"
)

//go:generate mockgen -destination=mocks/redis.go -package=redismocks -source=interface.go

// Client wraps redis.UniversalClient to allow for easy mocking
// Naming considerations:
// - "Client" is simple and clear in the redis package context
// - "ClientWrapper" (go-services style) is more explicit about its purpose
// - "UniversalClient" matches redis terminology but might be confusing
type Client interface {
	redis.UniversalClient
}

// Pipeliner wraps redis.Pipeliner for batch operations
type Pipeliner interface {
	redis.Pipeliner
}
