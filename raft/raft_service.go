package raft

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"distributed_algorithms/grpc/pb/raftpb"
	"distributed_algorithms/raft/config"
	"distributed_algorithms/raft/peers"
	"distributed_algorithms/raft/state"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type RaftService struct {
	mu  sync.Mutex
	wg  sync.WaitGroup
	ctx context.Context

	// nodeID is the server NodeID of this CS.
	nodeID string

	config       config.Config
	raftGroupMap map[string]RaftGroup
	// There should only be one instance of peer-handler across the application,
	// to avoid creating too many gRPC connections
	peerManager peers.PeerManager

	// signaling channel
	quit chan interface{}
	// embedded gRPC server
	grpcServer *grpc.Server
	listener   net.Listener
	// insecureGrpcForTest bool

	*raftpb.UnimplementedRaftServiceServer
}

const (
	// 10 seconds is the minimum keepalive interval permitted by gRPC.
	permissibleKeepaliveInterval = 1 * time.Second
)

// startElection starts the consensus election in background (in a goroutine) and returns
func (rs *RaftService) startElection(waitForReady <-chan interface{}) {

	for _, rg := range rs.raftGroupMap {

		// we need to pin the range variable, to avoid race condition
		// refer: https://github.com/golang/go/wiki/CommonMistakes
		raftGroup := rg

		go func() {
			raftGroup.StartElection(waitForReady)
		}()
	}
}

// Serve starts serving on the given consensus port, and starts the gRPC service
func (rs *RaftService) serve(
	servicePort int,
	readyForElection <-chan interface{},
) {
	serverOpts := []grpc.ServerOption{
		grpc.KeepaliveEnforcementPolicy(
			keepalive.EnforcementPolicy{
				MinTime:             permissibleKeepaliveInterval,
				PermitWithoutStream: true,
			},
		),
	}

	var err error

	rs.grpcServer = grpc.NewServer(serverOpts...)
	rs.listener, err = net.Listen(
		"tcp",
		fmt.Sprintf("%s:%d", "0.0.0.0", servicePort))
	if err != nil {
		panic(fmt.Errorf("failed to listen on port %d, err:%w", servicePort, err))
	}

	raftpb.RegisterRaftServiceServer(rs.grpcServer, rs)

	rs.mu.Lock()
	log.Printf("[%v] listening at %s\n", rs.nodeID, rs.listener.Addr())
	rs.mu.Unlock()

	rs.wg.Add(1)

	go func() {
		defer rs.wg.Done()

		err = rs.grpcServer.Serve(rs.listener)
		if err != nil {
			panic("Failed to Serve gRPC server. Err: " + err.Error())
		}
	}()

	go func() {
		rs.startElection(readyForElection)
	}()

}

// Start sets up and starts a consensus server, also connects with peer nodes
func (rs *RaftService) Start() {
	ready := make(chan interface{})

	// start running gRPC server
	rs.serve(rs.config.ElectionPort, ready)

	time.Sleep(2 * time.Second) // let grpc servers start, before trying to connect clients in the next line

	rs.ConnectToPeers()

	close(ready) // close this channel to let the election start
}

// RegisterRaftGroupWithName registers a new raft group with the service
func (rs *RaftService) RegisterRaftGroupWithName(groupName string) error {

	qg, vErr := NewRaftGroup(rs.ctx, rs.peerManager.GetNodeID(), groupName,
		WithPeerManager(rs.peerManager),
		WithRaftTimeOutConfigs(rs.config.RaftCfg),
	)
	if vErr != nil {
		return vErr
	}

	rs.registerRaftGroup(qg) // register single raft group

	return nil
}

// registerRaftGroup registers a new raft group with the service
func (rs *RaftService) registerRaftGroup(rg *raftGroup) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.raftGroupMap[rg.GroupName] = rg
}

func (rs *RaftService) getRaftGroup(groupName string) (RaftGroup, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	group, ok := rs.raftGroupMap[groupName]
	if !ok {
		return nil, fmt.Errorf("no consensus group registered with name %s", groupName)
	}

	return group, nil
}

// ConnectToPeers connects to the peer nodes
func (rs *RaftService) ConnectToPeers() {
	rs.peerManager.ConnectToAllPeers()
}

// DisconnectAll closes all the client connections to peers for this server.
func (rs *RaftService) DisconnectAll() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.peerManager.DisconnectAllPeers()
}

// Shutdown closes the server and waits for it to shut down properly.
func (rs *RaftService) Shutdown() {
	rs.grpcServer.Stop()
	rs.DisconnectAll()

	close(rs.quit)

	if cErr := rs.listener.Close(); cErr != nil {
		log.Printf("unable to close grpc listener, err:%v", cErr)
	}

	rs.wg.Wait()
}

// AskForVote RPC. Other nodes will call this RPC, to ask for this node's vote.
func (rs *RaftService) AskForVote(ctx context.Context, request *raftpb.AskForVoteRequest) (*raftpb.AskForVoteResponse, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	// Find the correct RaftGroup based on GroupId in the request
	rg, err := rs.getRaftGroup(request.GroupName)
	if err != nil {
		return nil, fmt.Errorf("RaftGroup with Name %s not found", request.GroupName)
	}

	response := &raftpb.AskForVoteResponse{
		GroupName:   request.GroupName,
		Term:        rg.GetState().GetRaftStateData().GetCurrentTerm(),
		VoteGranted: false,
	}

	voteGiven, currentTerm, err := rg.DecideVote(request.Term, request.CandidateId)

	response.Term = currentTerm
	response.VoteGranted = voteGiven

	return response, err
}

// AppendEntries implements raftpb.RaftServiceServer.
func (rs *RaftService) AppendEntries(ctx context.Context, request *raftpb.AppendEntriesRequest) (*raftpb.AppendEntriesResponse, error) {

	log.Printf("Received HeartBeat from node:[%s] with term:[%d]", request.LeaderId, request.Term)

	response := &raftpb.AppendEntriesResponse{Term: 0, Success: false}

	raftGrp, vErr := rs.getRaftGroup(request.GroupName)
	if vErr != nil {
		return response, vErr
	}

	rg := raftGrp.(*raftGroup)

	voteGiven, currentTerm, err := rg.AppendEntries(request.Term, request.LeaderId, request.Entries)
	response.Success = voteGiven
	response.Term = currentTerm

	if err != nil {
		return response, err
	}

	// Reset election timeout upon receiving heartbeat or log entry

	return response, nil
}

// HeartBeat RPC. Other nodes will call this RPC, to update this node of their status.
func (rs *RaftService) HeartBeat(ctx context.Context, request *raftpb.HeartBeatRequest) (
	*raftpb.HeartBeatResponse, error) {

	log.Printf("Received HeartBeat from node:[%s] with term:[%d]", request.LeaderId, request.Term)

	var reply raftpb.HeartBeatResponse

	raftGroup, vErr := rs.getRaftGroup(request.GroupName)
	if vErr != nil {
		reply.Term = 0
		reply.Success = false
		reply.GroupName = request.GroupName

		return &reply, vErr
	}

	reply.GroupName = raftGroup.Name()

	newTerm, termAccepted, evalErr := raftGroup.GetState().EvaluateTerm(request.GetTerm(), request.GetLeaderId())

	reply.Success = termAccepted
	reply.Term = newTerm

	log.Printf("Returning HeartBeat reply to node:[%s]: (success: %t, %d), reason:[%v]",
		request.LeaderId, reply.Success, reply.Term, evalErr)

	return &reply, nil
}

// State reports the state of a given groupName in this rs.
func (rs *RaftService) State(groupName string) (*state.GroupState, error) {

	raftGroup, vErr := rs.getRaftGroup(groupName)
	if vErr != nil {
		return nil, vErr
	}

	return &state.GroupState{
		IsLeader:      raftGroup.GetState().GetRaftStateData().GetState() == state.Leader,
		CurrentLeader: raftGroup.GetState().GetRaftStateData().GetCurrentLeader(),
		CurrentTerm:   raftGroup.GetState().GetRaftStateData().GetCurrentTerm(),
	}, nil

}

// GetStateNotifierChan returns a channel, that can be monitored to get node's current state
func (rs *RaftService) GetStateNotifierChan(groupName string) (<-chan state.NodeStateEnum, error) {

	group, vErr := rs.getRaftGroup(groupName)
	if vErr != nil {
		return nil, vErr
	}

	raftGroup := group.(*raftGroup)

	return raftGroup.GetStateNotifierChan(), nil
}

// StopGroup stops this a particular quorum group, cleaning up its state. This method returns quickly, but
// it may take a bit of time (up to ~election timeout) for all goroutines to see this change and stop
func (rs *RaftService) StopGroup(groupName string) error {

	raftGroup, vErr := rs.getRaftGroup(groupName)
	if vErr != nil {
		return vErr
	}

	raftGroup.Transition(NewDeadState(rs.ctx, raftGroup,
		raftGroup.GetState().GetRaftStateData(), nil), 0)
	log.Printf("node:[%s] made Dead by StopGroup() call", rs.nodeID)
	return nil
}
