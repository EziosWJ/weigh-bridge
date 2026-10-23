//go:build !linux

package d12sim

import "errors"

type terminal struct{ path string }

func openTerminal() (*terminal, error) {
	return nil, errors.New("D12 虚拟串口模拟器仅支持 Linux，请在 Linux 或 WSL 中运行")
}

func (t *terminal) readerPresent() (bool, error) { return false, nil }
func (t *terminal) write([]byte) error           { return nil }
func (t *terminal) close() error                 { return nil }
