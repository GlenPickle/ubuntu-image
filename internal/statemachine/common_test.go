// This file contains unit tests for all of the common state functions
package statemachine

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/google/uuid"
	"github.com/snapcore/snapd/gadget"
)

// TestMakeTemporaryDirectories tests a successful execution of the
// make_temporary_directories state with and without --workdir
func TestMakeTemporaryDirectories(t *testing.T) {
	testCases := []struct {
		name    string
		workdir string
	}{
		{"with_workdir", "/tmp/make_temporary_directories-" + uuid.NewString()},
		{"without_workdir", ""},
	}
	for _, tc := range testCases {
		t.Run("test_"+tc.name, func(t *testing.T) {
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.stateMachineFlags.WorkDir = tc.workdir
			if err := stateMachine.makeTemporaryDirectories(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}
			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestFailedMakeTemporaryDirectories tests some failed executions of the make_temporary_directories state
func TestFailedMakeTemporaryDirectories(t *testing.T) {
	t.Run("test_failed_make_temporary_directories", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		osMkdir = mockMkdir
		if err := stateMachine.makeTemporaryDirectories(); err == nil {
			// try adding a workdir to see if that triggers the failure
			stateMachine.stateMachineFlags.WorkDir = testDir
			if err := stateMachine.makeTemporaryDirectories(); err == nil {
				t.Error("Expected an error, but got none")
			}
		}
		osMkdirAll = mockMkdir
		if err := stateMachine.makeTemporaryDirectories(); err == nil {
			// try adding a workdir to see if that triggers the failure
			stateMachine.stateMachineFlags.WorkDir = testDir
			if err := stateMachine.makeTemporaryDirectories(); err == nil {
				t.Error("Expected an error, but got none")
			}
		}
		osMkdir = os.Mkdir
		osMkdirAll = os.MkdirAll
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestLoadGadgetYaml tests a succesful load of gadget.yaml
func TestLoadGadgetYaml(t *testing.T) {
	t.Run("test_load_gadget_yaml", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.yamlFilePath = filepath.Join("testdata", "gadget_tree", "meta", "gadget.yaml")

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		if err := stateMachine.loadGadgetYaml(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedLoadGadgetYaml tests a failure in the loadGadgetYaml state
// This is achieved by providing an invalid gadget.yaml
func TestFailedLoadGadgetYaml(t *testing.T) {
	t.Run("test_failed_load_gadget_yaml", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.yamlFilePath = filepath.Join("testdata", "gadget_tree_invalid", "meta", "gadget.yaml")
		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		if err := stateMachine.loadGadgetYaml(); err == nil {
			t.Error("Expected an error, but got none")
		}

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestPopulateRootfsContentsHooks ensures that the PopulateSnapRootfsContentsHooks
// function can successfully run hook scripts and that core20 skips them
func TestPopulateRootfsContentsHooks(t *testing.T) {
	testCases := []struct {
		name         string
		hooksAllowed bool
		hooksCreated []string
	}{
		{"hooks_succeed", true, []string{"post-populate-rootfs-hookfile", "post-populate-rootfs-hookfile.d1", "post-populate-rootfs-hookfile.d2"}},
		{"hooks_not_allowed", false, []string{}},
	}
	for _, tc := range testCases {
		t.Run("test_"+tc.name, func(t *testing.T) {
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.commonFlags.HooksDirectories = []string{
				filepath.Join("testdata", "good_hooksd"),
				filepath.Join("testdata", "good_hookscript"),
			}
			stateMachine.hooksAllowed = tc.hooksAllowed

			// need workdir set up for this
			if err := stateMachine.makeTemporaryDirectories(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}

			if err := stateMachine.populateRootfsContentsHooks(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}

			// the hook scripts used for testing simply touch some files
			for _, file := range tc.hooksCreated {
				_, err := os.Stat(filepath.Join(stateMachine.tempDirs.rootfs, file))
				if err != nil {
					if os.IsNotExist(err) {
						t.Errorf("File %s should exist, but does not", file)
					}
				}
			}

			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestFailedPopulateRootfsContentsHooks tests a variety of failures in running the hooks
func TestFailedPopulateRootfsContentsHooks(t *testing.T) {
	testCases := []struct {
		name      string
		hooksDirs []string
	}{
		{"hooks_not_executable", []string{filepath.Join("testdata", "hooks_not_executable")}},
		{"hooks_return_error", []string{filepath.Join("testdata", "hooks_return_error")}},
	}
	for _, tc := range testCases {
		t.Run("test_"+tc.name, func(t *testing.T) {
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.commonFlags.HooksDirectories = tc.hooksDirs
			stateMachine.hooksAllowed = true

			// need workdir set up for this
			if err := stateMachine.makeTemporaryDirectories(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}

			if err := stateMachine.populateRootfsContentsHooks(); err == nil {
				t.Errorf("Expected an error, but got none")
			}
			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestGenerateDiskInfo tests that diskInfo can be generated
func TestGenerateDiskInfo(t *testing.T) {
	t.Run("test_generate_disk_info", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.commonFlags.DiskInfo = filepath.Join("testdata", "disk_info")

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		if err := stateMachine.generateDiskInfo(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// make sure rootfs/.disk/info exists
		_, err := os.Stat(filepath.Join(stateMachine.tempDirs.rootfs, ".disk", "info"))
		if err != nil {
			if os.IsNotExist(err) {
				t.Errorf("Disk Info file should exist, but does not")
			}
		}

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedGenerateDiskInfo tests failure scenarios in the generate_disk_info state
func TestFailedGenerateDiskInfo(t *testing.T) {
	t.Run("test_failed_generate_disk_info", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.commonFlags.DiskInfo = filepath.Join("testdata", "fake_disk_info")

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		if err := stateMachine.generateDiskInfo(); err == nil {
			t.Errorf("Expected an error, but got none")
		}

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestCalculateRootfsSize tests that the rootfs size can be calculated
// this is accomplished by setting the test gadget tree as rootfs and
// verifying that the size is calculated correctly
func TestCalculateRootfsSize(t *testing.T) {
	t.Run("test_calculate_rootfs_size", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.tempDirs.rootfs = filepath.Join("testdata", "gadget_tree")

		if err := stateMachine.calculateRootfsSize(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		correctSize := "12.01 MiB"
		if stateMachine.rootfsSize.IECString() != correctSize {
			t.Errorf("expected rootfsSize = %s, got %s", correctSize, stateMachine.rootfsSize.IECString())
		}

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedCalculateRootfsSize tests a failure when calculating the rootfs size
// this is accomplished by setting rootfs to a directory that does not exist
func TestFailedCalculateRootfsSize(t *testing.T) {
	t.Run("test_failed_calculate_rootfs_size", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.tempDirs.rootfs = filepath.Join("testdata", uuid.NewString())

		if err := stateMachine.calculateRootfsSize(); err == nil {
			t.Errorf("Expected an error, but got none")
		}

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestPrepopulateBootfsContents tests a successful call to stateMachine.prePopulateBootfsContents
func TestPrepopulateBootfsContents(t *testing.T) {
	t.Run("test_prepopulate_bootfs_contents", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.tempDirs.rootfs = filepath.Join("testdata", uuid.NewString())

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// set up the gadget area
		gadgetDir := filepath.Join(stateMachine.tempDirs.unpack, "gadget")
		err := osMkdirAll(gadgetDir, 0755)
		if err != nil && !os.IsExist(err) {
			t.Errorf("Error creating unpack directory: %s", err.Error())
		}
		// recursively copy the gadget tree to unpack/gadget
		gadgetTree := filepath.Join("testdata", "gadget_tree")
		files, err := ioutilReadDir(gadgetTree)
		if err != nil {
			t.Errorf("Error reading gadget tree: %s", err.Error())
		}
		for _, gadgetFile := range files {
			srcFile := filepath.Join(gadgetTree, gadgetFile.Name())
			if err := osutilCopySpecialFile(srcFile, gadgetDir); err != nil {
				t.Errorf("Error copying gadget tree: %s", err.Error())
			}
		}

		// need to load the gadget yaml
		stateMachine.yamlFilePath = filepath.Join(gadgetTree, "meta", "gadget.yaml")
		if err := stateMachine.loadGadgetYaml(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// now test that the bootfs contents exist
		if err := stateMachine.populateBootfsContents(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		for volumeName, volume := range stateMachine.gadgetInfo.Volumes {
			for i, structure := range volume.Structure {
				if structure.Role == gadget.SystemBoot {
					structDir := filepath.Join(stateMachine.tempDirs.volumes, volumeName,
						"part"+strconv.Itoa(i))
					_, err := os.Stat(structDir)
					if err != nil {
						t.Errorf("Directory %s should exist but does not", structDir)
					}
				}
			}
		}

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedPrepopulateBootfsContents tests a failure in stateMachine.prepopulateBootfsContents
func TestFailedPrepopulateBootfsContents(t *testing.T) {
	t.Run("test_failed_prepopulate_bootfs_contents", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.tempDirs.rootfs = filepath.Join("testdata", uuid.NewString())

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// need to load the gadget yaml
		stateMachine.yamlFilePath = filepath.Join("testdata", "gadget_tree", "meta", "gadget.yaml")
		if err := stateMachine.loadGadgetYaml(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		if err := stateMachine.populateBootfsContents(); err == nil {
			t.Errorf("Expected an error, but got none")
		}
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}
