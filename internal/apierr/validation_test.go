package apierrors_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/apierrors"
)

type ValidationTestSuite struct {
	suite.Suite
}

func TestValidationSuite(t *testing.T) {
	suite.Run(t, new(ValidationTestSuite))
}

func (s *ValidationTestSuite) TestValidationError() {
	ve := apierrors.NewValidationError()
	ve.AddFieldError("name", "is required")
	ve.AddFieldError("email", "is invalid")
	ve.AddFieldErrorf("age", "must be at least %d", 18)

	s.Assert().True(ve.HasErrors())
	s.Assert().Contains(ve.Error(), "name: is required")
	s.Assert().Contains(ve.Error(), "email: is invalid")
	s.Assert().Contains(ve.Error(), "age: must be at least 18")

	err := ve.ToError()
	s.Assert().Equal(apierrors.CodeInvalidArgument, err.Code)
	s.Assert().NotNil(err.Meta["validation_errors"])
}

func (s *ValidationTestSuite) TestValidationBuilder() {
	vb := apierrors.NewValidationBuilder()
	vb.Field("name", "is required").
		Fieldf("level", "must be between %d and %d", 1, 20).
		RequiredField("class").
		InvalidField("alignment", "not a valid alignment")

	err := vb.Build()
	s.Require().NotNil(err)
	s.Assert().True(apierrors.IsInvalidArgument(err))
}

func (s *ValidationTestSuite) TestValidationBuilderNoErrors() {
	vb := apierrors.NewValidationBuilder()
	err := vb.Build()
	s.Assert().Nil(err)
}

func (s *ValidationTestSuite) TestValidateRequired() {
	testCases := []struct {
		name      string
		value     string
		shouldErr bool
	}{
		{"valid value", "test", false},
		{"empty string", "", true},
		{"whitespace only", "   ", true},
		{"valid with spaces", "  test  ", false},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			vb := apierrors.NewValidationBuilder()
			apierrors.ValidateRequired("field", tc.value, vb)
			err := vb.Build()
			if tc.shouldErr {
				s.Assert().NotNil(err)
			} else {
				s.Assert().Nil(err)
			}
		})
	}
}

func (s *ValidationTestSuite) TestValidateMinLength() {
	vb := apierrors.NewValidationBuilder()
	apierrors.ValidateMinLength("password", "short", 8, vb)
	apierrors.ValidateMinLength("username", "validuser", 3, vb)

	err := vb.Build()
	s.Require().NotNil(err)
	meta := apierrors.GetMeta(err)
	validationErrors := meta["validation_errors"].(map[string][]string)
	s.Assert().Contains(validationErrors["password"][0], "must be at least 8 characters")
	s.Assert().NotContains(validationErrors, "username")
}

func (s *ValidationTestSuite) TestValidateMaxLength() {
	vb := apierrors.NewValidationBuilder()
	apierrors.ValidateMaxLength("name", "this is a very long character name", 20, vb)
	apierrors.ValidateMaxLength("code", "ABC", 5, vb)

	err := vb.Build()
	s.Require().NotNil(err)
	meta := apierrors.GetMeta(err)
	validationErrors := meta["validation_errors"].(map[string][]string)
	s.Assert().Contains(validationErrors["name"][0], "must be no more than 20 characters")
	s.Assert().NotContains(validationErrors, "code")
}

func (s *ValidationTestSuite) TestValidateRange() {
	vb := apierrors.NewValidationBuilder()
	apierrors.ValidateRange("level", 25, 1, 20, vb)
	apierrors.ValidateRange("ability", 15, 3, 18, vb)
	apierrors.ValidateRange("hp", 0, 1, 100, vb)

	err := vb.Build()
	s.Require().NotNil(err)
	meta := apierrors.GetMeta(err)
	validationErrors := meta["validation_errors"].(map[string][]string)
	s.Assert().Contains(validationErrors["level"][0], "must be between 1 and 20")
	s.Assert().Contains(validationErrors["hp"][0], "must be between 1 and 100")
	s.Assert().NotContains(validationErrors, "ability")
}

func (s *ValidationTestSuite) TestValidateEnum() {
	allowedClasses := []string{"fighter", "wizard", "rogue", "cleric"}

	vb := apierrors.NewValidationBuilder()
	apierrors.ValidateEnum("class", "bard", allowedClasses, vb)
	apierrors.ValidateEnum("primary_class", "fighter", allowedClasses, vb)

	err := vb.Build()
	s.Require().NotNil(err)
	meta := apierrors.GetMeta(err)
	validationErrors := meta["validation_errors"].(map[string][]string)
	s.Assert().Contains(validationErrors["class"][0], "must be one of: fighter, wizard, rogue, cleric")
	s.Assert().NotContains(validationErrors, "primary_class")
}

func (s *ValidationTestSuite) TestComplexValidation() {
	// Simulate validating a character creation request
	type CharacterInput struct {
		Name      string
		Class     string
		Level     int
		Abilities map[string]int
	}

	input := CharacterInput{
		Name:  "",
		Class: "barbarian",
		Level: 25,
		Abilities: map[string]int{
			"strength":     20,
			"dexterity":    15,
			"constitution": 14,
		},
	}

	vb := apierrors.NewValidationBuilder()

	// Validate name
	apierrors.ValidateRequired("name", input.Name, vb)

	// Validate class
	allowedClasses := []string{"fighter", "wizard", "rogue", "cleric"}
	apierrors.ValidateEnum("class", input.Class, allowedClasses, vb)

	// Validate level
	apierrors.ValidateRange("level", input.Level, 1, 20, vb)

	// Validate abilities
	for ability, score := range input.Abilities {
		apierrors.ValidateRange(ability, score, 3, 18, vb)
	}

	err := vb.Build()
	s.Require().NotNil(err)
	s.Assert().True(apierrors.IsInvalidArgument(err))

	meta := apierrors.GetMeta(err)
	validationErrors := meta["validation_errors"].(map[string][]string)
	s.Assert().Contains(validationErrors, "name")
	s.Assert().Contains(validationErrors, "class")
	s.Assert().Contains(validationErrors, "level")
	s.Assert().Contains(validationErrors, "strength")
}
