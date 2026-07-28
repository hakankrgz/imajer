package probe

type DiskIdentity struct {
	Path       string   `json:"path"`
	IDs        []string `json:"ids"`
	Serial     string   `json:"serial,omitempty"`
	Model      string   `json:"model,omitempty"`
	Size       int64    `json:"size"`
	SectorSize int64    `json:"sector_size"`
}

// mergeLinuxDeviceMetadata keeps every identity representation because
// Raspberry Pi SD/eMMC devices can expose a short lsblk serial and a full
// sysfs CID for the same physical medium.
func mergeLinuxDeviceMetadata(
	sysSerial, cid, sysModel, deviceName, sysWWID,
	lsblkModel, lsblkSerial, lsblkWWN string,
) (model, serial, wwid string, alternateIDs []string) {
	model = lsblkModel
	if model == "" {
		model = sysModel
	}
	if model == "" {
		model = deviceName
	}
	serial = lsblkSerial
	if serial == "" {
		serial = sysSerial
	}
	if serial == "" {
		serial = cid
	}
	wwid = lsblkWWN
	if wwid == "" {
		wwid = sysWWID
	}
	alternateIDs = []string{
		serial, sysSerial, lsblkSerial, cid, wwid, sysWWID, lsblkWWN,
	}
	return model, serial, wwid, alternateIDs
}
