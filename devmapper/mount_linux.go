package devmapper

import (
	"github.com/dotcloud/docker/utils"
	"syscall"
)

func (devices *DeviceSetDM) MountDevice(hash, path string) error {
	devices.Lock()
	defer devices.Unlock()

	if err := devices.ensureInit(); err != nil {
		utils.Debugf("\n--->Err: %s\n", err)
		return err
	}

	if err := devices.activateDeviceIfNeeded(hash); err != nil {
		utils.Debugf("\n--->Err: %s\n", err)
		return err
	}

	info := devices.Devices[hash]
	err := syscall.Mount(info.DevName(), path, "ext4", syscall.MS_MGC_VAL, "discard")
	if err != nil && err == syscall.EINVAL {
		err = syscall.Mount(info.DevName(), path, "ext4", syscall.MS_MGC_VAL, "")
	}
	if err != nil {
		utils.Debugf("\n--->Err: %s\n", err)
		return err
	}

	count := devices.activeMounts[path]
	devices.activeMounts[path] = count + 1

	return nil
}

func (devices *DeviceSetDM) UnmountDevice(hash, path string, deactivate bool) error {
	devices.Lock()
	defer devices.Unlock()

	if err := syscall.Unmount(path, 0); err != nil {
		utils.Debugf("\n--->Err: %s\n", err)
		return err
	}

	if count := devices.activeMounts[path]; count > 1 {
		devices.activeMounts[path] = count - 1
	} else {
		delete(devices.activeMounts, path)
	}

	if deactivate {
		devices.deactivateDevice(hash)
	}

	return nil
}
