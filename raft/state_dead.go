package raft

import (
	"context"
	"distributed_algorithms/raft/state"
	"log"
)

// FollowerState represent Follower state in the leader election's state machine
type DeadState struct {
	*BaseNodeState
	stateTransitionCtx context.Context
}

// NewDeadState creates a new instance of DeadState
func NewDeadState(ctx context.Context, raftGroup RaftGroup, raftState *state.RaftStateData, raftOperations RaftOperations) state.NodeState {
	return &DeadState{
		stateTransitionCtx: nil,
		BaseNodeState: &BaseNodeState{
			ctx:            ctx,
			RaftGroup:      raftGroup,
			raftOperations: raftOperations,
			State:          state.Dead,
			ElectionData:   raftState,
		},
	}
}

// Enter implements state.NodeState.
func (d *DeadState) Enter(term uint64) {
	d.mu.Lock()
	d.stateTransitionCtx = context.Background()
	d.mu.Unlock()

	d.ElectionData.SetStateAndTerm(state.Dead, term)
	d.ElectionData.SetCurrentLeader(state.UndefinedNode)
	d.ElectionData.ResetElectionTimer()
	d.ElectionData.ResetVote()

	d.RaftGroup.NotifyStateChange(state.Dead)
}

// Exit implements state.NodeState.
func (d *DeadState) Exit() {
	d.mu.Lock()
	defer d.mu.Unlock()

	log.Printf("node:[%s] exiting state: [%s] ", d.RaftGroup.NodeID(), d.StateName())

	if d.stateTransitionCtx != nil {
		d.stateTransitionCtx.Done()
	}
	d.stateTransitionCtx = nil
}

// EvaluateTerm implements state.NodeState.
func (d *DeadState) EvaluateTerm(offeredTerm uint64, offeredLeader string) (newTerm uint64, termAccepted bool, err error) {
	return 0, false, ErrThisNodeIsMarkedDead
}
