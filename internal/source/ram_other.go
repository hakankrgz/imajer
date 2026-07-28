//go:build !linux && !windows

package source

import (
	"context"
	"errors"
)

func openRAM(context.Context, string, string, string) (*Handle, error) {
	return nil, errors.New("RAM acquisition provider is not supported on this platform")
}
