//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Only private PTYs are opened; no device discovery or physical serial access.
func privateSerialPTY(t *testing.T) (*os.File, string) {
	t.Helper()
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	master := os.NewFile(uintptr(fd), "private-ptmx")
	t.Cleanup(func() { master.Close() })
	if err = unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	return master, fmt.Sprintf("/dev/pts/%d", n)
}

func TestLinuxSerialRejectsNonpollable(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatal(err)
	}
	name := file.Name()
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	p, err := openSerialPort(name)
	if err == nil {
		p.Close()
		t.Fatal("opened a nonpollable file")
	}
	if !errors.Is(err, os.ErrNoDeadline) {
		t.Fatalf("open nonpollable = %v; want ErrNoDeadline", err)
	}
	if p != nil {
		t.Fatal("failed open returned a non-nil port")
	}
}

func openPrivateSerial(t *testing.T) (*os.File, *linuxSerialPort, string) {
	t.Helper()
	master, path := privateSerialPTY(t)
	p, err := openLinuxSerial(path, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	return master, p, path
}

func TestLinuxSerialCloseInterruptsReadWrite(t *testing.T) {
	_, p, _ := openPrivateSerial(t)
	readDone := make(chan error, 1)
	writeDone := make(chan error, 1)
	go func() { _, err := p.Read(make([]byte, 1)); readDone <- err }()
	go func() { _, err := p.Write(make([]byte, 4<<20)); writeDone <- err }()
	for _, done := range []<-chan error{readDone, writeDone} {
		select {
		case err := <-done:
			t.Fatalf("I/O returned before Close: %v", err)
		case <-time.After(50 * time.Millisecond):
		}
	}
	start := time.Now()
	closeDone := make(chan error, 2)
	go func() { closeDone <- p.Close() }()
	go func() { closeDone <- p.Close() }()
	for _, done := range []<-chan error{readDone, writeDone, closeDone, closeDone} {
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, os.ErrClosed) {
				t.Fatalf("Close/I/O error: %v", err)
			}
		case <-time.After(300 * time.Millisecond):
			t.Fatal("Close did not interrupt pending I/O promptly")
		}
	}
	t.Logf("concurrent Close/read/write finished in %s", time.Since(start))
}

// Saturate the private slave without ever consuming the master's input. Repeat
// after tty-worker settling, since one EAGAIN can precede a queue transfer.
func fillPrivateSerial(t *testing.T, p *linuxSerialPort) {
	t.Helper()
	raw, err := p.file.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		var writeErr error
		err = raw.Control(func(fd uintptr) {
			for {
				_, writeErr = unix.Write(int(fd), make([]byte, 1024))
				if writeErr != nil {
					break
				}
			}
			if errors.Is(writeErr, unix.EAGAIN) {
				for {
					_, writeErr = unix.Write(int(fd), []byte{0})
					if writeErr != nil {
						break
					}
				}
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if !errors.Is(writeErr, unix.EAGAIN) {
			t.Fatal(writeErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type observedSerialWrite struct {
	*linuxSerialPort
	started  chan struct{}
	returned chan struct{}
}

func (p observedSerialWrite) Write(b []byte) (int, error) {
	close(p.started)
	n, err := p.linuxSerialPort.Write(b)
	close(p.returned)
	return n, err
}

func TestLinuxSerialSessionCancellation(t *testing.T) {
	for _, blockedWrite := range []bool{false, true} {
		t.Run(fmt.Sprintf("blockedWrite=%t", blockedWrite), func(t *testing.T) {
			_, p, _ := openPrivateSerial(t)
			if blockedWrite {
				fillPrivateSerial(t, p)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			observed := observedSerialWrite{p, make(chan struct{}), make(chan struct{})}
			c := &Controller{Backend: &memoryBackend{state: State{Workspace: 2, Connected: true}}, Max: 10}
			done := make(chan error, 1)
			go func() { done <- serveSession(ctx, observed, c, nil, time.Hour) }()
			select {
			case <-observed.started:
			case <-time.After(time.Second):
				t.Fatal("session did not start writing")
			}
			if blockedWrite {
				select {
				case <-observed.returned:
					t.Fatal("initial session Write was not blocked")
				case <-time.After(50 * time.Millisecond):
				}
			} else {
				select {
				case <-observed.returned:
				case <-time.After(time.Second):
					t.Fatal("initial session Write did not finish")
				}
			}
			start := time.Now()
			cancel()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("cancelled session returned nil")
				}
				t.Logf("session cancellation finished in %s: %v", time.Since(start), err)
			case <-time.After(300 * time.Millisecond):
				t.Fatal("private PTY session ignored cancellation")
			}
		})
	}
}

func TestLinuxSerialSessionWriteWatchdog(t *testing.T) {
	_, p, _ := openPrivateSerial(t)
	fillPrivateSerial(t, p)
	observed := observedSerialWrite{p, make(chan struct{}), make(chan struct{})}
	c := &Controller{Backend: &memoryBackend{state: State{Workspace: 2, Connected: true}}, Max: 10}
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- serveSession(context.Background(), observed, c, nil, time.Hour) }()
	select {
	case err := <-done:
		if !errors.Is(err, os.ErrDeadlineExceeded) && !errors.Is(err, os.ErrClosed) {
			t.Fatalf("session watchdog error = %v", err)
		}
		elapsed := time.Since(start)
		if elapsed < 900*time.Millisecond || elapsed > 2*time.Second {
			t.Fatalf("session watchdog elapsed %s", elapsed)
		}
		t.Logf("unread private PTY session stopped after %s: %v", elapsed, err)
	case <-time.After(2 * time.Second):
		t.Fatal("unread private PTY session ignored write watchdog")
	}
}

func TestLinuxSerialSettingsAndExclusiveRelease(t *testing.T) {
	_, p, path := openPrivateSerial(t)
	raw, err := p.file.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var settings *unix.Termios
	var flags, fdFlags, exclusive int
	var inspectErr error
	err = raw.Control(func(fd uintptr) {
		settings, inspectErr = unix.IoctlGetTermios(int(fd), unix.TCGETS)
		if inspectErr != nil {
			return
		}
		flags, inspectErr = unix.FcntlInt(fd, unix.F_GETFL, 0)
		if inspectErr != nil {
			return
		}
		fdFlags, inspectErr = unix.FcntlInt(fd, unix.F_GETFD, 0)
		if inspectErr != nil {
			return
		}
		exclusive, inspectErr = unix.IoctlGetInt(int(fd), unix.TIOCGEXCL)
	})
	if err != nil || inspectErr != nil {
		t.Fatal(err, inspectErr)
	}
	if settings.Iflag != 0 || settings.Oflag != 0 || settings.Lflag != 0 ||
		settings.Cflag != unix.B115200|unix.CS8|unix.CLOCAL|unix.CREAD ||
		settings.Cc[unix.VMIN] != 1 || settings.Cc[unix.VTIME] != 0 {
		t.Fatalf("not raw 115200 8N1 without flow control: %+v", settings)
	}
	if flags&unix.O_NONBLOCK == 0 || fdFlags&unix.FD_CLOEXEC == 0 || exclusive != 1 {
		t.Fatalf("nonblock=%x fdflags=%x exclusive=%d", flags, fdFlags, exclusive)
	}
	if err = p.Close(); err != nil {
		t.Fatal(err)
	}
	// The master is deliberately still open: kernel tty lifetime alone does
	// not release TIOCEXCL. Successful reopen proves explicit TIOCNXCL.
	reopened, err := openLinuxSerial(path, true)
	if err != nil {
		t.Fatalf("exclusive access not released: %v", err)
	}
	reopened.Close()
}

func TestLinuxSerialProductionRejectsMissingModemIOCTL(t *testing.T) {
	_, path := privateSerialPTY(t)
	p, err := openSerialPort(path)
	if err == nil {
		p.Close()
		t.Fatal("production bypassed required modem controls")
	}
	if p != nil || !errors.Is(err, unix.ENOTTY) {
		t.Fatalf("production modem failure = %v, %v", p, err)
	}
	// Failed setup must also release exclusivity with the master still open.
	reopened, err := openLinuxSerial(path, true)
	if err != nil {
		t.Fatal(err)
	}
	reopened.Close()
}

func TestLinuxSerialWriteDeadline(t *testing.T) {
	_, path := privateSerialPTY(t) // Peer intentionally never reads.
	p, err := openLinuxSerial(path, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	type result struct {
		n       int
		err     error
		elapsed time.Duration
	}
	done := make(chan result, 1)
	start := time.Now()
	go func() {
		n, err := p.Write(make([]byte, 4<<20)) // Much larger than a PTY queue.
		done <- result{n, err, time.Since(start)}
	}()
	select {
	case r := <-done:
		if !errors.Is(r.err, os.ErrDeadlineExceeded) {
			t.Fatalf("Write = %d, %v; want deadline", r.n, r.err)
		}
		if r.n <= 0 || r.n >= 4<<20 {
			t.Fatalf("partial write count = %d", r.n)
		}
		if r.elapsed < 900*time.Millisecond || r.elapsed > 2*time.Second {
			t.Fatalf("deadline elapsed %s", r.elapsed)
		}
		t.Logf("unread private PTY: %d bytes then deadline after %s", r.n, r.elapsed)
	case <-time.After(2 * time.Second):
		t.Fatal("unread private PTY Write exceeded two seconds")
	}
}
