package raft

import (
	"context"
	"fmt"
	"log"
	"time"

	"distributed_algorithms/raft/config"
	"distributed_algorithms/raft/state"
)

// LeaderState represent Leader state in the leader election's state machine
type LeaderState struct {
	*BaseNodeState
	stateTransitionCtx context.Context
}

// NewLeaderState creates a new instance of LeaderState
func NewLeaderState(ctx context.Context, raftGroup RaftGroup, raftStateData *state.RaftStateData,
	raftOperations RaftOperations) state.NodeState {
	return &LeaderState{
		stateTransitionCtx: nil,
		BaseNodeState: &BaseNodeState{
			ctx:            ctx,
			RaftGroup:      raftGroup,
			raftOperations: raftOperations,
			State:          state.Leader,
			ElectionData:   raftStateData,
		},
	}
}

// Enter implements state.NodeState.
func (l *LeaderState) Enter(term uint64) {
	l.mu.Lock()
	l.stateTransitionCtx = context.Background()
	l.mu.Unlock()

	l.ElectionData.SetState(state.Leader)
	l.ElectionData.SetTerm(term)
	l.ElectionData.SetCurrentLeader(l.RaftGroup.NodeID())

	go l.keepSendingHeartBeats()

	log.Printf("node:[%s] became leader with term=%d, starting to send HBs",
		l.RaftGroup.NodeID(), l.ElectionData.GetCurrentTerm())

	l.RaftGroup.NotifyStateChange(state.Leader)
}

// Exit implements state.NodeState.
func (l *LeaderState) Exit() {

	l.mu.Lock()
	defer l.mu.Unlock()

	log.Printf("node:[%s] exiting state: [%s] ", l.RaftGroup.NodeID(), l.StateName())

	if l.stateTransitionCtx != nil {
		l.stateTransitionCtx.Done()
	}
	l.stateTransitionCtx = nil
}

// EvaluateTerm implements state.NodeState.
func (l *LeaderState) EvaluateTerm(offeredTerm uint64, offeredLeader string) (newTerm uint64, termAccepted bool, err error) {
	currentTerm := l.ElectionData.GetCurrentTerm()

	switch {
	case currentTerm > offeredTerm:

		return currentTerm, false, fmt.Errorf("current term: %d, offered term: %d, err:%w",
			currentTerm, offeredTerm, ErrCurrentNodeHasHigherTerm)

	case currentTerm == offeredTerm:

		return currentTerm, true, fmt.Errorf("current term: %d, offered term: %d, err:%w",
			currentTerm, offeredTerm, ErrCurrentNodeHasSameTerm)

	default:

		l.ElectionData.SetTerm(offeredTerm)
		l.RaftGroup.Transition(NewFollowerState(l.ctx, l.RaftGroup, l.ElectionData, l.raftOperations), offeredTerm)
		l.ElectionData.SetCurrentLeader(offeredLeader)

		return offeredTerm, true, fmt.Errorf("current term: %d, offered term: %d, err: %w", currentTerm, offeredTerm, ErrCurrentNodeHasLowerTerm)
	}
}

// GetRaftStateData implements state.NodeState.
// Subtle: this method shadows the method (*BaseNodeState).GetRaftStateData of LeaderState.BaseNodeState.
func (l *LeaderState) GetRaftStateData() *state.RaftStateData {
	panic("unimplemented")
}

// SetRaftStateData implements state.NodeState.
// Subtle: this method shadows the method (*BaseNodeState).SetRaftStateData of LeaderState.BaseNodeState.
func (l *LeaderState) SetRaftStateData(data *state.RaftStateData) {
	panic("unimplemented")
}

// StateName implements state.NodeState.
// Subtle: this method shadows the method (*BaseNodeState).StateName of LeaderState.BaseNodeState.
func (l *LeaderState) StateName() state.NodeStateEnum {
	panic("unimplemented")
}

// keepSendingHeartBeats sends a round of heartbeats to all peers, collects their
// replies and adjusts CS's state.
func (l *LeaderState) keepSendingHeartBeats() {
	ticker := time.NewTicker(time.Duration(config.DefaultConfig.RaftCfg.ElectionTimeoutInMs) * time.Millisecond)
	defer ticker.Stop()

	// Send periodic heartbeats, as long as still leader.
	for {
		currentTerm := l.ElectionData.GetCurrentTerm()

		newState, updatedTerm, updatedLeader, _ := l.raftOperations.SendHeartbeatToPeers(currentTerm)

		if newState == state.Follower {
			log.Printf("term out of date in heartbeat reply")

			l.ElectionData.SetCurrentLeader(updatedLeader)

			l.RaftGroup.Transition(NewFollowerState(l.ctx, l.RaftGroup, l.ElectionData, l.raftOperations), updatedTerm)

			return
		}

		<-ticker.C // wait till it's time to send the next heartbeat

		if l.ElectionData.GetState() != state.Leader {
			return
		}
	}
}
