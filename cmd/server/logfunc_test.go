package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"testing"

	grpc_logging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
)

// LogFuncSuite pins the "completed in" line the benchmark harness parses
// (rpg-project#740, report.mjs): `✓ <method> completed in <ms>ms`.
type LogFuncSuite struct {
	suite.Suite
	out    bytes.Buffer
	flags  int
	writer io.Writer
}

func TestLogFuncSuite(t *testing.T) { suite.Run(t, new(LogFuncSuite)) }

func (s *LogFuncSuite) SetupTest() {
	s.out.Reset()
	s.flags = log.Flags()
	s.writer = log.Writer()
	log.SetOutput(&s.out)
	log.SetFlags(0)
}

func (s *LogFuncSuite) TearDownTest() {
	log.SetOutput(s.writer)
	log.SetFlags(s.flags)
}

func (s *LogFuncSuite) finished(timeMs any) string {
	s.out.Reset()
	logFunc(context.Background(), grpc_logging.LevelInfo, "finished call",
		"grpc.method", "ListDungeons", "grpc.code", codes.OK, "grpc.time_ms", timeMs)
	return s.out.String()
}

// The middleware (go-grpc-middleware v2 DurationToTimeMillisFields) sends
// the duration as a string; before the fix every call logged 0.00ms.
func (s *LogFuncSuite) TestCompletedLine_StringTimeMsIsPrinted() {
	s.Equal("✓ ListDungeons completed in 12.35ms\n", s.finished("12.345"))
}

func (s *LogFuncSuite) TestCompletedLine_FloatTimeMsIsPrinted() {
	s.Equal("✓ ListDungeons completed in 7.50ms\n", s.finished(7.5))
}

func (s *LogFuncSuite) TestCompletedLine_TheMiddlewaresOwnFieldIsPrinted() {
	fields := grpc_logging.DurationToTimeMillisFields(1234567 * 1000) // 1234.567 ms
	s.Require().Len(fields, 2)
	s.Equal("✓ ListDungeons completed in 1234.57ms\n", s.finished(fields[1]))
}

func (s *LogFuncSuite) TestFailedLine_StringTimeMsIsPrinted() {
	s.out.Reset()
	logFunc(context.Background(), grpc_logging.LevelInfo, "finished call",
		"grpc.method", "GetDungeon", "grpc.code", codes.NotFound, "grpc.time_ms", "3.2", "grpc.error", "nope")
	s.Equal("✗ GetDungeon failed (NotFound) in 3.20ms: nope\n", s.out.String())
}
