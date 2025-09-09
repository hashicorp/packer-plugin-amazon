package chroot

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"path/filepath"

	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

type manualMountCommandData struct {
	Device string
}

// StepManualMountCommand sets up the a new block device when building from scratch
type StepManualMountCommand struct {
	Command   string
	mountPath string

	GeneratedData *packerbuilderdata.GeneratedData
}

func (s *StepManualMountCommand) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	device := state.Get("device").(string)
	ui := state.Get("ui").(packersdk.Ui)

	ui.Say("Running manual mount commands...")

	if config.NVMEDevicePath != "" {
		// customizable device path for mounting NVME block devices on c5 and m5 HVM
		device = config.NVMEDevicePath
	}
	ui.Say(fmt.Sprintf("Command is: %s", s.Command))
	if len(s.Command) == 0 {
		return multistep.ActionContinue
	}

	ictx := config.GetContext()
	ictx.Data = &manualMountCommandData{Device: filepath.Base(device)}
	mountPath, err := interpolate.Render(config.MountPath, &ictx)

	if err != nil {
		err := fmt.Errorf("Error preparing mount directory: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	mountPath, err = filepath.Abs(mountPath)
	if err != nil {
		err := fmt.Errorf("Error preparing mount directory: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	ui.Say(fmt.Sprintf("Mount Path After ABS is: %s", mountPath))

	log.Printf("Mount path: %s", mountPath)
	stderr := new(bytes.Buffer)

	wrappedCommand := state.Get("wrappedCommand").(common.CommandWrapper)

	ui.Say("Running manual mount commands...")
	mountCommand, err := wrappedCommand(fmt.Sprintf("%s %s", s.Command, mountPath))
	if err != nil {
		err := fmt.Errorf("Error creating mount command: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	cmd := common.ShellCommand(mountCommand)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		err := fmt.Errorf(
			"Error mounting root volume: %s\nStderr: %s", err, stderr.String())
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// Set the mount path so we remember to unmount it later
	s.mountPath = mountPath
	state.Put("mount_path", s.mountPath)
	s.GeneratedData.Put("MountPath", s.mountPath)
	state.Put("mount_device_cleanup", s)

	return multistep.ActionContinue
}

func (s *StepManualMountCommand) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packersdk.Ui)
	if err := s.CleanupFunc(state); err != nil {
		ui.Error(err.Error())
	}
}

func (s *StepManualMountCommand) CleanupFunc(state multistep.StateBag) error {
	if s.mountPath == "" {
		return nil
	}

	ui := state.Get("ui").(packersdk.Ui)

	ui.Say("Skipping UnMount Root Mount, it must be manually unmounted...")

	s.mountPath = ""
	return nil
}
