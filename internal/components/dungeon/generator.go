package dungeon

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// LayoutGenerator creates room layouts and connections
type LayoutGenerator interface {
	Generate(ctx context.Context, input *LayoutInput) (*LayoutOutput, error)
}

// LayoutInput defines parameters for layout generation
type LayoutInput struct {
	Length     int
	LayoutType LayoutType
	Seed       int64
}

// LayoutOutput contains the generated room layout
type LayoutOutput struct {
	RoomSlots   []RoomSlot
	Connections []*LayoutConnection
	StartRoom   int // index
	BossRoom    int // index
}

// LayoutConnection defines a connection between two room slots by index
type LayoutConnection struct {
	FromRoom     int // index into RoomSlots
	ToRoom       int // index into RoomSlots
	Type         ConnectionType
	IsMainPath   bool
	PhysicalHint string
}

// RoomSlot represents a placeholder for a room to be generated
type RoomSlot struct {
	ID       string
	RoomType string // "entrance", "regular", "boss"
}

// ShapeGenerator creates room geometry
type ShapeGenerator interface {
	Generate(ctx context.Context, input *ShapeInput) (*ShapeOutput, error)
}

// ShapeInput defines parameters for shape generation
type ShapeInput struct {
	Size  RoomSize
	Style ShapeStyle
	Seed  int64
}

// ShapeOutput contains the generated shape
type ShapeOutput struct {
	Shape *Shape
}

// FeatureGenerator places obstacles and spawn zones
type FeatureGenerator interface {
	Generate(ctx context.Context, input *FeatureInput) (*FeatureOutput, error)
}

// FeatureInput defines parameters for feature placement
type FeatureInput struct {
	Shape    *Shape
	Rules    FeatureRules
	RoomType string // affects spawn zone placement
	Seed     int64
}

// FeatureOutput contains the generated features
type FeatureOutput struct {
	Features *FeatureLayout
}

// EncounterGenerator creates monster encounters for rooms
type EncounterGenerator interface {
	Generate(ctx context.Context, input *EncounterInput) (*EncounterOutput, error)
}

// EncounterInput defines parameters for encounter generation
type EncounterInput struct {
	MonsterPool []MonsterRef
	BossPool    []MonsterRef
	CRBudget    float64
	IsBossRoom  bool
	Seed        int64
}

// EncounterOutput contains the generated encounter
type EncounterOutput struct {
	Encounter *Encounter
}

// BudgetAllocator allocates CR budget across rooms
type BudgetAllocator interface {
	AllocateBudget(input *BudgetInput) *BudgetOutput
}

// BudgetInput defines parameters for budget allocation
type BudgetInput struct {
	TotalRooms int
	PartySize  int
	TargetCR   int
}

// BudgetOutput contains the allocated budgets
type BudgetOutput struct {
	RoomBudgets []float64
}

// GeneratorConfig contains dependencies for the generator
type GeneratorConfig struct {
	LayoutGen       LayoutGenerator
	ShapeGen        ShapeGenerator
	FeatureGen      FeatureGenerator
	EncounterGen    EncounterGenerator
	BudgetAllocator BudgetAllocator
}

// Generator orchestrates all layers of dungeon generation
type Generator struct {
	layoutGen       LayoutGenerator
	shapeGen        ShapeGenerator
	featureGen      FeatureGenerator
	encounterGen    EncounterGenerator
	budgetAllocator BudgetAllocator
}

// NewGenerator creates a new dungeon generator with injected dependencies
func NewGenerator(cfg *GeneratorConfig) *Generator {
	return &Generator{
		layoutGen:       cfg.LayoutGen,
		shapeGen:        cfg.ShapeGen,
		featureGen:      cfg.FeatureGen,
		encounterGen:    cfg.EncounterGen,
		budgetAllocator: cfg.BudgetAllocator,
	}
}

// Generate creates a complete dungeon from the input parameters
func (g *Generator) Generate(ctx context.Context, input *GenerateInput) (*GenerateOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	if input.Length <= 0 {
		return nil, fmt.Errorf("length must be greater than 0")
	}

	if input.PartySize <= 0 {
		return nil, fmt.Errorf("party size must be greater than 0")
	}

	if input.TargetCR <= 0 {
		return nil, fmt.Errorf("target CR must be greater than 0")
	}

	// Generate or use provided seed
	seed := input.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	// Step 1: Allocate CR budget across rooms
	budgetOutput := g.budgetAllocator.AllocateBudget(&BudgetInput{
		TotalRooms: input.Length,
		PartySize:  input.PartySize,
		TargetCR:   input.TargetCR,
	})

	// Step 2: Generate layout (rooms and connections)
	layoutOutput, err := g.layoutGen.Generate(ctx, &LayoutInput{
		Length:     input.Length,
		LayoutType: input.Layout,
		Seed:       seed,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate layout: %w", err)
	}

	// Step 3: Generate each room (shape, features, encounter)
	rooms := make([]*Room, len(layoutOutput.RoomSlots))
	for i, slot := range layoutOutput.RoomSlots {
		room, err := g.generateRoom(ctx, &generateRoomInput{
			slot:       slot,
			size:       input.Size,
			theme:      input.Theme,
			crBudget:   budgetOutput.RoomBudgets[i],
			isBossRoom: i == layoutOutput.BossRoom,
			seed:       seed + int64(i),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to generate room %d: %w", i, err)
		}
		rooms[i] = room
	}

	// Step 4: Update connections with actual room IDs
	connections := make([]*RoomConnection, len(layoutOutput.Connections))
	for i, conn := range layoutOutput.Connections {
		connections[i] = &RoomConnection{
			FromRoom:     rooms[conn.FromRoom].ID,
			ToRoom:       rooms[conn.ToRoom].ID,
			Type:         conn.Type,
			IsMainPath:   conn.IsMainPath,
			PhysicalHint: conn.PhysicalHint,
		}
	}

	// Step 5: Assemble final dungeon
	dungeon := &Dungeon{
		ID:          uuid.New().String(),
		Theme:       input.Theme,
		Rooms:       rooms,
		Connections: connections,
		StartRoom:   rooms[layoutOutput.StartRoom].ID,
		BossRoom:    rooms[layoutOutput.BossRoom].ID,
	}

	return &GenerateOutput{
		Dungeon: dungeon,
		Seed:    seed,
	}, nil
}

// generateRoomInput contains all parameters needed to generate a single room
type generateRoomInput struct {
	slot       RoomSlot
	size       RoomSize
	theme      Theme
	crBudget   float64
	isBossRoom bool
	seed       int64
}

// generateRoom creates a single room with shape, features, and encounter
func (g *Generator) generateRoom(ctx context.Context, input *generateRoomInput) (*Room, error) {
	// Generate shape
	shapeOutput, err := g.shapeGen.Generate(ctx, &ShapeInput{
		Size:  input.size,
		Style: input.theme.ShapeStyle,
		Seed:  input.seed,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate shape: %w", err)
	}

	// Generate features
	featureOutput, err := g.featureGen.Generate(ctx, &FeatureInput{
		Shape:    shapeOutput.Shape,
		Rules:    input.theme.Features,
		RoomType: input.slot.RoomType,
		Seed:     input.seed + 1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate features: %w", err)
	}

	// Generate encounter
	encounterOutput, err := g.encounterGen.Generate(ctx, &EncounterInput{
		MonsterPool: input.theme.MonsterPool,
		BossPool:    input.theme.BossPool,
		CRBudget:    input.crBudget,
		IsBossRoom:  input.isBossRoom,
		Seed:        input.seed + 2,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate encounter: %w", err)
	}

	// Convert spawn zones to pointers
	spawnZones := make([]*Zone, len(featureOutput.Features.SpawnZones))
	for i := range featureOutput.Features.SpawnZones {
		zone := featureOutput.Features.SpawnZones[i]
		spawnZones[i] = &zone
	}

	return &Room{
		ID:         input.slot.ID,
		Shape:      shapeOutput.Shape,
		Features:   featureOutput.Features,
		SpawnZones: spawnZones,
		Encounter:  encounterOutput.Encounter,
	}, nil
}
