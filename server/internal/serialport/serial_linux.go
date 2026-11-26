//go:build linux

package serialport

import (
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

// Config is the raw serial profile used by the QR scanner. The scanner sends
// text frames; no hardware handshake is required.
type Config struct {
	Device   string
	BaudRate int
	DataBits int
	StopBits int
	Parity   string
}

type Port struct {
	fd     int
	closed atomic.Bool
}

func Open(cfg Config) (*Port, error) {
	if cfg.Device == "" {
		return nil, errors.New("串口设备未配置")
	}
	fd, err := unix.Open(cfg.Device, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("打开串口 %s 失败：%w", cfg.Device, err)
	}
	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("读取串口参数失败：%w", err)
	}
	speed, err := baudConstant(cfg.BaudRate)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	bits, err := dataBitsConstant(cfg.DataBits)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	termios.Iflag = unix.IGNPAR
	termios.Oflag = 0
	termios.Cflag = unix.CREAD | unix.CLOCAL | speed | bits
	switch cfg.Parity {
	case "", "none":
	case "even":
		termios.Cflag |= unix.PARENB
	case "odd":
		termios.Cflag |= unix.PARENB | unix.PARODD
	default:
		_ = unix.Close(fd)
		return nil, errors.New("串口校验位无效")
	}
	if cfg.StopBits == 2 {
		termios.Cflag |= unix.CSTOPB
	} else if cfg.StopBits != 0 && cfg.StopBits != 1 {
		_ = unix.Close(fd)
		return nil, errors.New("串口停止位无效")
	}
	termios.Lflag = 0
	termios.Cc[unix.VMIN] = 0
	termios.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, termios); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("设置串口参数失败：%w", err)
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("设置串口非阻塞模式失败：%w", err)
	}
	return &Port{fd: fd}, nil
}

func (p *Port) Read(buf []byte) (int, error) {
	for {
		if p.closed.Load() {
			return 0, io.EOF
		}
		pollFDs := []unix.PollFd{{Fd: int32(p.fd), Events: unix.POLLIN}}
		n, err := unix.Poll(pollFDs, 200)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return 0, err
		}
		if n == 0 {
			continue
		}
		if p.closed.Load() {
			return 0, io.EOF
		}
		if pollFDs[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return 0, io.EOF
		}
		count, err := unix.Read(p.fd, buf)
		if err == nil {
			return count, nil
		}
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
			continue
		}
		return 0, err
	}
}

func (p *Port) Close() error {
	if p == nil || p.closed.Swap(true) {
		return nil
	}
	return unix.Close(p.fd)
}

func baudConstant(value int) (uint32, error) {
	switch value {
	case 1200:
		return unix.B1200, nil
	case 2400:
		return unix.B2400, nil
	case 4800:
		return unix.B4800, nil
	case 9600:
		return unix.B9600, nil
	case 19200:
		return unix.B19200, nil
	case 38400:
		return unix.B38400, nil
	case 57600:
		return unix.B57600, nil
	case 115200:
		return unix.B115200, nil
	default:
		return 0, errors.New("串口波特率无效")
	}
}

func dataBitsConstant(value int) (uint32, error) {
	switch value {
	case 5:
		return unix.CS5, nil
	case 6:
		return unix.CS6, nil
	case 7:
		return unix.CS7, nil
	case 8, 0:
		return unix.CS8, nil
	default:
		return 0, errors.New("串口数据位无效")
	}
}
