package main_test

import (
	"testing"

	"github.com/plainid/git-backup/config"
	"github.com/plainid/git-backup/plainid"
	"github.com/stretchr/testify/suite"
)

// PlainIDServiceTestSuite defines the test suite for PlainID service
type PlainIDServiceTestSuite struct {
	suite.Suite
	cfg config.Config
}

func TestPlainIDServiceSuite(t *testing.T) {
	suite.Run(t, new(PlainIDServiceTestSuite))
}

// SetupSuite runs once before all tests in the suite
func (s *PlainIDServiceTestSuite) SetupSuite() {
	cfg, err := config.LoadConfig(nil)
	s.Require().NoError(err, "Failed to load configuration")
	s.cfg = *cfg
}

func (s *PlainIDServiceTestSuite) TestPAAGroups() {
	// Create service instance
	service := plainid.NewService(s.cfg)

	// Call PAAGroups method
	result, err := service.PAAGroups(s.cfg.PlainID.Envs[0].ID)
	s.Require().NoError(err, "PAAGroups should not return an error")
	s.Assert().NotEmpty(result, "PAAGroups should return non-empty result")

}
