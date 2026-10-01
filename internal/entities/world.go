package entities

// World is API-owned access configuration for one Discord server.
// Discord manages membership of the roles; no game session state is held here.
type World struct {
	WorldID       string `json:"world_id"`
	AdminRoleID   string `json:"admin_role_id"`
	BuilderRoleID string `json:"builder_role_id"`
	PlayerRoleID  string `json:"player_role_id"`
}
