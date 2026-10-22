//go:build linux

package agentscale

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

type serialPort struct {
	file    *os.File
	fd      int
	old     unix.Termios
	closeMu sync.Mutex
	closed  bool
}

func openSerial(path string, baud int) (io.ReadCloser, error) {
	speed, ok := baudConstant(baud)
	if !ok {
		return nil, fmt.Errorf("unsupported serial baud %d", baud)
	}
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	closeFD := func() { _ = unix.Close(fd) }
	if err = unix.IoctlSetInt(fd, unix.TIOCEXCL, 1); err != nil {
		closeFD()
		return nil, fmt.Errorf("exclusive open failed: %w", err)
	}
	old, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		_ = unix.IoctlSetInt(fd, unix.TIOCNXCL, 1)
		closeFD()
		return nil, fmt.Errorf("read serial settings failed: %w", err)
	}
	t := *old
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON | unix.IXOFF | unix.IXANY
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB | unix.CSTOPB | unix.CRTSCTS
	t.Cflag |= unix.CS8 | unix.CLOCAL | unix.CREAD
	t.Cc[unix.VMIN] = 0
	t.Cc[unix.VTIME] = 10 // 1s read timeout; cancellation still closes the fd
	t.Cflag = t.Cflag&^unix.CBAUD | speed
	t.Ispeed, t.Ospeed = speed, speed
	if err = unix.IoctlSetTermios(fd, unix.TCSETS, &t); err != nil {
		_ = unix.IoctlSetTermios(fd, unix.TCSETS, old)
		_ = unix.IoctlSetInt(fd, unix.TIOCNXCL, 1)
		closeFD()
		return nil, fmt.Errorf("configure serial port failed: %w", err)
	}
	return &serialPort{file: os.NewFile(uintptr(fd), path), fd: fd, old: *old}, nil
}

// Read waits at most one second for bytes. A timeout is reported separately so
// the monitor can keep checking cancellation and freshness without treating an
// idle instrument as a serial failure. Closing the port still unblocks it.
func (p *serialPort) Read(b []byte) (int, error) {
	for {
		fds := []unix.PollFd{{Fd: int32(p.fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, 1000)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return 0, err
		}
		if n == 0 {
			return 0, errReadTimeout
		}
		n, err = unix.Read(p.fd, b)
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
			continue
		}
		return n, err
	}
}

func (p *serialPort) Close() error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	// Restore the port settings before releasing the file descriptor so a
	// diagnostic tool or the next agent run sees the machine's original tty
	// configuration rather than our raw 9600 8N1 setup.
	_ = unix.IoctlSetTermios(p.fd, unix.TCSETS, &p.old)
	_ = unix.IoctlSetInt(p.fd, unix.TIOCNXCL, 1)
	return p.file.Close()
}

func baudConstant(baud int) (uint32, bool) {
	switch baud {
	case 1200:
		return unix.B1200, true
	case 2400:
		return unix.B2400, true
	case 4800:
		return unix.B4800, true
	case 9600:
		return unix.B9600, true
	case 19200:
		return unix.B19200, true
	case 38400:
		return unix.B38400, true
	case 57600:
		return unix.B57600, true
	case 115200:
		return unix.B115200, true
	default:
		return 0, false
	}
}
