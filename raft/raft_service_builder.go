package raft

import (
	"context"
	"time"

	"distributed_algorithms/raft/config"
	"distributed_algorithms/raft/peers"
)

// RaftServiceOption is a function used to implement Options builder pattern for building QuorumService
type RaftServiceOption func(handler *RaftService)

// NewQuorumService builds a new instance of peerHandler
func NewRaftService(ctx context.Context, nodeID string, opts ...RaftServiceOption) (*RaftService, error) {

	service := &RaftService{
		nodeID:       nodeID,
		raftGroupMap: make(map[string]RaftGroup),
		quit:         make(chan any),
		config:       config.DefaultConfig,
		peerManager:  nil,
		grpcServer:   nil,
		listener:     nil,
	}

	for _, opt := range opts {
		// Call the options one-by-one
		opt(service)
	}

	if service.peerManager == nil {

		var pErr error
		service.peerManager, pErr =
			peers.NewPeerManager(
				ctx, service.nodeID,
				peers.WithReloadDuration(1*time.Minute),
				peers.WithStaticPeerConfig(
					service.config.PeerNodesMap,
				),
			)
		if pErr != nil {
			return nil, pErr
		}
	}

	return service, nil
}

// WithGrpcPort sets the port on which consensus server will listen for election messages
func WithGrpcPort(port int) RaftServiceOption {
	return func(rs *RaftService) {
		rs.config.ElectionPort = port
	}
}

// WithCustomPeerHandler sets peer handler for the consensus server
func WithCustomPeerManager(manager peers.PeerManager) RaftServiceOption {
	return func(rs *RaftService) {
		rs.peerManager = manager
	}
}

// WithRaftConfig adds an option to set raft config params
func WithRaftConfig(vc *config.RaftElectionConfig) RaftServiceOption {
	return func(rs *RaftService) {
		rs.config.RaftCfg = vc
	}
}

// WithPeerNodes adds an option to set QuorumConfig
func WithPeerNodes(pn map[string]string) RaftServiceOption {
	return func(rs *RaftService) {
		rs.config.PeerNodesMap = pn
	}
}
