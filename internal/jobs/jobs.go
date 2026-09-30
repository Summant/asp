//go:build linux

// Package jobs runs agents under asp with shell-style job control.
//
// Each agent is a child process in its own process group, given the
// terminal while it runs. When the user suspends it (ctrl+z, which both
// Claude Code and Codex support) the child stops, asp takes the terminal
// back and shows its list; choosing the session again continues the same
// process. Nothing is emulated: the agent draws on the real terminal.
package jobs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/summant/asp/internal/source"
)

type State int

const (
	Running State = iota
	Paused
	Exited
)

// Job is one agent process started by asp.
type Job struct {
	N        int // sequence number, for sessions whose id is not yet known
	Agent    source.Agent
	Dir      string
	ResumeID string          // session to resume; empty starts a new one
	Name     string          // name to give a new session once it exists
	Group    string          // group to put a new session in once it exists
	Before   map[string]bool // the agent's session ids before a new start
	Started  time.Time

	// SessionID is the session this job runs: ResumeID, or the new id once
	// it has been found on disk.
	SessionID string

	State    State
	ExitCode int
	pid      int
}

// Command gives the binary and arguments for an agent: resume id, or start
// a new session when id is empty.
type Command func(agent source.Agent, id string) (string, []string, error)

// Host owns the terminal and the jobs running on it.
type Host struct {
	TTY     int       // controlling terminal, normally stdin
	Out     io.Writer // the terminal, for clearing between sessions
	Command Command

	saved *unix.Termios // terminal settings before asp started

	mu       sync.Mutex
	jobs     []*Job
	onScreen *Job // the job whose output is on the terminal's main screen
	next     int
}

func NewHost(cmd Command) *Host {
	h := &Host{TTY: int(os.Stdin.Fd()), Out: os.Stdout, Command: cmd}
	if t, err := unix.IoctlGetTermios(h.TTY, unix.TCGETS); err == nil {
		h.saved = t
	}
	return h
}

// NewJob prepares a job; Start runs it.
func (h *Host) NewJob(agent source.Agent, dir, resumeID, name string) *Job {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.next++
	return &Job{N: h.next, Agent: agent, Dir: dir, ResumeID: resumeID, SessionID: resumeID, Name: name, Started: time.Now()}
}

// Start runs j in the foreground and returns when it pauses or exits.
func (h *Host) Start(j *Job) error {
	bin, args, err := h.Command(j.Agent, j.ResumeID)
	if err != nil {
		return err
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return fmt.Errorf("%s not found in PATH", bin)
	}
	if fi, err := os.Stat(j.Dir); err != nil || !fi.IsDir() {
		return fmt.Errorf("not a directory: %s", j.Dir)
	}

	cmd := exec.Command(path, args...)
	cmd.Dir = j.Dir
	cmd.Env = append(os.Environ(), "PWD="+j.Dir)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Own process group, placed in the foreground: ctrl+c and ctrl+z from
	// the terminal reach the agent and never asp.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Foreground: true, Ctty: h.TTY}

	h.clear()
	if err := cmd.Start(); err != nil {
		h.reclaim()
		return err
	}
	j.pid = cmd.Process.Pid
	_ = cmd.Process.Release() // reaped by wait4 below, not by os/exec

	h.mu.Lock()
	h.jobs = append(h.jobs, j)
	h.onScreen = j
	h.mu.Unlock()
	return h.wait(j)
}

// Resume continues a paused job in the foreground and returns when it
// pauses or exits again.
func (h *Host) Resume(j *Job) error {
	if j.State != Paused {
		return errors.New("session is not paused")
	}
	h.mu.Lock()
	switching := h.onScreen != j
	h.onScreen = j
	h.mu.Unlock()
	if switching {
		// Another session's output is on screen; start clean and ask the
		// agent to redraw once it continues.
		h.clear()
	}
	if err := unix.IoctlSetPointerInt(h.TTY, unix.TIOCSPGRP, j.pid); err != nil {
		return fmt.Errorf("giving the terminal to %s: %w", j.Agent, err)
	}
	_ = syscall.Kill(-j.pid, syscall.SIGCONT)
	if switching {
		_ = syscall.Kill(-j.pid, syscall.SIGWINCH)
	}
	return h.wait(j)
}

// wait blocks until the job stops or exits, then takes the terminal back.
func (h *Host) wait(j *Job) error {
	j.State = Running
	var ws syscall.WaitStatus
	for {
		_, err := syscall.Wait4(j.pid, &ws, syscall.WUNTRACED, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			h.reclaim()
			j.State = Exited
			h.drop(j)
			return err
		}
		break
	}
	h.reclaim()
	if ws.Stopped() {
		j.State = Paused
		return nil
	}
	j.State = Exited
	j.ExitCode = ws.ExitStatus()
	if ws.Signaled() {
		j.ExitCode = 128 + int(ws.Signal())
	}
	h.drop(j)
	return nil
}

// reclaim makes asp's process group the terminal's foreground group again.
// asp is in the background at this point, so tcsetpgrp raises SIGTTOU
// unless it is blocked or ignored. It is blocked on this thread only:
// signal.Ignore would persist (signal.Reset does not undo it) and every
// later agent would inherit the ignored disposition.
func (h *Host) reclaim() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var block, old unix.Sigset_t
	sig := uint(syscall.SIGTTOU) - 1
	block.Val[sig/64] |= 1 << (sig % 64)
	_ = unix.PthreadSigmask(unix.SIG_BLOCK, &block, &old)
	_ = unix.IoctlSetPointerInt(h.TTY, unix.TIOCSPGRP, unix.Getpgrp())
	_ = unix.PthreadSigmask(unix.SIG_SETMASK, &old, nil)
}

func (h *Host) clear() {
	if h.Out != nil {
		// Reset attributes, home, clear screen and scrollback.
		_, _ = io.WriteString(h.Out, "\x1b[0m\x1b[H\x1b[2J\x1b[3J")
	}
}

func (h *Host) drop(j *Job) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, x := range h.jobs {
		if x == j {
			h.jobs = append(h.jobs[:i], h.jobs[i+1:]...)
			break
		}
	}
}

// Paused lists the jobs waiting to be continued.
func (h *Host) Paused() []*Job {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []*Job
	for _, j := range h.jobs {
		if j.State == Paused {
			out = append(out, j)
		}
	}
	return out
}

// Close ends every paused job the way a shell does when it exits: each in
// turn is given the terminal, sent SIGHUP (as a closing terminal would) and
// continued, then waited for, so its goodbye output lands before asp exits
// and its terminal cleanup runs while it is in the foreground. Signalled
// in the background instead, an agent re-stops as soon as it touches the
// terminal and outlives asp. Anything still alive after endTimeout is
// killed. Finally the terminal settings from before asp started are
// restored, in case an agent could not.
func (h *Host) Close() {
	paused := h.Paused()
	for _, j := range paused {
		_ = h.End(j)
	}
	if len(paused) > 0 && h.Out != nil {
		// Undo modes a killed agent may have left on: cursor hidden,
		// bracketed paste, focus and mouse reporting, kitty keyboard flags.
		_, _ = io.WriteString(h.Out, "\x1b[0m\x1b[?25h\x1b[?2004l\x1b[?1004l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[=0;1u")
	}
	if h.saved != nil {
		_ = unix.IoctlSetTermios(h.TTY, unix.TCSETS, h.saved)
	}
}

const endTimeout = 10 * time.Second

// End finishes one paused job as Close does — in the foreground, so the
// agent can clean up and write its goodbye — and takes the terminal back.
// The caller must have released the terminal (asp's UI is not drawing).
func (h *Host) End(j *Job) error {
	if j.State != Paused {
		return errors.New("session is not paused")
	}
	_ = unix.IoctlSetPointerInt(h.TTY, unix.TIOCSPGRP, j.pid)
	_ = syscall.Kill(-j.pid, syscall.SIGHUP) // pending, delivered on continue
	_ = syscall.Kill(-j.pid, syscall.SIGCONT)
	deadline := time.Now().Add(endTimeout)
	for {
		var ws syscall.WaitStatus
		pid, err := syscall.Wait4(j.pid, &ws, syscall.WNOHANG|syscall.WUNTRACED, nil)
		if err != nil && !errors.Is(err, syscall.EINTR) {
			break // already reaped
		}
		if pid == j.pid && (ws.Exited() || ws.Signaled()) {
			break
		}
		if pid == j.pid && ws.Stopped() { // paused itself again: insist
			_ = syscall.Kill(-j.pid, syscall.SIGCONT)
		}
		if time.Now().After(deadline) {
			_ = syscall.Kill(-j.pid, syscall.SIGKILL)
			_, _ = syscall.Wait4(j.pid, &ws, 0, nil)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	j.State = Exited
	h.drop(j)
	h.reclaim()
	return nil
}
