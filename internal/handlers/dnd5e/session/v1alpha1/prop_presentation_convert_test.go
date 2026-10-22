package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type PropPresentationConversionSuite struct{ suite.Suite }

func TestPropPresentationConversionSuite(t *testing.T) {
	suite.Run(t, new(PropPresentationConversionSuite))
}
