package peers

import (
	"context"
	"distributed_algorithms/grpc/pb/raftpb"
	"errors"
	"time"
)

// PeerManagerOption is a function used to implement Options builder pattern for building peerManager
type PeerManagerOption func(handler *peerManager)

var (
	// ErrNoPeerConfigAvailable indicates that we haven't found peer nodes' details from the DB,
	// and static commonconfig is missing as well
	ErrNoPeerConfigAvailable = errors.New("neither static nor DB based peer commonconfig could be initialized")
)

// NewpeerManager builds a new instance of peerManager
func NewPeerManager(ctx context.Context, nodeID string,
	opts ...PeerManagerOption) (PeerManager, error) {

	const (
		defaultConfigReloadDuration = 30 * time.Second
	)

	pm := &peerManager{
		ctx:             ctx,
		nodeID:          nodeID,
		peerIds:         make([]string, 0),
		reloadDuration:  defaultConfigReloadDuration,
		peerIPAddresses: make(map[string]string),
		peerClients:     make(map[string]raftpb.RaftServiceClient),
	}

	for _, opt := range opts {
		// Call the options one-by-one
		opt(pm)
	}

	// if we have static peer configs available, we are done here
	if len(pm.peerIds) > 0 || len(pm.peerIPAddresses) > 0 {
		return pm, nil
	}

	pm.ConnectToAllPeers()

	return pm, nil
}

// WithStaticPeerConfig sets a fixed map of peer IDs & IP addresses
func WithStaticPeerConfig(peerAddresses map[string]string) PeerManagerOption {
	return func(pm *peerManager) {

		for k, addr := range peerAddresses {
			pm.peerIds = append(pm.peerIds, k)
			pm.peerIPAddresses[k] = addr
		}
	}
}

// WithReloadDuration sets the duration for reloading peer info from the DB (default: 30 sec)
func WithReloadDuration(reloadDuration time.Duration) PeerManagerOption {
	return func(pm *peerManager) {
		pm.reloadDuration = reloadDuration
	}
}
