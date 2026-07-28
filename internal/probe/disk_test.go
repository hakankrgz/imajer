package probe

import (
	"slices"
	"testing"
)

func TestMergeLinuxDeviceMetadataKeepsRaspberryPiSerialAndCID(t *testing.T) {
	model, serial, wwid, ids := mergeLinuxDeviceMetadata(
		"", "1b534d30303000000000abcd12345678",
		"", "SC64G", "",
		"SD Card", "0x12345678", "",
	)
	if model != "SD Card" || serial != "0x12345678" || wwid != "" {
		t.Fatalf("unexpected primary metadata: model=%q serial=%q wwid=%q", model, serial, wwid)
	}
	for _, expected := range []string{"0x12345678", "1b534d30303000000000abcd12345678"} {
		if !slices.Contains(ids, expected) {
			t.Fatalf("alternate identity %q is missing from %#v", expected, ids)
		}
	}
}
