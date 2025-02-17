package memfile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/bradleyjkemp/cupaloy"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"
	"github.com/two-hundred/celerity/libs/blueprint-state/internal"
	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/state"
)

type MemFileStateContainerResourceDriftTestSuite struct {
	container                 state.Container
	saveResourceDriftFixtures map[int]saveResourceDriftFixture
	stateDir                  string
	fs                        afero.Fs
	suite.Suite
}

func (s *MemFileStateContainerResourceDriftTestSuite) SetupTest() {
	stateDir := path.Join("__testdata", "initial-state")
	memoryFS := afero.NewMemMapFs()
	loadMemoryFS(stateDir, memoryFS, &s.Suite)
	s.fs = memoryFS
	s.stateDir = stateDir
	// Use a low max guide file size of 100 bytes to trigger the logic that splits
	// instance and resource drift state across multiple chunk files.
	container, err := LoadStateContainer(stateDir, memoryFS, core.NewNopLogger(), WithMaxGuideFileSize(100))
	s.Require().NoError(err)
	s.container = container

	s.setupSaveResourceDriftFixtures()
}

func (s *MemFileStateContainerResourceDriftTestSuite) Test_retrieves_resource_drift() {
	resources := s.container.Resources()
	resourceDriftState, err := resources.GetDrift(
		context.Background(),
		existingResourceID,
	)
	s.Require().NoError(err)
	s.Require().NotNil(resourceDriftState)
	err = cupaloy.Snapshot(resourceDriftState)
	s.Require().NoError(err)
}

func (s *MemFileStateContainerResourceDriftTestSuite) Test_reports_resource_not_found_for_drift_retrieval() {
	resources := s.container.Resources()

	_, err := resources.GetDrift(
		context.Background(),
		nonExistentResourceID,
	)
	s.Require().Error(err)
	stateErr, isStateErr := err.(*state.Error)
	s.Assert().True(isStateErr)
	s.Assert().Equal(state.ErrResourceNotFound, stateErr.Code)
}

func (s *MemFileStateContainerResourceDriftTestSuite) Test_saves_new_resource_drift() {
	fixture := s.saveResourceDriftFixtures[1]
	resources := s.container.Resources()
	err := resources.SaveDrift(
		context.Background(),
		*fixture.driftState,
	)
	s.Require().NoError(err)

	savedDriftState, err := resources.GetDrift(
		context.Background(),
		fixture.driftState.ResourceID,
	)
	s.Require().NoError(err)
	internal.AssertResourceDriftEqual(fixture.driftState, &savedDriftState, &s.Suite)
	s.assertPersistedResourceDrift(fixture.driftState)
}

func (s *MemFileStateContainerResourceDriftTestSuite) Test_updates_existing_resource_drift() {
	fixture := s.saveResourceDriftFixtures[2]
	resources := s.container.Resources()
	err := resources.SaveDrift(
		context.Background(),
		*fixture.driftState,
	)
	s.Require().NoError(err)

	savedDriftState, err := resources.GetDrift(
		context.Background(),
		fixture.driftState.ResourceID,
	)
	s.Require().NoError(err)
	internal.AssertResourceDriftEqual(fixture.driftState, &savedDriftState, &s.Suite)
	s.assertPersistedResourceDrift(fixture.driftState)
}

func (s *MemFileStateContainerResourceDriftTestSuite) Test_reports_resource_not_found_for_saving_drift() {
	// Fixture 3 is a resource state that references a non-existent instance.
	fixture := s.saveResourceDriftFixtures[3]
	resources := s.container.Resources()

	err := resources.SaveDrift(
		context.Background(),
		*fixture.driftState,
	)
	s.Require().Error(err)
	stateErr, isStateErr := err.(*state.Error)
	s.Assert().True(isStateErr)
	s.Assert().Equal(state.ErrResourceNotFound, stateErr.Code)
}

func (s *MemFileStateContainerResourceDriftTestSuite) Test_reports_malformed_state_error_for_saving_drift() {
	// The malformed state for this test case contains a resource
	// that references an instance that does not exist.
	container, err := loadMalformedStateContainer(&s.Suite)
	s.Require().NoError(err)

	resources := container.Resources()
	err = resources.SaveDrift(
		context.Background(),
		state.ResourceDriftState{
			ResourceID:   existingResourceID,
			ResourceName: existingResourceName,
			SpecData:     &core.MappingNode{},
		},
	)
	s.Require().Error(err)
	memFileErr, isMemFileErr := err.(*Error)
	s.Assert().True(isMemFileErr)
	s.Assert().Equal(ErrorReasonCodeMalformedState, memFileErr.ReasonCode)
}

func (s *MemFileStateContainerResourceDriftTestSuite) Test_removes_resource_drift() {
	resources := s.container.Resources()
	_, err := resources.RemoveDrift(context.Background(), existingResourceID)
	s.Require().NoError(err)

	drift, err := resources.GetDrift(context.Background(), existingResourceID)
	s.Require().NoError(err)
	// The resource should still exist but the drift should be an empty value.
	s.Assert().True(isEmptyDriftState(drift))

	resource, err := resources.Get(context.Background(), existingResourceID)
	s.Require().NoError(err)
	s.Assert().False(resource.Drifted)

	s.assertResourceDriftRemovedFromPersistence(existingResourceID)
}

func (s *MemFileStateContainerResourceDriftTestSuite) Test_reports_resource_not_found_for_removing_drift() {
	resources := s.container.Resources()

	_, err := resources.RemoveDrift(
		context.Background(),
		nonExistentResourceID,
	)
	s.Require().Error(err)
	stateErr, isStateErr := err.(*state.Error)
	s.Assert().True(isStateErr)
	s.Assert().Equal(state.ErrResourceNotFound, stateErr.Code)
}

func (s *MemFileStateContainerResourceDriftTestSuite) Test_does_nothing_for_missing_drift_entry_for_existing_resource() {
	resources := s.container.Resources()

	drift, err := resources.RemoveDrift(
		context.Background(),
		"test-save-order-function-id",
	)
	s.Require().NoError(err)
	s.Assert().True(isEmptyDriftState(drift))
}

func (s *MemFileStateContainerResourceDriftTestSuite) Test_reports_malformed_state_error_for_removing_drift() {
	// The malformed state for this test case contains a resource
	// that references an instance that does not exist.
	container, err := loadMalformedStateContainer(&s.Suite)
	s.Require().NoError(err)

	resources := container.Resources()
	_, err = resources.RemoveDrift(
		context.Background(),
		existingResourceID,
	)
	s.Require().Error(err)
	memFileErr, isMemFileErr := err.(*Error)
	s.Assert().True(isMemFileErr)
	s.Assert().Equal(ErrorReasonCodeMalformedState, memFileErr.ReasonCode)
}

func (s *MemFileStateContainerResourceDriftTestSuite) setupSaveResourceDriftFixtures() {
	dirPath := path.Join("__testdata", "save-input", "resource-drift")
	dirEntries, err := os.ReadDir(dirPath)
	s.Require().NoError(err)

	s.saveResourceDriftFixtures = make(map[int]saveResourceDriftFixture)
	for i := 1; i <= len(dirEntries); i++ {
		fixture, err := loadSaveResourceDriftFixture(i)
		s.Require().NoError(err)
		s.saveResourceDriftFixtures[i] = fixture
	}
}

func (s *MemFileStateContainerResourceDriftTestSuite) assertPersistedResourceDrift(expected *state.ResourceDriftState) {
	// Check that the resource drift state was saved to "disk" correctly by
	// loading a new state container from persistence and retrieving the resource drift.
	container, err := LoadStateContainer(s.stateDir, s.fs, core.NewNopLogger())
	s.Require().NoError(err)

	resources := container.Resources()
	savedDrift, err := resources.GetDrift(
		context.Background(),
		expected.ResourceID,
	)
	s.Require().NoError(err)
	internal.AssertResourceDriftEqual(expected, &savedDrift, &s.Suite)

	savedResource, err := resources.Get(
		context.Background(),
		expected.ResourceID,
	)
	s.Require().NoError(err)
	s.Assert().True(savedResource.Drifted)
	s.Assert().Equal(expected.Timestamp, savedResource.LastDriftDetectedTimestamp)
}

func (s *MemFileStateContainerResourceDriftTestSuite) assertResourceDriftRemovedFromPersistence(resourceID string) {
	// Check that the resource drift state was removed from "disk" correctly by
	// loading a new state container from persistence and retrieving the resource drift.
	container, err := LoadStateContainer(s.stateDir, s.fs, core.NewNopLogger())
	s.Require().NoError(err)

	resources := container.Resources()
	drift, err := resources.GetDrift(context.Background(), resourceID)
	s.Require().NoError(err)
	s.Assert().True(isEmptyDriftState(drift))

	resource, err := resources.Get(context.Background(), resourceID)
	s.Require().NoError(err)
	s.Assert().False(resource.Drifted)
}

type saveResourceDriftFixture struct {
	driftState *state.ResourceDriftState
}

func loadSaveResourceDriftFixture(fixtureNumber int) (saveResourceDriftFixture, error) {
	fileName := fmt.Sprintf("%d.json", fixtureNumber)
	filePath := path.Join("__testdata", "save-input", "resource-drift", fileName)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return saveResourceDriftFixture{}, err
	}

	driftState := &state.ResourceDriftState{}
	err = json.Unmarshal(data, driftState)
	if err != nil {
		return saveResourceDriftFixture{}, err
	}

	return saveResourceDriftFixture{
		driftState: driftState,
	}, nil
}

func isEmptyDriftState(actual state.ResourceDriftState) bool {
	return actual.ResourceID == "" &&
		actual.ResourceName == "" &&
		actual.SpecData == nil &&
		actual.Difference == nil &&
		actual.Timestamp == nil
}

func TestMemFileStateContainerResourceDriftTestSuite(t *testing.T) {
	suite.Run(t, new(MemFileStateContainerResourceDriftTestSuite))
}
