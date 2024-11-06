package raft

import (
	"context"
	"errors"

	"distributed_algorithms/raft/config"
	"distributed_algorithms/raft/peers"
	"distributed_algorithms/raft/state"
)

var (
	// ErrNoPeerManager error represents missing PeerManager
	ErrNoPeerManager = errors.New("no valid WithPeerHandler() provided to Quorum group builder")
)

// RaftGroupOption is a function used to implement Options builder pattern for RaftGroup
type RaftGroupOption func(*raftGroup)

// NewRaftGroup builds a new instance of RaftGroup
func NewRaftGroup(
	ctx context.Context,
	nodeID string,
	quorumGroupName string,
	opts ...RaftGroupOption,
) (*raftGroup, error) {

	rg := &raftGroup{
		ctx:                 ctx,
		nodeID:              nodeID,
		GroupName:           quorumGroupName,
		peerManager:         nil,
		State:               nil,
		StateData:           nil,
		stateChangeNotifier: make(chan state.NodeStateEnum, 1),
	}

	for _, opt := range opts {
		// Call the options one-by-one
		opt(rg)
	}

	if rg.peerManager == nil {
		return nil, ErrNoPeerManager
	}

	// initialize the quorum group state as a follower, but do not enter yet
	rg.State = NewFollowerState(rg.ctx, rg, rg.State.GetRaftStateData(), rg.raftOps)

	return rg, nil
}

// WithPeerHandler adds an option to set PeerHandler
func WithPeerManager(pm peers.PeerManager) RaftGroupOption {
	return func(rg *raftGroup) {
		rg.peerManager = pm
	}
}

// WithRaftTimeOutConfigs adds an option to set QuorumConfig
func WithRaftTimeOutConfigs(vc *config.RaftElectionConfig) RaftGroupOption {
	return func(rg *raftGroup) {
		rg.raftElectionConfig = vc
	}
}

// WithInitialStateData adds an option to set initial consensus state data.
// Mostly meant for testing, as initializing it will skip fetching the latest state from DB
func WithInitialStateData(stateData *state.RaftStateData) RaftGroupOption {
	return func(vg *raftGroup) {
		vg.StateData = stateData
	}
}
