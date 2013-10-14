package devmapper

func (devices *DeviceSetDM) MountDevice(hash, path string) error {
	panic("Unimplemented on darwin")
}

func (devices *DeviceSetDM) UnmountDevice(hash, path string, deactivate bool) error {
	panic("Unimplemented on darwin")
}
