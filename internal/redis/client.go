// Package redis provides a wrapper around the go-redis client library
// for improved testing and abstraction.
package redis

import (
	"crypto/tls"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// Options configures Redis client behavior
// Naming considerations:
// - "Options" is simple (redis.Options might conflict though)
// - "ClientOptions" is more explicit (go-services style)
// - "Config" or "ClientConfig" emphasizes it's configuration
type Options struct {
	PoolSize        int
	MinIdleConns    int
	ConnMaxIdleTime time.Duration
	MaxRetries      int
	UseTLS          bool
	ReadOnly        bool // For cluster mode routing
}

// NewClient creates a Redis client for a single instance
// Naming considerations:
// - "NewClient" is standard Go factory pattern
// - "NewRedisClient" would be redundant in redis package
// - "Connect" implies immediate connection (Redis is lazy)
func NewClient(endpoint string, opts *Options) (Client, error) {
	if endpoint == "" {
		return nil, errors.New("redis: endpoint is required")
	}

	if opts == nil {
		opts = &Options{}
	}

	redisOpts := &redis.Options{
		Addr:            endpoint,
		MinIdleConns:    opts.MinIdleConns,
		PoolSize:        opts.PoolSize,
		ConnMaxIdleTime: opts.ConnMaxIdleTime,
		MaxRetries:      opts.MaxRetries,
	}

	if opts.UseTLS {
		redisOpts.TLSConfig = &tls.Config{
			InsecureSkipVerify: true, // #nosec G402 // For self-signed certs #nosec G402
		}
	}

	return redis.NewClient(redisOpts), nil
}

// NewClusterClient creates a Redis client for cluster mode
// Naming considerations:
// - Matches redis.NewClusterClient naming
// - Clear about what it creates
func NewClusterClient(endpoints []string, opts *Options) (Client, error) {
	if len(endpoints) == 0 {
		return nil, errors.New("redis: at least one endpoint is required")
	}

	if opts == nil {
		opts = &Options{}
	}

	clusterOpts := &redis.ClusterOptions{
		Addrs:        endpoints,
		MinIdleConns: opts.MinIdleConns,
		PoolSize:     opts.PoolSize,
		MaxRetries:   opts.MaxRetries,
		ReadOnly:     opts.ReadOnly,
	}

	if opts.UseTLS {
		clusterOpts.TLSConfig = &tls.Config{
			InsecureSkipVerify: true, // #nosec G402
		}
	}

	return redis.NewClusterClient(clusterOpts), nil
}

// NewFailoverClient creates a Redis client with Sentinel support
// Naming considerations:
// - Follows pattern of other factories
// - "Sentinel" vs "Failover" - using Redis terminology
func NewFailoverClient(masterName string, sentinelAddrs []string, opts *Options) (Client, error) {
	if masterName == "" {
		return nil, errors.New("redis: master name is required")
	}
	if len(sentinelAddrs) == 0 {
		return nil, errors.New("redis: at least one sentinel address is required")
	}

	if opts == nil {
		opts = &Options{}
	}

	failoverOpts := &redis.FailoverOptions{
		MasterName:    masterName,
		SentinelAddrs: sentinelAddrs,
		MinIdleConns:  opts.MinIdleConns,
		PoolSize:      opts.PoolSize,
		MaxRetries:    opts.MaxRetries,
	}

	if opts.UseTLS {
		failoverOpts.TLSConfig = &tls.Config{
			InsecureSkipVerify: true, // #nosec G402
		}
	}

	return redis.NewFailoverClient(failoverOpts), nil
}
