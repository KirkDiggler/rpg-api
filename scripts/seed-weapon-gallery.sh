#!/bin/bash
set -euo pipefail

REPO_ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
API_ADDRESS="${RPG_API_ADDRESS:-localhost:8080}"
REDIS_CONTAINER="${RPG_REDIS_CONTAINER:-rpg-dev-redis-1}"

resolve_redis_address() {
	if [ "${RPG_REDIS_ADDRESS+x}" = x ]; then
		if [ -z "$RPG_REDIS_ADDRESS" ]; then
			echo 'seed-weapon-gallery: RPG_REDIS_ADDRESS must be non-empty when set' >&2
			exit 1
		fi
		printf '%s\n' "$RPG_REDIS_ADDRESS"
		return 0
	fi

	local ip
	if ! ip="$(docker inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$REDIS_CONTAINER" 2>/dev/null)" || [ -z "$ip" ]; then
		echo "seed-weapon-gallery: could not resolve Redis from Docker container $REDIS_CONTAINER" >&2
		echo 'seed-weapon-gallery: Set RPG_REDIS_ADDRESS explicitly or ensure Docker and the container are available.' >&2
		exit 1
	fi
	printf '%s:6379\n' "$ip"
}

REDIS_ADDRESS="$(resolve_redis_address)"

printf 'seed-weapon-gallery: API=%s Redis=%s\n' "$API_ADDRESS" "$REDIS_ADDRESS"
(
	cd "$REPO_ROOT"
	exec go run ./cmd/sandboxseed --fixture=weapon-gallery --address="$API_ADDRESS" --redis-address="$REDIS_ADDRESS"
)
