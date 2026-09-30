//go:build !linux

package jobs

import (
	"errors"
	"time"

	"github.com/summant/asp/internal/source"
)

// Job control is implemented for Linux only so far; the terminal-reclaim
// step needs a per-thread signal mask. Elsewhere asp still lists and names
// sessions but cannot run them.

type State int

const (
	Running State = iota
	Paused
	Exited
)

type Job struct {
	N         int
	Agent     source.Agent
	Dir       string
	ResumeID  string
	Name      string
	Group     string
	Before    map[string]bool
	Started   time.Time
	SessionID string
	State     State
	ExitCode  int
}

type Command func(agent source.Agent, id string) (string, []string, error)

type Host struct{ next int }

var errUnsupported = errors.New("running agents from asp is only supported on Linux so far")

func NewHost(Command) *Host { return &Host{} }

func (h *Host) NewJob(agent source.Agent, dir, resumeID, name string) *Job {
	h.next++
	return &Job{N: h.next, Agent: agent, Dir: dir, ResumeID: resumeID, SessionID: resumeID, Name: name, Started: time.Now()}
}
func (h *Host) Start(*Job) error  { return errUnsupported }
func (h *Host) Resume(*Job) error { return errUnsupported }
func (h *Host) End(*Job) error    { return errUnsupported }
func (h *Host) Paused() []*Job    { return nil }
func (h *Host) Close()            {}
