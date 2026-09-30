//go:build linux

package jobs

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/summant/asp/internal/source"
)

// The job-control tests need a controlling terminal, which `go test` does
// not have. TestHost re-runs this test binary as a helper in a new session
// whose controlling terminal is a fresh pseudo-terminal, exactly as asp
// runs inside kitty, and drives it from the pty's other end.

const helperEnv = "ASP_JOBS_HELPER"

// fakeAgent pauses itself once (as ctrl+z would), reports its ignored
// signals, then waits for a line or an interrupt.
const fakeAgent = `#!/bin/sh
trap 'echo got-interrupt; exit 7' INT
echo "started $*"
grep SigIgn /proc/self/status
kill -TSTP $$
echo resumed
read line
echo "read $line"
exit 3
`

// slowAgent pauses, puts the terminal in raw mode when continued (as
// Claude does) and takes longer to exit on SIGHUP than asp used to wait.
const slowAgent = `#!/bin/sh
trap 'sleep 3; echo got-hup; exit 0' HUP
trap 'stty raw -echo' CONT
echo slow-started
kill -TSTP $$
while :; do sleep 1; done
`

func openPTY(t *testing.T) (master, slave *os.File) {
	t.Helper()
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	if err := unix.IoctlSetPointerInt(int(m.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(m.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	s, err := os.OpenFile("/dev/pts/"+strconv.Itoa(n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	return m, s
}

func TestHost(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(fakeAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(slowAgent), 0o755); err != nil {
		t.Fatal(err)
	}

	master, slave := openPTY(t)
	defer master.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperHost$", "-test.v")
	cmd.Env = append(os.Environ(), helperEnv+"=1", "PATH="+bin+":/usr/bin:/bin", "WORK="+dir)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	slave.Close()

	lines := make(chan string, 100)
	go func() {
		sc := bufio.NewScanner(master)
		for sc.Scan() {
			lines <- strings.TrimRight(sc.Text(), "\r")
		}
		close(lines)
	}()
	var out []string
	expect := func(want string) {
		t.Helper()
		timeout := time.After(10 * time.Second)
		for {
			select {
			case l, ok := <-lines:
				if !ok {
					t.Fatalf("pty closed waiting for %q; got:\n%s", want, strings.Join(out, "\n"))
				}
				out = append(out, l)
				if strings.Contains(l, want) {
					return
				}
			case <-timeout:
				t.Fatalf("timed out waiting for %q; got:\n%s", want, strings.Join(out, "\n"))
			}
		}
	}

	expect("started --resume sess-1")
	expect("STEP paused")                  // helper saw the job stop and has the terminal
	expect("resumed")                      // helper continued the same process
	_, _ = master.Write([]byte("hello\r")) // typed into the agent, not asp
	expect("read hello")
	expect("STEP exited 3")
	expect("started") // second job: a new session
	expect("resumed")
	_, _ = master.Write([]byte{3}) // ctrl+c while the agent runs
	expect("got-interrupt")
	expect("STEP interrupted 7")
	expect("slow-started")
	expect("got-hup") // quitting waited for the agent to finish
	expect("STEP closed")
	expect("HELPER OK")
	if err := cmd.Wait(); err != nil {
		t.Fatalf("helper failed: %v\n%s", err, strings.Join(out, "\n"))
	}
	for _, l := range out {
		if strings.HasPrefix(l, "SigIgn:") {
			mask, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(l, "SigIgn:")), 16, 64)
			if err != nil {
				t.Fatal(err)
			}
			for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTSTP, syscall.SIGTTOU, syscall.SIGTTIN} {
				if mask&(1<<(uint(sig)-1)) != 0 {
					t.Errorf("agent started with %v ignored (SigIgn %016x)", sig, mask)
				}
			}
		}
	}
}

// TestHelperHost is the asp side, run only inside TestHost's pty.
func TestHelperHost(t *testing.T) {
	if os.Getenv(helperEnv) == "" {
		t.Skip("helper for TestHost")
	}
	fail := func(f string, a ...any) { fmt.Printf("HELPER FAIL "+f+"\n", a...); os.Exit(1) }
	cmdFor := func(a source.Agent, id string) (string, []string, error) {
		if a == source.Codex {
			return "codex", nil, nil
		}
		if id == "" {
			return "claude", nil, nil
		}
		return "claude", []string{"--resume", id}, nil
	}
	h := NewHost(cmdFor)
	h.Out = nil // keep the transcript readable: no clear sequences
	work := os.Getenv("WORK")
	foreground := func() bool {
		pg, err := unix.IoctlGetInt(h.TTY, unix.TIOCGPGRP)
		return err == nil && pg == unix.Getpgrp()
	}

	j := h.NewJob(source.Claude, work, "sess-1", "")
	if err := h.Start(j); err != nil {
		fail("start: %v", err)
	}
	if j.State != Paused || len(h.Paused()) != 1 || !foreground() {
		fail("after pause: state %v, %d paused, foreground %v", j.State, len(h.Paused()), foreground())
	}
	fmt.Println("STEP paused")
	if err := h.Resume(j); err != nil {
		fail("resume: %v", err)
	}
	if j.State != Exited || j.ExitCode != 3 || len(h.Paused()) != 0 || !foreground() {
		fail("after exit: state %v code %d, %d paused, foreground %v", j.State, j.ExitCode, len(h.Paused()), foreground())
	}
	fmt.Println("STEP exited", j.ExitCode)

	k := h.NewJob(source.Claude, work, "", "named")
	if err := h.Start(k); err != nil {
		fail("start 2: %v", err)
	}
	if err := h.Resume(k); err != nil { // the test sends ctrl+c now
		fail("resume 2: %v", err)
	}
	if k.State != Exited || k.ExitCode != 7 || !foreground() {
		fail("after interrupt: state %v code %d foreground %v", k.State, k.ExitCode, foreground())
	}
	fmt.Println("STEP interrupted", k.ExitCode)

	slow := h.NewJob(source.Codex, work, "", "")
	if err := h.Start(slow); err != nil || slow.State != Paused {
		fail("slow start: %v %v", err, slow.State)
	}
	h.Close()
	if len(h.Paused()) != 0 || slow.State != Exited || !foreground() {
		fail("after close: %d paused, state %v, foreground %v", len(h.Paused()), slow.State, foreground())
	}
	if err := syscall.Kill(slow.pid, 0); err == nil {
		fail("slow agent still running after Close")
	}
	tio, err := unix.IoctlGetTermios(h.TTY, unix.TCGETS)
	if err != nil || tio.Lflag&unix.ICANON == 0 || tio.Lflag&unix.ECHO == 0 {
		fail("terminal left in raw mode after Close: %v", err)
	}
	fmt.Println("STEP closed")

	missing := h.NewJob(source.Claude, work+"/nope", "", "")
	if err := h.Start(missing); err == nil || !foreground() {
		fail("missing dir: err %v", err)
	}
	fmt.Println("HELPER OK")
}
