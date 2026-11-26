//go:build !linux

package serialport

import "errors"

type Config struct {
	Device   string
	BaudRate int
	DataBits int
	StopBits int
	Parity   string
}

type Port struct{}

func Open(Config) (*Port, error) {
	return nil, errors.New("串口扫码枪仅支持 Linux 部署机")
}

func (p *Port) Read([]byte) (int, error) { return 0, errors.New("当前平台不支持串口") }
func (p *Port) Close() error             { return nil }
