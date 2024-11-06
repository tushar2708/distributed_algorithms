package raft

import (
	"context"
	"log"
	"sync"

	"distributed_algorithms/grpc/pb/raftpb"
	"distributed_algorithms/raft/config"
	"distributed_algorithms/raft/peers"
	"distributed_algorithms/raft/state"
)

// GroupWrapper is an interface used to represent a part of QuorumGroup
// it's used mostly for testing
type RaftGroup interface {
	NodeID() string
	Name() string
	GetState() state.NodeState
	NotifyStateChange(state state.NodeStateEnum)
	Transition(newState state.NodeState, term uint64)
	StartElection(waitForReady <-chan any)
	DecideVote(requestedTerm uint64, candidateID string) (voteGiven bool, currentTerm uint64, err error)
}

type raftGroup struct {
	ctx                context.Context
	mu                 sync.Mutex
	nodeID             string
	GroupName          string
	StateData          *state.RaftStateData
	State              state.NodeState
	raftElectionConfig *config.RaftElectionConfig
	raftOps            RaftOperations

	peerManager         peers.PeerManager
	stateChangeNotifier chan state.NodeStateEnum

	commitIndex int
	lastApplied int
	nextIndex   []int
	matchIndex  []int
}

// StartElection starts the election state-machine for a consensus-group in a blocking call
func (rg *raftGroup) StartElection(waitForReady <-chan interface{}) {
	// The CS is quiescent until waitForReady is signaled; then, it starts a countdown for leader election.
	<-waitForReady
	rg.StateData.ResetElectionTimer()

	// enter the state that has been set earlier (should be Follower at startup)
	rg.mu.Lock()
	newState := rg.State
	rg.mu.Unlock()
	newState.Enter(rg.StateData.GetCurrentTerm())
}

// Transition exits the current state, and enters the new state
func (rg *raftGroup) Transition(newState state.NodeState, term uint64) {

	rg.mu.Lock()
	groupState := rg.State
	rg.mu.Unlock()

	currentState := state.Uninitialized

	if groupState != nil {
		currentState = rg.State.StateName()
		rg.State.Exit()
	}

	log.Printf("exited from '%s' state, going to enter '%s'\n", currentState, newState.StateName())

	rg.mu.Lock()
	rg.State = newState
	rg.mu.Unlock()

	rg.State.Enter(term)
}

func (rg *raftGroup) DecideVote(requestedTerm uint64, candidateID string) (voteGiven bool, currentTerm uint64, err error) {
	// If the request term is outdated, reject the request
	rg.mu.Lock()
	stateData := rg.GetState().GetRaftStateData()
	rg.mu.Unlock()

	if requestedTerm <= stateData.GetCurrentTerm() {
		return false, stateData.GetCurrentTerm(), ErrCurrentNodeHasHigherTerm
	}

	// Update term if the request's term is higher
	rg.mu.Lock()
	stateData.SetTerm(requestedTerm) // Update current term to the requestedTerm
	stateData.SetVote(candidateID)   // Assign vote to this candidate
	rg.mu.Unlock()
	rg.Transition(NewFollowerState(rg.ctx, rg, stateData, rg.raftOps), stateData.GetCurrentTerm()) // Transition to follower for higher term

	return true, stateData.GetCurrentTerm(), nil

	/*
			// TODO: Also consider log records for the vote
			// Check if this node can vote for the candidate
			if rg.votedFor == "" || rg.votedFor == request.CandidateId {
		        lastLogIndex := len(rg.logs) - 1
		        lastLogTerm := rg.logs[lastLogIndex].Term

		        // Vote if candidate’s log is up-to-date with this node's log
		        if request.LastLogTerm > lastLogTerm || (request.LastLogTerm == lastLogTerm && request.LastLogIndex >= lastLogIndex) {
		            rg.votedFor = request.CandidateId
		            response.VoteGranted = true
		        }
		    }
	*/
}

func (rg *raftGroup) AppendEntries(
	requestedTerm uint64, leaderID string, logEntries []*raftpb.LogEntry,
) (
	voteGiven bool, currentTerm uint64, err error,
) {
	// If the request term is outdated, reject the request
	rg.mu.Lock()
	stateData := rg.GetState().GetRaftStateData()
	rg.mu.Unlock()

	if requestedTerm <= stateData.GetCurrentTerm() {
		return false, stateData.GetCurrentTerm(), ErrCurrentNodeHasEqualOrHigherTerm
	}

	// Update term if the request's term is higher
	rg.mu.Lock()
	stateData.SetTerm(requestedTerm)     // Update current term to the requestedTerm
	stateData.SetCurrentLeader(leaderID) // accept this node as leader
	rg.mu.Unlock()

	if rg.State.StateName() != state.Follower {
		rg.Transition(NewFollowerState(rg.ctx, rg, stateData, rg.raftOps), stateData.GetCurrentTerm()) // Transition to follower for higher term
	}

	/*

		// TODO: Verify log consistency using PrevLogIndex and PrevLogTerm, and commit the entries locally.

		if request.PrevLogIndex >= 0 {
			if len(rg.logs) <= request.PrevLogIndex || rg.logs[request.PrevLogIndex].Term != request.PrevLogTerm {
				// If log does not match, reject the request
				return response, nil
			}
		}

		// Append new entries if there's a mismatch or if they’re missing
		for i, entry := range request.Entries {
			if len(rg.logs) <= request.PrevLogIndex+1+i || rg.logs[request.PrevLogIndex+1+i].Term != entry.Term {
				// Truncate conflicting entries and append new ones
				rg.logs = rg.logs[:request.PrevLogIndex+1+i]
				rg.logs = append(rg.logs, request.Entries[i:]...)
				break
			}
		}

		// Update commit index if leader’s commit index is higher
		if request.LeaderCommit > rg.commitIndex {
			rg.commitIndex = min(request.LeaderCommit, len(rg.logs)-1)
			rg.applyCommittedEntries()
		}
	*/

	return true, stateData.GetCurrentTerm(), nil
}

// NotifyStateChange updates the underlying channel with new state, after removing any previous message
func (rg *raftGroup) NotifyStateChange(state state.NodeStateEnum) {
	rg.mu.Lock()
	defer rg.mu.Unlock()
	// channel of size 1, clean it up if there's any unread value
	if len(rg.stateChangeNotifier) > 0 {
		<-rg.stateChangeNotifier
	}
	// add new value
	rg.stateChangeNotifier <- state
}

// GetStateNotifierChan returns a channel, that can be monitored to get node's current state
func (rg *raftGroup) GetStateNotifierChan() <-chan state.NodeStateEnum {
	return rg.stateChangeNotifier
}

// GetNodeID gives NodeID of the quorum group
func (rg *raftGroup) NodeID() string {
	return rg.nodeID
}

// GetNodeID gives NodeID of the quorum group
func (rg *raftGroup) GetState() state.NodeState {
	return rg.State
}

// GetGroupName gives GroupName of the quorum group
func (rg *raftGroup) Name() string {
	return rg.GroupName
}

// GetState gives State of the quorum group
func (rg *raftGroup) StateName() state.NodeState {
	return rg.State
}
