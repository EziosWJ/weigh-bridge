//go:build linux

package d12sim

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

type terminal struct {
	fd   int
	path string
}

func openTerminal() (*terminal, error) {
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*terminal, error) {
		_ = unix.Close(fd)
		return nil, err
	}
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		return fail(err)
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		return fail(err)
	}
	path := fmt.Sprintf("/dev/pts/%d", n)
	slave, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fail(err)
	}
	defer unix.Close(slave)
	t, err := unix.IoctlGetTermios(slave, unix.TCGETS)
	if err != nil {
		return fail(err)
	}
	// Configure raw mode before any reader connects; never retain a slave fd.
	// POLLHUP on the master then tells us when there is no reader, so frames
	// cannot accumulate and later look like fresh measurements to an Agent.
	t.Iflag = 0
	t.Oflag = 0
	t.Lflag = 0
	t.Cflag = unix.CS8 | unix.CLOCAL | unix.CREAD | unix.B9600
	t.Ispeed, t.Ospeed = unix.B9600, unix.B9600
	t.Cc[unix.VMIN], t.Cc[unix.VTIME] = 1, 0
	if err := unix.IoctlSetTermios(slave, unix.TCSETS, t); err != nil {
		return fail(err)
	}
	return &terminal{fd: fd, path: path}, nil
}

func (t *terminal) readerPresent() (bool, error) {
	fds := []unix.PollFd{{Fd: int32(t.fd), Events: unix.POLLOUT}}
	if _, err := unix.Poll(fds, 0); err != nil {
		return false, err
	}
	if fds[0].Revents&unix.POLLNVAL != 0 {
		return false, unix.EBADF
	}
	if fds[0].Revents&unix.POLLHUP != 0 {
		// Flush the slave's input queue: flushing the master's output alone
		// leaves bytes already delivered to the slave line discipline intact.
		// Open only briefly; retaining this fd would hide reader disconnects.
		slave, err := unix.Open(t.path, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if errors.Is(err, unix.EBUSY) {
			// A reader acquired TIOCEXCL after Poll; retry on the next tick.
			return false, nil
		}
		if err != nil {
			return false, err
		}
		err = unix.IoctlSetInt(slave, unix.TCFLSH, unix.TCIFLUSH)
		_ = unix.Close(slave)
		if err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

func (t *terminal) write(frame []byte) error {
	n, err := unix.Write(t.fd, frame)
	if err != nil {
		return err
	}
	if n != len(frame) {
		return fmt.Errorf("串口只写入 %d/%d 字节", n, len(frame))
	}
	return nil
}

func (t *terminal) close() error { return unix.Close(t.fd) }
