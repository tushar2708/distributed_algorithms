package state

import (
	"errors"
	"sync"
	"time"
)

// UndefinedNode signifies the state of undefined node
const (
	UndefinedNode = "UNDEFINED_NODE"
)

var (
	ErrThisNodeIsNotTheLeader = errors.New("this node is not the leader")
)

// Log entry struct
type LogEntry struct {
	Term    int
	Command any
}

/*

type RaftNode struct {
	mu          sync.Mutex
	state       State
	currentTerm int
	votedFor    *int
	log         []LogEntry
	commitIndex int
	lastApplied int
	nextIndex   []int
	matchIndex  []int
	peers       []string
}

*/

// RaftStateData is used to maintain the local state of each node, as part of raft's state machine.
type RaftStateData struct {
	mu                 sync.RWMutex
	state              NodeStateEnum
	currentTerm        uint64 // current raft term going on according to this node
	votedFor           string // candidate id that this node voted for in current term
	electionResetEvent time.Time
	currentLeader      string

	Logs        []LogEntry
	commitIndex int
	nextIndex   []int
	matchIndex  []int
}

// NewRaftStateData creates a new instance of RaftStateData
func NewRaftStateData(state NodeStateEnum, term uint64, currentLeader string) *RaftStateData {
	return &RaftStateData{ //nolint:exhaustivestruct
		votedFor:           UndefinedNode,
		currentTerm:        term,
		state:              state,
		electionResetEvent: time.Now(),
		currentLeader:      currentLeader,
	}
}

// // GetAllFields gets all fields
// func (rsd *RaftStateData) GetAllFields() (votedFor string, currentTerm uint64, state NodeStateName) {
// 	rsd.mu.RLock()
// 	defer rsd.mu.RUnlock()

// 	return rsd.votedFor, rsd.currentTerm, rsd.state
// }

// GetVote fetches votedFor
func (rsd *RaftStateData) GetVote() string {
	rsd.mu.RLock()
	defer rsd.mu.RUnlock()

	return rsd.votedFor
}

// GetCurrentLeader fetches current leader NodeID
func (rsd *RaftStateData) GetCurrentLeader() string {
	rsd.mu.RLock()
	defer rsd.mu.RUnlock()

	return rsd.currentLeader
}

// GetCurrentTerm fetches currentTerm
func (rsd *RaftStateData) GetCurrentTerm() uint64 {
	rsd.mu.RLock()
	defer rsd.mu.RUnlock()

	return rsd.currentTerm
}

// GetState fetches state
func (rsd *RaftStateData) GetState() NodeStateEnum {
	rsd.mu.RLock()
	defer rsd.mu.RUnlock()

	return rsd.state
}

// GetElectionResetTime fetches electionResetEvent
func (rsd *RaftStateData) GetElectionResetTime() time.Time {
	rsd.mu.RLock()
	defer rsd.mu.RUnlock()

	return rsd.electionResetEvent
}

// ResetVote resets votedFor to -1
func (rsd *RaftStateData) ResetVote() {
	rsd.mu.Lock()
	defer rsd.mu.Unlock()

	rsd.votedFor = UndefinedNode
}

// ResetElectionTimer resets election timer to current time
func (rsd *RaftStateData) ResetElectionTimer() {
	rsd.mu.Lock()
	defer rsd.mu.Unlock()

	rsd.electionResetEvent = time.Now()
}

// IncrementTerm increments term value
func (rsd *RaftStateData) IncrementTerm() uint64 {
	rsd.mu.Lock()
	defer rsd.mu.Unlock()

	rsd.currentTerm++

	return rsd.currentTerm
}

// SetVote sets the candidateID that this node voted for
func (rsd *RaftStateData) SetVote(votedFor string) {
	rsd.mu.Lock()
	defer rsd.mu.Unlock()

	rsd.votedFor = votedFor
}

// SetCurrentLeader sets the current leader NodeID
func (rsd *RaftStateData) SetCurrentLeader(currentLeader string) {
	rsd.mu.Lock()
	defer rsd.mu.Unlock()

	rsd.currentLeader = currentLeader
}

// ResetCurrentLeader resets the current leader NodeID to UndefinedNode (to be used when leader information is somehow lost)
func (rsd *RaftStateData) ResetCurrentLeader() {
	rsd.mu.Lock()
	defer rsd.mu.Unlock()

	rsd.currentLeader = UndefinedNode
}

// SetTerm sets currentTerm
func (rsd *RaftStateData) SetTerm(term uint64) {
	rsd.mu.Lock()
	defer rsd.mu.Unlock()

	rsd.currentTerm = term
}

// SetState sets state
func (rsd *RaftStateData) SetState(state NodeStateEnum) {
	rsd.mu.Lock()
	defer rsd.mu.Unlock()

	rsd.state = state
}

// SetStateAndTerm atomically changes both state & currentTerm
func (rsd *RaftStateData) SetStateAndTerm(state NodeStateEnum, term uint64) {
	rsd.mu.Lock()
	defer rsd.mu.Unlock()

	rsd.state = state
	rsd.currentTerm = term
}
