package auth

import "strings"

// GameMethodPermission explicitly classifies game RPCs. New methods must be
// classified here; unknown methods never inherit a permissive service default.
func GameMethodPermission(method string) (Permissions, bool) {
	parts := strings.Split(strings.TrimPrefix(method, "/"), "/")
	if len(parts) != 2 {
		return 0, false
	}
	service, name := parts[0], parts[1]
	switch service {
	case "api.composition.v1alpha1.CompositionService":
		switch name {
		case "CreateComposition", "DeleteComposition":
			return PermissionBuild, true
		case "GetComposition", "ListCompositions":
			return PermissionPlay, true
		}
	case "dnd5e.api.authoring.v1alpha1.AuthoringService":
		switch name {
		case "PutDungeon", "ListScenarios", "ListWeapons":
			return PermissionBuild, true
		case "GetDungeon":
			return PermissionPlay, true
		}
	case "api.v1alpha1.DiceService":
		switch name {
		case "RollDice", "GetRollSession", "ClearRollSession":
			return PermissionPlay, true
		}
	case "dnd5e.api.v1alpha1.CharacterService":
		switch name {
		case "CreateDraft", "GetDraft", "ListDrafts", "DeleteDraft", "UpdateName", "UpdateRace", "UpdateClass", "UpdateBackground",
			"UpdateAbilityScores", "UpdateSkills", "UpdateAppearance", "ValidateDraft", "GetDraftPreview", "FinalizeDraft", "GetCharacter",
			"ListCharacters", "DeleteCharacter", "GetNextLevel", "LevelUp", "ListRaces", "ListClasses", "ListBackgrounds", "GetRaceDetails",
			"GetClassDetails", "GetBackgroundDetails", "GetFeature", "RollAbilityScores", "GetRequirements", "SubmitChoices",
			"ListEquipmentByType", "ListSpellsByLevel", "GetCharacterInventory", "EquipItem", "UnequipItem", "AddToInventory", "RemoveFromInventory":
			return PermissionPlay, true
		}
	case "dnd5e.api.v1alpha2.character.CharacterService":
		switch name {
		case "GetCharacterData", "EquipItem", "UnequipItem":
			return PermissionPlay, true
		}
	case "dnd5e.api.lobby.v1alpha1.LobbyService":
		switch name {
		case "CreateLobby", "JoinLobby", "SetReady", "LeaveLobby", "StartEncounter", "AbandonEncounter", "StreamLobby", "GetMyActiveLobby", "ListDungeons":
			return PermissionPlay, true
		}
	case "dnd5e.api.session.v1alpha1.SessionService":
		switch name {
		case "Join", "Exit", "Move", "Attack", "DeathSave", "OpenDoor", "Unlock", "Search", "Interact", "Trade", "Unpack", "Loot", "Hold", "Turn",
			"Afford", "Activate", "Cast", "React", "Intimidate", "Persuade", "EndTurn", "Dissolve", "End", "GetStatus", "GetStory", "GetView",
			"GetWhere", "GetAtlas", "GetRoster", "GetDoors", "StreamEvents":
			return PermissionPlay, true
		}
	case "dnd5e.api.session.presentation.v1alpha1.SessionPresentationService":
		switch name {
		case "PublishDiceThrow", "StreamDiceThrows":
			return PermissionPlay, true
		}
	}
	return 0, false
}

func isWorldManagementMethod(method string) bool {
	switch method {
	case "/api.world.v1alpha1.WorldService/GetWorld", "/api.world.v1alpha1.WorldService/SetWorldRoles", "/api.world.v1alpha1.WorldService/SetWorldMemberRoles":
		return true
	}
	return false
}
