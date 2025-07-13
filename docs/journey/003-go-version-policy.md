# Journey: Go Version Policy Decision

**Date**: January 13, 2025  
**Context**: Setting up gRPC server infrastructure

## The Decision Point

While setting up the gRPC server, we noticed Go 1.24 was installed (a development version). This sparked an important discussion: should we use the latest Go version or play it safe with an older, stable version?

## The Story

When building a real-time API for tabletop RPG sessions, performance matters. Players expect instant responses when they roll dice, cast spells, or move on the battlefield. Any latency breaks immersion.

During early development, we faced a choice: play it safe with an older, "stable" Go version, or embrace the latest? We chose latest. Here's why:

### Performance is Non-Negotiable

- **Every millisecond counts**: In combat with 6 players, each action triggers multiple calculations. Go's runtime improvements compound across thousands of concurrent operations.
- **Memory efficiency**: Recent Go versions have dramatically improved memory allocation patterns. When tracking game state for multiple sessions, efficient memory usage directly impacts how many games we can host.
- **Better concurrency**: Go 1.24's scheduler improvements mean smoother handling of concurrent gRPC streams - critical when broadcasting updates to all players simultaneously.

### Benchmarking Truth

We plan to benchmark extensively. Using an older Go version would give us outdated baseline metrics. When we optimize, we want to know we're optimizing against the best the Go team has achieved, not last year's runtime.

### The Greenfield Advantage

- **No technical debt**: We're not maintaining legacy code that requires older Go versions.
- **No migration costs**: Starting with the latest means never needing a "upgrade Go version" sprint.
- **Future-proof**: By staying current from day one, we establish a culture of continuous improvement.

## The Outcome

We decided to always use the latest Go version. This means:
- Contributors need the latest Go version
- CI/CD uses the latest Go version
- We upgrade promptly when new versions release
- Performance benchmarks always reflect current Go capabilities

## Lessons Learned

1. **Document the "why"**: This decision could seem arbitrary to future developers without context
2. **Performance decisions compound**: Starting with the best foundation pays dividends as the system scales
3. **Greenfield is an opportunity**: Without legacy constraints, we can make optimal choices from day one

This isn't about using new features for their own sake. It's about building the most performant foundation possible for real-time tabletop gaming.
