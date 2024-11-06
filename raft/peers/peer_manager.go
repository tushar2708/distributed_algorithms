package peers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"distributed_algorithms/grpc/pb/raftpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// PeerManager takes care of maintaining connections to peers
type PeerManager interface {
	GetNodeID() string
	GetPeerIds() []string
	GetClient(peerID string) (raftpb.RaftServiceClient, error)

	ConnectToPeer(peerID string) (raftpb.RaftServiceClient, error)
	DisconnectPeer(peerID string) error
	ConnectToAllPeers()
	DisconnectAllPeers()
}

type peerManager struct {
	mu  sync.RWMutex
	ctx context.Context

	nodeID         string        // self node-id
	reloadDuration time.Duration // how frequently do we want to reload peer details
	grpcPort       int           // gRPC port number

	// derived fields
	peerIds         []string                            //	list of peer IDs
	peerIPAddresses map[string]string                   //	contains peer NodeID as key, and it's IP address as value
	peerClients     map[string]raftpb.RaftServiceClient // contains peer NodeID as key, and it's grpcClient as value
}

var (
	// ErrCouldNotFetchNodes indicates that we haven't been able to fetch the nodes' details from the DB
	ErrCouldNotFetchNodes = errors.New("couldn't fetch nodes from DB")
)

// GetPeerIds gives a list of all the peer node IDs
func (ph *peerManager) GetPeerIds() []string {
	ph.mu.RLock()
	defer ph.mu.RUnlock()

	return ph.peerIds
}

// GetNodeID fetches current node's ID
func (ph *peerManager) GetNodeID() string {
	ph.mu.RLock()
	defer ph.mu.RUnlock()

	return ph.nodeID
}

// GetClient fetches a gRPC client (connects if missing)
func (ph *peerManager) GetClient(peerID string) (raftpb.RaftServiceClient, error) {
	ph.mu.Lock()
	defer ph.mu.Unlock()

	peerClient, cErr := ph.ConnectToPeer(peerID)
	if cErr != nil {
		return nil, cErr
	}

	return peerClient, nil
}

// ConnectToAllPeers connects to all peers
func (ph *peerManager) ConnectToAllPeers() {
	for peerID := range ph.peerIPAddresses {
		if peerID == ph.nodeID {
			continue
		}

		if _, pErr := ph.ConnectToPeer(peerID); pErr != nil {
			continue
		}
	}
}

const keepaliveInterval = 10 * time.Second

func (ph *peerManager) createClient(ctx context.Context, peerIPAddress string) (raftpb.RaftServiceClient, error) {
	var conn *grpc.ClientConn
	var gErr error
	peerAddress := fmt.Sprintf("%s:%d", peerIPAddress, ph.grpcPort)

	// Insecure is only to be used for unit testing.
	transportDialOpt := grpc.WithInsecure()
	conn, gErr = grpc.DialContext(
		ctx,
		peerAddress,
		transportDialOpt,
		grpc.WithKeepaliveParams(
			keepalive.ClientParameters{
				Time:                keepaliveInterval,
				Timeout:             keepaliveInterval,
				PermitWithoutStream: true,
			},
		),
	)
	if gErr != nil {
		return nil, gErr
	}

	return raftpb.NewRaftServiceClient(conn), gErr
}

// ConnectToPeer connects to a given peer
func (ph *peerManager) ConnectToPeer(peerID string) (raftpb.RaftServiceClient, error) { //nolint:ireturn
	ph.mu.Lock()
	defer ph.mu.Unlock()

	if ph.peerClients[peerID] == nil {
		peerIPAddress := ph.peerIPAddresses[peerID]

		quorumClient, err := ph.createClient(ph.ctx, peerIPAddress)

		if err != nil {
			log.Printf("fail to create client on %s for peerID: %s: [%v]\n",
				peerIPAddress, peerID, err)
			ph.peerClients[peerID] = nil

			return nil, fmt.Errorf(
				"fail to create client on %s for peerID: %s, err: %w",
				peerIPAddress, peerID, err)
		}

		ph.peerClients[peerID] = quorumClient
	}

	return ph.peerClients[peerID], nil
}

// DisconnectAllPeers closes all the client connections to peers for this server.
func (ph *peerManager) DisconnectAllPeers() {
	ph.mu.Lock()
	defer ph.mu.Unlock()

	for id := range ph.peerClients {
		if ph.peerClients[id] != nil {
			ph.peerClients[id] = nil
		}
	}
}

// DisconnectPeer disconnects this server from the peer identified by peerId.
// It also cleans up any state maintained for the peerID in PeerManager
func (ph *peerManager) DisconnectPeer(peerID string) error {
	ph.mu.Lock()
	defer ph.mu.Unlock()

	delete(ph.peerClients, peerID)
	delete(ph.peerIPAddresses, peerID)
	for i, id := range ph.peerIds {
		if id == peerID {
			ph.peerIds = append(ph.peerIds[:i], ph.peerIds[i+1:]...)
			break
		}
	}

	return nil
}
