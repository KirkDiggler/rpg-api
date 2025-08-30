package character

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
)

func (o *Orchestrator) ListRaces(ctx context.Context, input *ListRacesInput) (*ListRacesOutput, error) {
	// For now, continue using external client for complete race data
	// TODO: Migrate to pure toolkit approach when toolkit has all display data
	allRaces := races.All

	// Get race data for each race
	raceList := make([]RaceListItem, 0, len(allRaces))
	for _, raceID := range allRaces {
		// Use external client for now since it has complete data
		raceDataOutput, err := o.externalClient.GetRaceData(ctx, string(raceID))
		if err != nil {
			// Skip races that fail to load
			continue
		}

		raceList = append(raceList, RaceListItem{
			RaceData: raceDataOutput.RaceData,
			UIData:   raceDataOutput.UIData,
		})
	}

	// Simple pagination - for now just return all races
	// TODO: Implement proper pagination when needed
	return &ListRacesOutput{
		Races:         raceList,
		NextPageToken: "",
		TotalSize:     len(raceList),
	}, nil
}

func (o *Orchestrator) ListClasses(ctx context.Context, input *ListClassesInput) (*ListClassesOutput, error) {
	// Get all starting classes from the toolkit
	// This includes subclasses for Cleric, Sorcerer, and Warlock
	startingClasses := character.ListStartingClasses()

	// Simple pagination - for now just return all classes
	// TODO: Implement proper pagination when needed
	return &ListClassesOutput{
		Classes:       startingClasses,
		NextPageToken: "",
		TotalSize:     len(startingClasses),
	}, nil
}

func (o *Orchestrator) ListBackgrounds(ctx context.Context, input *ListBackgroundsInput) (*ListBackgroundsOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) GetRaceDetails(ctx context.Context, input *GetRaceDetailsInput) (*GetRaceDetailsOutput, error) {
	if input.RaceID == "" {
		return nil, errors.InvalidArgument("race ID is required")
	}

	// Get race data from external client
	raceDataOutput, err := o.externalClient.GetRaceData(ctx, input.RaceID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get race data for %s", input.RaceID)
	}

	return &GetRaceDetailsOutput{
		RaceData: raceDataOutput.RaceData,
		UIData:   raceDataOutput.UIData,
	}, nil
}

func (o *Orchestrator) GetClassDetails(ctx context.Context, input *GetClassDetailsInput) (*GetClassDetailsOutput, error) {
	if input.ClassID == "" {
		return nil, errors.InvalidArgument("class ID is required")
	}

	// Get class data from external client
	classDataOutput, err := o.externalClient.GetClassData(ctx, input.ClassID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get class data for %s", input.ClassID)
	}

	return &GetClassDetailsOutput{
		ClassData: classDataOutput.ClassData,
		UIData:    classDataOutput.UIData,
	}, nil
}

func (o *Orchestrator) GetBackgroundDetails(ctx context.Context, input *GetBackgroundDetailsInput) (*GetBackgroundDetailsOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}
