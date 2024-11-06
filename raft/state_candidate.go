package raft

import (
	"context"
	"distributed_algorithms/raft/state"
	"fmt"
	"log"
)

// CandidateState represent Candidate state in the leader election's state machine
type CandidateState struct {
	*BaseNodeState
	stateTransitionCtx context.Context
}

// NewCandidateState creates a new instance of CandidateState
func NewCandidateState(ctx context.Context, raftGroup RaftGroup, raftStateData *state.RaftStateData, raftOperations RaftOperations) state.NodeState {
	return &CandidateState{
		stateTransitionCtx: context.Background(),
		BaseNodeState: &BaseNodeState{
			ctx:            ctx,
			RaftGroup:      raftGroup,
			raftOperations: raftOperations,
			State:          state.Candidate,
			ElectionData:   raftStateData,
		},
	}
}

// Enter implements state.NodeState.
func (c *CandidateState) Enter(term uint64) {
	// become a candidate
	log.Printf("node:[%s] became a candidate for election within group:[%s]",
		c.RaftGroup.NodeID(), c.RaftGroup.Name())

	c.mu.Lock()
	c.stateTransitionCtx = context.Background()
	c.mu.Unlock()

	c.ElectionData.SetTerm(term)
	c.ElectionData.IncrementTerm()
	c.ElectionData.SetState(state.Candidate)
	c.ElectionData.SetCurrentLeader(state.UndefinedNode)
	c.ElectionData.ResetElectionTimer()

	newState, updatedTerm, vErr := c.raftOperations.ContestElection(term)

	if vErr == nil && newState == state.Leader {
		// became leader
		c.RaftGroup.Transition(NewLeaderState(c.ctx, c.RaftGroup, c.ElectionData, c.raftOperations), updatedTerm)
		c.ElectionData.SetCurrentLeader(c.RaftGroup.NodeID())
		return
	}

	// If not leader or dead, become a follower by default.
	c.ElectionData.SetCurrentLeader(state.UndefinedNode)
	c.RaftGroup.Transition(NewFollowerState(c.ctx, c.RaftGroup, c.ElectionData, c.raftOperations), updatedTerm)
}

// Exit implements state.NodeState.
func (c *CandidateState) Exit() {
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Printf("node:[%s] exiting state: [%s] ", c.RaftGroup.NodeID(), c.StateName())

	if c.stateTransitionCtx != nil {
		c.stateTransitionCtx.Done()
	}
	c.stateTransitionCtx = nil
}

// EvaluateTerm implements state.NodeState.
func (c *CandidateState) EvaluateTerm(offeredTerm uint64, offeredLeader string) (newTerm uint64, termAccepted bool, err error) {
	currentTerm := c.ElectionData.GetCurrentTerm()

	// if there's a leader somewhere with lower term than this candidate node, then we reject the offered term
	if currentTerm > offeredTerm {

		return currentTerm, false, fmt.Errorf("current term: %d, offered term: %d, err:%w",
			currentTerm, offeredTerm, ErrCurrentNodeHasHigherTerm)
	}

	// if there's a leader somewhere with higher or equal term as this candidate node, then we should become a follower
	c.RaftGroup.Transition(NewFollowerState(c.ctx, c.RaftGroup, c.ElectionData, c.raftOperations), offeredTerm)
	c.ElectionData.SetCurrentLeader(offeredLeader)

	return offeredTerm, true, fmt.Errorf("current term: %d, offered term: %d, err:%w",
		currentTerm, offeredTerm, ErrCurrentNodeHasEqualOrLowerTerm)
}
