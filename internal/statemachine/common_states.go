package statemachine

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/google/uuid"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/osutil"
)

// generate work directory file structure
func (stateMachine *StateMachine) makeTemporaryDirectories() error {
	// if no workdir was specified, open a /tmp dir
	if stateMachine.stateMachineFlags.WorkDir == "" {
		stateMachine.stateMachineFlags.WorkDir = filepath.Join("/tmp", "ubuntu-image-"+uuid.NewString())
		if err := osMkdir(stateMachine.stateMachineFlags.WorkDir, 0755); err != nil {
			return fmt.Errorf("Failed to create temporary directory: %s", err.Error())
		}
		stateMachine.cleanWorkDir = true
	} else {
		err := osMkdirAll(stateMachine.stateMachineFlags.WorkDir, 0755)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("Error creating work directory: %s", err.Error())
		}
	}

	stateMachine.tempDirs.rootfs = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "root")
	stateMachine.tempDirs.unpack = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "unpack")
	stateMachine.tempDirs.volumes = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "volumes")

	if err := osMkdir(stateMachine.tempDirs.rootfs, 0755); err != nil {
		return fmt.Errorf("Error creating temporary directory: %s", err.Error())
	}

	return nil
}

// Load the gadget yaml passed in via command line
func (stateMachine *StateMachine) loadGadgetYaml() error {
	if err := osutilCopySpecialFile(stateMachine.yamlFilePath,
		stateMachine.stateMachineFlags.WorkDir); err != nil {
		return fmt.Errorf("Error loading gadget.yaml: %s", err.Error())
	}

	// read in the gadget.yaml as bytes, because snapd expects it that way
	gadgetYamlBytes, err := ioutilReadFile(stateMachine.yamlFilePath)
	if err != nil {
		return fmt.Errorf("Error loading gadget.yaml: %s", err.Error())
	}

	stateMachine.gadgetInfo, err = gadget.InfoFromGadgetYaml(gadgetYamlBytes, nil)
	if err != nil {
		return fmt.Errorf("Error loading gadget.yaml: %s", err.Error())
	}

	var rootfsSeen bool = false
	var farthestOffset quantity.Offset = 0
	var lastOffset quantity.Offset = 0
	for volumeName, volume := range stateMachine.gadgetInfo.Volumes {
		volumeBaseDir := filepath.Join(stateMachine.tempDirs.volumes, volumeName)
		if err := osMkdirAll(volumeBaseDir, 0755); err != nil {
			return fmt.Errorf("Error creating volume dir: %s", err.Error())
		}
		// look for the rootfs and check if the image is seeded
		for ii, structure := range volume.Structure {
			if structure.Role == "" && structure.Label == gadget.SystemBoot {
				fmt.Printf("WARNING: volumes:%s:structure:%d:filesystem_label "+
					"used for defining partition roles; use role instead\n",
					volumeName, ii)
			} else if structure.Role == gadget.SystemData {
				rootfsSeen = true
			} else if structure.Role == gadget.SystemSeed {
				stateMachine.isSeeded = true
				stateMachine.hooksAllowed = false
			}

			fmt.Println(structure)
			// update farthestOffset if needed
			var offset quantity.Offset
			if structure.Offset == nil {
				if structure.Role != "mbr" && lastOffset < quantity.OffsetMiB {
					offset = quantity.OffsetMiB
				} else {
					offset = lastOffset
				}
			} else {
				offset = *structure.Offset
			}
			lastOffset = offset + quantity.Offset(structure.Size)
			farthestOffset = helper.MaxOffset(farthestOffset, lastOffset)
		}
	}

	if !rootfsSeen && len(stateMachine.gadgetInfo.Volumes) == 1 {
		// We still need to handle the case of unspecified system-data
		// partition where we simply attach the rootfs at the end of the
		// partition list.
		//
		// Since so far we have no knowledge of the rootfs contents, the
		// size is set to 0, and will be calculated later
		rootfsStructure := gadget.VolumeStructure{
			Name:        "",
			Label:       "writable",
			Offset:      &farthestOffset,
			OffsetWrite: new(gadget.RelativeOffset),
			Size:        quantity.Size(0),
			Type:        "83,0FC63DAF-8483-4772-8E79-3D69D8477DE4",
			Role:        gadget.SystemData,
			ID:          "",
			Filesystem:  "ext4",
			Content:     []gadget.VolumeContent{},
			Update:      gadget.VolumeUpdate{},
		}

		// TODO: un-hardcode this
		stateMachine.gadgetInfo.Volumes["pc"].Structure =
			append(stateMachine.gadgetInfo.Volumes["pc"].Structure, rootfsStructure)
	}

	// check if the unpack dir should be preserved
	envar := os.Getenv("UBUNTU_IMAGE_PRESERVE_UNPACK")
	if envar != "" {
		preserveDir := filepath.Join(envar, "unpack")
		if err := osutilCopySpecialFile(stateMachine.tempDirs.unpack, preserveDir); err != nil {
			return fmt.Errorf("Error preserving unpack dir: %s", err.Error())
		}
	}

	return nil
}

// Run hooks for populating rootfs contents
func (stateMachine *StateMachine) populateRootfsContentsHooks() error {
	if !stateMachine.hooksAllowed {
		if stateMachine.commonFlags.Debug {
			fmt.Println("Building from a seeded gadget - " +
				"skipping the post-populate-rootfs hook execution: unsupported")
		}
		return nil
	}

	if len(stateMachine.commonFlags.HooksDirectories) == 0 {
		// no hooks, move on
		return nil
	}

	err := stateMachine.runHooks("post-populate-rootfs",
		"UBUNTU_IMAGE_HOOK_ROOTFS", stateMachine.tempDirs.rootfs)
	if err != nil {
		return err
	}

	return nil
}

// Generate the disk info
func (stateMachine *StateMachine) generateDiskInfo() error {
	if stateMachine.commonFlags.DiskInfo != "" {
		diskInfoDir := filepath.Join(stateMachine.tempDirs.rootfs, ".disk")
		if err := osMkdir(diskInfoDir, 0755); err != nil {
			return fmt.Errorf("Failed to create disk info directory: %s", err.Error())
		}
		diskInfoFile := filepath.Join(diskInfoDir, "info")
		err := osutilCopyFile(stateMachine.commonFlags.DiskInfo, diskInfoFile, osutil.CopyFlagDefault)
		if err != nil {
			return fmt.Errorf("Failed to copy Disk Info file: %s", err.Error())
		}
	}
	return nil
}

// Calculate the size of the root filesystem
// on a 100MiB filesystem, ext4 takes a little over 7MiB for the
// metadata. Use 8MB as a minimum padding here
func (stateMachine *StateMachine) calculateRootfsSize() error {
	rootfsSize, err := helper.Du(stateMachine.tempDirs.rootfs)
	if err != nil {
		return fmt.Errorf("Error getting rootfs size: %s", err.Error())
	}
	var rootfsQuantity quantity.Size = rootfsSize
	rootfsPadding := 8 * quantity.SizeMiB
	rootfsQuantity += rootfsPadding

	// fudge factor for incidentals
	rootfsQuantity += (rootfsQuantity / 2)

	stateMachine.rootfsSize = rootfsQuantity
	return nil
}

// Pre populate the bootfs contents
func (stateMachine *StateMachine) populateBootfsContents() error {
	// find the name of the system volume
	var systemVolumeName string
	var systemVolume *gadget.Volume
	for volumeName, volume := range stateMachine.gadgetInfo.Volumes {
		for _, structure := range volume.Structure {
			// use the system-boot role to identify the system volume
			fmt.Printf("JAWN structure.Role = %s\n", structure.Role)
			fmt.Printf("%v", structure)
			if structure.Role == gadget.SystemBoot || structure.Label == gadget.SystemBoot {
				systemVolumeName = volumeName
				systemVolume = volume
			}
		}
	}
	fmt.Printf("JAWN systemVolumeName = %s\n", systemVolumeName)

	// now call LayoutVolume to get a LaidOutVolume we can use
	// with a mountedFilesystemWriter
	layoutConstraints := gadget.LayoutConstraints{SkipResolveContent: false}
	laidOutVolume, err := gadget.LayoutVolume(
		filepath.Join(stateMachine.tempDirs.unpack, "gadget"),
		filepath.Join(stateMachine.tempDirs.unpack, "kernel"),
		systemVolume, layoutConstraints)
	if err != nil {
		return fmt.Errorf("Error laying out bootfs contents: %s", err.Error())
	}

	for ii, laidOutStructure := range laidOutVolume.LaidOutStructure {
		if laidOutStructure.HasFilesystem() {
			mountedFilesystemWriter, err := gadget.NewMountedFilesystemWriter(&laidOutStructure, nil)
			if err != nil {
				return fmt.Errorf("Error creating NewMountedFilesystemWriter: %s", err.Error())
			}

			var targetDir string
			if laidOutStructure.Role == "system-seed" {
				targetDir = stateMachine.tempDirs.rootfs
			} else {
				targetDir = filepath.Join(stateMachine.tempDirs.volumes,
					systemVolumeName,
					"part"+strconv.Itoa(ii))
			}
			err = mountedFilesystemWriter.Write(targetDir, []string{})
			if err != nil {
				return fmt.Errorf("Error in mountedFilesystem.Write(): %s", err.Error())
			}
		}
	}
	return nil
}

// TODO: gocyclo is going to hate this. break it up
// Populate and prepare the partitions
func (stateMachine *StateMachine) populatePreparePartitions() error {
	// initialize the size map
	stateMachine.imageSizes = make(map[string]quantity.Size)

	// now iterate through all the volumes
	for volumeName, volume := range stateMachine.gadgetInfo.Volumes {
		if volume.Bootloader == "lk" {
			// For the LK bootloader we need to copy boot.img and snapbootsel.bin to
			// the gadget folder so they can be used as partition content. The first
			// one comes from the kernel snap, while the second one is modified by
			// the prepare_image step to set the right core and kernel for the kernel
			// command line.
			bootDir := filepath.Join(stateMachine.tempDirs.unpack,
				"image", "boot", "lk")
			gadgetDir := filepath.Join(stateMachine.tempDirs.unpack, "gadget")
			if _, err := os.Stat(bootDir); err == nil {
				err := osMkdir(gadgetDir, 0755)
				if err != nil && !os.IsExist(err) {
					return fmt.Errorf("Failed to create gadget dir: %s", err.Error())
				}
				files, err := ioutilReadDir(bootDir)
				if err != nil {
					return fmt.Errorf("Error reading lk bootloader dir: %s", err.Error())
				}
				for _, lkFile := range files {
					srcFile := filepath.Join(bootDir, lkFile.Name())
					if err := osutilCopySpecialFile(srcFile, gadgetDir); err != nil {
						return fmt.Errorf("Error copying lk bootloader dir: %s", err.Error())
					}
				}
			}
		}
		var farthestOffset quantity.Offset = 0
		// TODO: change these all from partition to structure
		for partitionNumber, partition := range volume.Structure {
			if partition.Role == gadget.SystemData || partition.Role == gadget.SystemSeed {
				// system-data and system-seed partitions are not required to have
				// an explicit size set in the yaml file
				if partition.Size == 0 {
					partition.Size = stateMachine.rootfsSize
				} else if partition.Size < stateMachine.rootfsSize {
					fmt.Printf("WARNING: rootfs partition size %s smaller"+
						"than actual rootfs contents %s\n",
						partition.Size.IECString(),
						stateMachine.rootfsSize.IECString())
					// TODO: I think this is a local copy and has no effect on the actual partition
					partition.Size = stateMachine.rootfsSize
				}
			}
			var offset quantity.Offset
			if partition.Offset != nil {
				offset = *partition.Offset
			} else {
				offset = 0
			}
			farthestOffset = helper.MaxOffset(farthestOffset,
				quantity.Offset(partition.Size) + offset)
			if stateMachine.isSeeded &&
				(partition.Role == gadget.SystemBoot ||
					partition.Role == gadget.SystemData ||
					partition.Role == gadget.SystemSave ||
					partition.Label == gadget.SystemBoot) {
				continue
			}
			partImg := filepath.Join(stateMachine.tempDirs.volumes, volumeName,
				"part"+strconv.Itoa(partitionNumber)+".img")
			var contentRoot string
			if partition.Role == gadget.SystemSeed || partition.Role == gadget.SystemData {
				contentRoot = stateMachine.tempDirs.rootfs
			} else {
				contentRoot = filepath.Join(stateMachine.tempDirs.volumes, volumeName,
					"part"+strconv.Itoa(partitionNumber))
			}
			if partition.Filesystem == "" {
				// copy the contents to the new location
				var runningOffset quantity.Offset = 0
				for _, content := range partition.Content {
					if content.Offset != nil {
						runningOffset = *content.Offset
					}
					// first zero it out
					ddArgs := []string{"if=/dev/zero", "of=" + partImg, "count=0",
						"bs=" + strconv.FormatUint(uint64(partition.Size), 10),
						"seek=1"}
					if err := helper.CopyBlob(ddArgs); err != nil {
						return fmt.Errorf("Error copying image blob: %s",
							err.Error())
					}

					// now write the real value
					inFile := filepath.Join(stateMachine.tempDirs.unpack,
						"gadget", content.Image)
					ddArgs = []string{"if=" + inFile, "of=" + partImg, "bs=1",
						"seek=" + strconv.FormatUint(uint64(runningOffset), 10),
						"conv=sparse,notrunc"}
					if err := helper.CopyBlob(ddArgs); err != nil {
						return fmt.Errorf("Error copying image blob: %s",
							err.Error())
					}
					runningOffset += quantity.Offset(content.Size)
				}
			} else {
				ddArgs := []string{"if=/dev/zero", "of=" + partImg, "count=0",
					"bs=" + strconv.FormatUint(uint64(partition.Size), 10), "seek=1"}
				if err := helper.CopyBlob(ddArgs); err != nil {
					return fmt.Errorf("Error zeroing image file %s: %s",
						partImg, err.Error())
				}
				err := helper.MkfsWithContent(partition.Filesystem, partImg, partition.Label,
					contentRoot, partition.Size, quantity.Size(512))
				if err != nil {
					return fmt.Errorf("Error running mkfs: %s", err.Error())
				}
			}
		}
		// store volume sizes in the stateMachineStruct. These will be used during
		// the make_image step
		// TODO: check this is accurate
		calculated := quantity.Size((farthestOffset / quantity.OffsetMiB + 17) * quantity.OffsetMiB)
		volumeString, found := stateMachine.commonFlags.Size[volumeName]
		if !found {
			stateMachine.imageSizes[volumeName] = calculated
		} else {
			fmt.Printf("JAWN found %s with size %s\n", volumeName, volumeString)
			volumeSize, err := quantity.ParseSize(volumeString)
			if err != nil {
				return fmt.Errorf("Failed to parse volume size %s: %s",
					volumeString, err.Error())
			}
			if volumeSize < calculated {
				fmt.Printf("WARNING: ignoring image size smaller than " +
					"minimum required size: vol:%s %d < %d",
					volumeName, uint64(volumeSize), uint64(calculated))
				stateMachine.imageSizes[volumeName] = calculated
			} else {
				stateMachine.imageSizes[volumeName] = volumeSize
			}
		}
	}
	return nil
}

// Make the disk
func (stateMachine *StateMachine) makeDisk() error {
	return nil
}

// Generate the manifest
func (stateMachine *StateMachine) generateManifest() error {
	return nil
}

// Clean up and organize files
func (stateMachine *StateMachine) finish() error {
	if stateMachine.cleanWorkDir {
		if err := stateMachine.cleanup(); err != nil {
			return err
		}
	}
	return nil
}
