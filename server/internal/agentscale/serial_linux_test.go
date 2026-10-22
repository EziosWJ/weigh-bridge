//go:build linux

package agentscale

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const (
	tiocgptn   = 0x80045430
	tiocsptlck = 0x40045431
)

func openPTY(t *testing.T) (master *os.File, slavePath string, original *unix.Termios) {
	t.Helper()
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = unix.IoctlSetPointerInt(fd, tiocsptlck, 0); err != nil {
		unix.Close(fd)
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(fd, tiocgptn)
	if err != nil {
		unix.Close(fd)
		t.Fatal(err)
	}
	slavePath = fmt.Sprintf("/dev/pts/%d", n)
	sfd, err := unix.Open(slavePath, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		unix.Close(fd)
		t.Fatal(err)
	}
	t.Cleanup(func() { unix.Close(sfd); unix.Close(fd) })
	original, err = unix.IoctlGetTermios(sfd, unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	return os.NewFile(uintptr(fd), "ptmx"), slavePath, original
}

func TestSerialPortUsesRawExclusiveSettingsAndRestoresOnClose(t *testing.T) {
	master, path, original := openPTY(t)
	port, err := openSerial(path, 9600)
	if err != nil {
		t.Fatal(err)
	}
	sp := port.(*serialPort)
	termios, err := unix.IoctlGetTermios(sp.fd, unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	if termios.Cflag&unix.CSIZE != unix.CS8 || termios.Cflag&unix.CREAD == 0 || termios.Cflag&unix.CLOCAL == 0 {
		t.Fatalf("not raw 8N1/CLOCAL: %#x", termios.Cflag)
	}
	if termios.Cflag&(unix.PARENB|unix.CSTOPB|unix.CRTSCTS) != 0 {
		t.Fatalf("parity/stop/flow control left enabled: %#x", termios.Cflag)
	}
	if termios.Lflag&(unix.ICANON|unix.ECHO) != 0 || termios.Iflag&(unix.IXON|unix.IXOFF) != 0 {
		t.Fatalf("canonical mode or software flow control left enabled: iflag=%#x lflag=%#x", termios.Iflag, termios.Lflag)
	}
	if termios.Cflag&unix.CBAUD != unix.B9600 && (termios.Ispeed != unix.B9600 || termios.Ospeed != unix.B9600) {
		t.Fatalf("speed cflag=%#x ispeed=%#x ospeed=%#x", termios.Cflag&unix.CBAUD, termios.Ispeed, termios.Ospeed)
	}
	if termios.Cc[unix.VMIN] != 0 || termios.Cc[unix.VTIME] != 10 {
		t.Fatalf("read timeout vmin=%d vtime=%d", termios.Cc[unix.VMIN], termios.Cc[unix.VTIME])
	}
	if _, err := openSerial(path, 9600); err == nil {
		t.Fatal("second exclusive open succeeded")
	}
	buf := make([]byte, 64)
	start := time.Now()
	if n, err := sp.Read(buf); n != 0 || !errors.Is(err, errReadTimeout) {
		t.Fatalf("idle read returned n=%d err=%v", n, err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond || elapsed > 2*time.Second {
		t.Fatalf("idle read timeout took %v", elapsed)
	}
	frame := makeFrame(0x2d, 0x30, 0x20, "001550", "000000")
	go func() { _, _ = master.Write(frame) }()
	n, err := sp.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf[:n], frame) {
		t.Fatalf("read %x want %x", buf[:n], frame)
	}
	if err := port.Close(); err != nil {
		t.Fatal(err)
	}
	checkFD, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := unix.IoctlGetTermios(checkFD, unix.TCGETS)
	unix.Close(checkFD)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Cflag != original.Cflag || restored.Lflag != original.Lflag || restored.Iflag != original.Iflag {
		t.Fatalf("termios not restored: got c=%#x l=%#x i=%#x want c=%#x l=%#x i=%#x", restored.Cflag, restored.Lflag, restored.Iflag, original.Cflag, original.Lflag, original.Iflag)
	}
	if _, err := openSerial(path, 9600); err != nil {
		t.Fatalf("reopen after close failed: %v", err)
	}
}

func TestSerialOpenErrorsAreReported(t *testing.T) {
	if _, err := openSerial("/definitely/not/a/tty", 9600); err == nil {
		t.Fatal("missing device accepted")
	}
	if _, err := openSerial("/dev/null", 9600); err == nil {
		if _, ok := err.(*os.PathError); !ok && !errors.Is(err, unix.ENOTTY) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if _, err := openSerial("/dev/null", 1234); err == nil {
		t.Fatal("invalid baud accepted")
	}
}
