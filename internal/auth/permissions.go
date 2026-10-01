package auth

import "fmt"

// Permissions describes game capabilities, not Discord's permission bitset.
type Permissions uint8

const (
	PermissionPlay Permissions = 1 << iota
	PermissionBuild
	PermissionAdmin
)

// Allows reports whether every required capability is present.
func (p Permissions) Allows(required Permissions) bool {
	return required != 0 && p&required == required
}

// WorldRoleConfig is server-controlled configuration for one Discord guild.
// Discord manages membership of the configured roles; the game maps them to capabilities.
type WorldRoleConfig struct {
	WorldID       string
	AdminRoleID   string
	BuilderRoleID string
	PlayerRoleID  string
}

// EvaluatePermissionsInput combines configuration with a verified member's roles.
// AssignedRoleIDs must come from the membership verifier, never a request body.
type EvaluatePermissionsInput struct {
	Config          WorldRoleConfig
	WorldID         string
	AssignedRoleIDs []string
}

// EvaluatePermissionsOutput contains only derived game capabilities.
type EvaluatePermissionsOutput struct {
	Permissions Permissions
}

// EvaluatePermissions grants cumulative capabilities for a configured world.
// No matching role is a valid zero-capability result, not an implicit player grant.
func EvaluatePermissions(input *EvaluatePermissionsInput) (*EvaluatePermissionsOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("permission evaluation input is required")
	}
	config := input.Config
	if !isCanonicalUint64(config.WorldID) ||
		!isCanonicalUint64(config.AdminRoleID) ||
		!isCanonicalUint64(config.BuilderRoleID) ||
		!isCanonicalUint64(config.PlayerRoleID) {
		return nil, fmt.Errorf("world and all configured role IDs must be canonical non-zero uint64 values")
	}
	if input.WorldID != config.WorldID {
		return nil, fmt.Errorf("role configuration does not match the verified world")
	}

	var permissions Permissions
	for _, roleID := range input.AssignedRoleIDs {
		// Independent matches intentionally allow one Discord role to be
		// configured at multiple levels; it receives the highest grant.
		if roleID == config.AdminRoleID {
			permissions |= PermissionAdmin | PermissionBuild | PermissionPlay
		}
		if roleID == config.BuilderRoleID {
			permissions |= PermissionBuild | PermissionPlay
		}
		if roleID == config.PlayerRoleID {
			permissions |= PermissionPlay
		}
	}
	return &EvaluatePermissionsOutput{Permissions: permissions}, nil
}
