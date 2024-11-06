package raft

import (
	"context"
	"sync"

	"distributed_algorithms/raft/state"
)

// BaseNodeState represent Base state, that every other in the leader election's state machine inherits from
type BaseNodeState struct {
	mu sync.Mutex
	/*
		context stateTransitionCtx to be initialized to a Background() context, when we enter a given state
		And it should be marked as Done when exiting any state.
		Any internal goroutines or loops should monitor this stateTransitionCtx to abort & clean-up resources on exit
	*/
	ctx            context.Context
	RaftGroup      RaftGroup
	State          state.NodeStateEnum
	ElectionData   *state.RaftStateData
	raftOperations RaftOperations
}

// StateName returns the corresponding NodeStateName for the state instance
func (b *BaseNodeState) StateName() state.NodeStateEnum {
	if b == nil {
		return state.Follower
	}
	return b.State
}

// GetRaftStateData returns underlying raft state data
func (b *BaseNodeState) GetRaftStateData() *state.RaftStateData {

	return b.ElectionData
}

// SetElectionData sets underlying election state data
func (b *BaseNodeState) SetRaftStateData(data *state.RaftStateData) {

	b.ElectionData = data
}
