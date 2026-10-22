//go:build !linux

package agentscale

import (
	"errors"
	"io"
)

func openSerial(string, int) (io.ReadCloser, error) {
	return nil, errors.New("serial weighing is only implemented on Linux")
}
