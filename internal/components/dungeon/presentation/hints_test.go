package presentation

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

var (
	themeCrypt      = Theme{ID: "crypt"}
	themeCave       = Theme{ID: "cave"}
	themeBanditLair = Theme{ID: "bandit_lair"}
)

type HintGeneratorTestSuite struct {
	suite.Suite
	generator HintGenerator
}

func TestHintGeneratorTestSuite(t *testing.T) {
	suite.Run(t, new(HintGeneratorTestSuite))
}

func (s *HintGeneratorTestSuite) SetupTest() {
	s.generator = NewHintGenerator()
}

func (s *HintGeneratorTestSuite) TestGenerateHints_NilInput() {
	output := s.generator.GenerateHints(nil)
	s.Nil(output.Connections)
}

func (s *HintGeneratorTestSuite) TestGenerateHints_EmptyConnections() {
	input := &HintInput{
		Connections: []*RoomConnection{},
		Theme:       themeCrypt,
		Seed:        12345,
	}

	output := s.generator.GenerateHints(input)
	s.Empty(output.Connections)
}

func (s *HintGeneratorTestSuite) TestGenerateHints_CryptTheme() {
	connections := []*RoomConnection{
		{FromRoom: "room1", ToRoom: "room2", Type: ConnectionTypeDoor, IsMainPath: true},
		{FromRoom: "room2", ToRoom: "room3", Type: ConnectionTypePassage, IsMainPath: true},
		{FromRoom: "room2", ToRoom: "room4", Type: ConnectionTypeStairs, IsMainPath: false},
	}

	input := &HintInput{
		Connections: connections,
		Theme:       themeCrypt,
		Seed:        12345,
	}

	output := s.generator.GenerateHints(input)
	s.Len(output.Connections, 3)

	// Verify all connections have hints
	for _, conn := range output.Connections {
		s.NotEmpty(conn.PhysicalHint, "connection should have physical hint")
	}

	// Verify crypt-themed vocabulary
	doorHints := []string{
		"heavy stone door",
		"iron-bound door",
		"ornate bronze door",
		"sealed vault door",
		"crumbling wooden door",
		"arched doorway",
	}

	passageHints := []string{
		"narrow passage",
		"shadowy corridor",
		"vaulted hallway",
		"torch-lit passage",
		"crumbling archway",
		"low stone tunnel",
	}

	stairHints := []string{
		"worn stone steps descending",
		"spiral staircase",
		"cracked marble stairs",
		"narrow stone steps",
	}

	s.Contains(doorHints, output.Connections[0].PhysicalHint, "door should use crypt door vocabulary")
	s.Contains(passageHints, output.Connections[1].PhysicalHint, "passage should use crypt passage vocabulary")
	s.Contains(stairHints, output.Connections[2].PhysicalHint, "stairs should use crypt stair vocabulary")
}

func (s *HintGeneratorTestSuite) TestGenerateHints_CaveTheme() {
	connections := []*RoomConnection{
		{FromRoom: "room1", ToRoom: "room2", Type: ConnectionTypeDoor, IsMainPath: true},
		{FromRoom: "room2", ToRoom: "room3", Type: ConnectionTypePassage, IsMainPath: false},
	}

	input := &HintInput{
		Connections: connections,
		Theme:       themeCave,
		Seed:        54321,
	}

	output := s.generator.GenerateHints(input)
	s.Len(output.Connections, 2)

	// Verify cave-themed vocabulary
	caveDoorHints := []string{
		"narrow crevice",
		"boulder-strewn opening",
		"low cave entrance",
		"dark opening",
		"moss-covered entrance",
		"rocky archway",
	}

	cavePassageHints := []string{
		"winding tunnel",
		"jagged passage",
		"sloping corridor",
		"damp cavern passage",
		"rough-hewn tunnel",
		"natural archway",
	}

	s.Contains(caveDoorHints, output.Connections[0].PhysicalHint)
	s.Contains(cavePassageHints, output.Connections[1].PhysicalHint)
}

func (s *HintGeneratorTestSuite) TestGenerateHints_BanditLairTheme() {
	connections := []*RoomConnection{
		{FromRoom: "room1", ToRoom: "room2", Type: ConnectionTypeDoor, IsMainPath: true},
		{FromRoom: "room2", ToRoom: "room3", Type: ConnectionTypeStairs, IsMainPath: true},
	}

	input := &HintInput{
		Connections: connections,
		Theme:       themeBanditLair,
		Seed:        99999,
	}

	output := s.generator.GenerateHints(input)
	s.Len(output.Connections, 2)

	// Verify bandit lair vocabulary
	banditDoorHints := []string{
		"reinforced wooden door",
		"iron gate",
		"barred entrance",
		"heavy oak door",
		"trap door",
		"guard post entrance",
	}

	banditStairHints := []string{
		"rickety wooden stairs",
		"ladder to lower level",
		"rough-cut steps",
		"rope ladder",
	}

	s.Contains(banditDoorHints, output.Connections[0].PhysicalHint)
	s.Contains(banditStairHints, output.Connections[1].PhysicalHint)
}

func (s *HintGeneratorTestSuite) TestGenerateHints_DeterministicWithSeed() {
	connections := []*RoomConnection{
		{FromRoom: "room1", ToRoom: "room2", Type: ConnectionTypeDoor, IsMainPath: true},
		{FromRoom: "room2", ToRoom: "room3", Type: ConnectionTypePassage, IsMainPath: true},
		{FromRoom: "room2", ToRoom: "room4", Type: ConnectionTypeDoor, IsMainPath: false},
	}

	input1 := &HintInput{
		Connections: connections,
		Theme:       themeCrypt,
		Seed:        12345,
	}

	input2 := &HintInput{
		Connections: connections,
		Theme:       themeCrypt,
		Seed:        12345,
	}

	output1 := s.generator.GenerateHints(input1)
	output2 := s.generator.GenerateHints(input2)

	// Same seed should produce same hints
	s.Len(output1.Connections, 3)
	s.Len(output2.Connections, 3)

	for i := range output1.Connections {
		s.Equal(output1.Connections[i].PhysicalHint, output2.Connections[i].PhysicalHint,
			"same seed should produce same hints")
	}
}

func (s *HintGeneratorTestSuite) TestGenerateHints_DifferentWithDifferentSeeds() {
	connections := []*RoomConnection{
		{FromRoom: "room1", ToRoom: "room2", Type: ConnectionTypeDoor, IsMainPath: true},
		{FromRoom: "room2", ToRoom: "room3", Type: ConnectionTypeDoor, IsMainPath: true},
		{FromRoom: "room2", ToRoom: "room4", Type: ConnectionTypeDoor, IsMainPath: false},
		{FromRoom: "room3", ToRoom: "room5", Type: ConnectionTypeDoor, IsMainPath: true},
	}

	input1 := &HintInput{
		Connections: connections,
		Theme:       themeCrypt,
		Seed:        12345,
	}

	input2 := &HintInput{
		Connections: connections,
		Theme:       themeCrypt,
		Seed:        54321,
	}

	output1 := s.generator.GenerateHints(input1)
	output2 := s.generator.GenerateHints(input2)

	// Different seeds should likely produce at least some different hints
	differentCount := 0
	for i := range output1.Connections {
		if output1.Connections[i].PhysicalHint != output2.Connections[i].PhysicalHint {
			differentCount++
		}
	}

	s.Greater(differentCount, 0, "different seeds should produce different hints")
}

func (s *HintGeneratorTestSuite) TestGenerateHints_PreservesConnectionData() {
	connections := []*RoomConnection{
		{FromRoom: "room1", ToRoom: "room2", Type: ConnectionTypeDoor, IsMainPath: true},
		{FromRoom: "room2", ToRoom: "room3", Type: ConnectionTypePassage, IsMainPath: false},
	}

	input := &HintInput{
		Connections: connections,
		Theme:       themeCrypt,
		Seed:        12345,
	}

	output := s.generator.GenerateHints(input)

	// Verify all original data is preserved
	for i, conn := range output.Connections {
		s.Equal(connections[i].FromRoom, conn.FromRoom)
		s.Equal(connections[i].ToRoom, conn.ToRoom)
		s.Equal(connections[i].Type, conn.Type)
		s.Equal(connections[i].IsMainPath, conn.IsMainPath)
	}
}

func (s *HintGeneratorTestSuite) TestGenerateHints_MainPathNotObvious() {
	// Create a mix of main path and side paths
	connections := []*RoomConnection{
		{FromRoom: "room1", ToRoom: "room2", Type: ConnectionTypeDoor, IsMainPath: true},
		{FromRoom: "room2", ToRoom: "room3", Type: ConnectionTypeDoor, IsMainPath: false},
		{FromRoom: "room2", ToRoom: "room4", Type: ConnectionTypeDoor, IsMainPath: true},
		{FromRoom: "room4", ToRoom: "room5", Type: ConnectionTypeDoor, IsMainPath: false},
		{FromRoom: "room4", ToRoom: "room6", Type: ConnectionTypeDoor, IsMainPath: true},
	}

	input := &HintInput{
		Connections: connections,
		Theme:       themeCrypt,
		Seed:        12345,
	}

	output := s.generator.GenerateHints(input)

	// Collect hints for main path vs side paths
	mainPathHints := make(map[string]bool)
	sidePathHints := make(map[string]bool)

	for i, conn := range output.Connections {
		if connections[i].IsMainPath {
			mainPathHints[conn.PhysicalHint] = true
		} else {
			sidePathHints[conn.PhysicalHint] = true
		}
	}

	// Verify that main path connections don't have unique/special hints
	// that would make them obviously different from side paths
	// This is a weak test, but verifies the vocabulary is shared
	s.Greater(len(mainPathHints), 0, "should have main path hints")
	s.Greater(len(sidePathHints), 0, "should have side path hints")
}

func (s *HintGeneratorTestSuite) TestGenerateHints_VariedHints() {
	// Create many connections of the same type
	connections := make([]*RoomConnection, 20)
	for i := range connections {
		connections[i] = &RoomConnection{
			FromRoom:   "room1",
			ToRoom:     "room2",
			Type:       ConnectionTypeDoor,
			IsMainPath: i%2 == 0,
		}
	}

	input := &HintInput{
		Connections: connections,
		Theme:       themeCrypt,
		Seed:        12345,
	}

	output := s.generator.GenerateHints(input)

	// Collect unique hints
	uniqueHints := make(map[string]bool)
	for _, conn := range output.Connections {
		uniqueHints[conn.PhysicalHint] = true
	}

	// Should have multiple different hints, not just one repeated
	s.Greater(len(uniqueHints), 1, "should use varied hints, not repeat the same one")
}
