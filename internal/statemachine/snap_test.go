package statemachine

import (
	"testing"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
)

// TestFailedValidateInputSnap tests a failure in the Setup() function when validating common input
func TestFailedValidateInputSnap(t *testing.T) {
	t.Run("test_failed_validate_input", func(t *testing.T) {
		restoreArgs := helper.Setup()
		defer restoreArgs()

		// use both --until and --thru to trigger this failure
		commands.StateMachineOptsPassed.Until = "until-test"
		commands.StateMachineOptsPassed.Thru = "thru-test"

		var stateMachine snapStateMachine
		if err := stateMachine.Setup(); err == nil {
			t.Error("Expected an error but there was none")
		}
	})
}

// TestFailedReadMetadataSnap tests a failed metadata read by passing --resume with no previous partial state machine run
func TestFailedReadMetadataSnap(t *testing.T) {
	t.Run("test_failed_read_metadata", func(t *testing.T) {
		restoreArgs := helper.Setup()
		defer restoreArgs()

		// start a --resume with no previous SM run
		commands.StateMachineOptsPassed.Resume = true
		commands.StateMachineOptsPassed.WorkDir = testDir

		var stateMachine snapStateMachine
		if err := stateMachine.Setup(); err == nil {
			t.Error("Expected an error but there was none")
		}
	})
}

// TestSuccessfulSnapRun runs through all states ensuring none failed
func TestSuccessfulSnapRun(t *testing.T) {
	t.Run("test_successful_snap_run", func(t *testing.T) {
		restoreArgs := helper.Setup()
		defer restoreArgs()

		var stateMachine snapStateMachine

		if err := stateMachine.Setup(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

		if err := stateMachine.Run(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

		if err := stateMachine.Teardown(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}
	})
}