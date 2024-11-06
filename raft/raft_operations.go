package raft

import (
	"context"
	"errors"
	"log"
	"time"

	"distributed_algorithms/grpc/pb/raftpb"
	"distributed_algorithms/raft/peers"
	"distributed_algorithms/raft/state"
)

// RaftOperations is an interface that represents Raft operations
type RaftOperations interface {
	ContestElection(currentTerm uint64) (newState state.NodeStateEnum, updatedTerm uint64, err error)
	SendHeartbeatToPeers(currentTerm uint64) (newState state.NodeStateEnum,
		updatedTerm uint64, updatedLeaderID string, err error)
	AppendEntries(currentTerm uint64, logs []*raftpb.LogEntry) (newState state.NodeStateEnum,
		updatedTerm uint64, updatedLeaderID string, err error)
}

type raftGrpcOperations struct {
	ctx           context.Context
	nodeID        string
	raftGroupName string
	raftStateData *state.RaftStateData
	peerManager   peers.PeerManager
}

// ErrUnableToInitializeLeaderNode unable to run "insert if not exist" for the leader node of the given quorum group
var ErrUnableToInitializeLeaderNode = errors.New("unable to initialize leader node on startup")

// NewHybridVotingStrategy creates a new strategy instance, and fills in quorumStateData struct from DB.
func NewRaftGrpcOperations(ctx context.Context,
	nodeID string,
	raftGroupName string,
	raftStateData *state.RaftStateData,
	peerManager peers.PeerManager,
) (RaftOperations, *state.RaftStateData, error) {

	if raftStateData == nil {

		raftStateData = state.NewRaftStateData(state.Follower, 0, state.UndefinedNode)
	}

	return &raftGrpcOperations{
		ctx:           ctx,
		nodeID:        nodeID,
		raftGroupName: raftGroupName,
		raftStateData: raftStateData,
		peerManager:   peerManager,
	}, raftStateData, nil
}

// ContestElection contest election for current term
func (rgo *raftGrpcOperations) ContestElection(currentTerm uint64) (newState state.NodeStateEnum,
	updatedTerm uint64, err error) {

	proposedLeaderID := rgo.nodeID
	updatedTerm = currentTerm
	electionFalied := false
	votesReceived := 0

	// loop over all the peers, and send them heartbeat messages
	for _, peerID := range rgo.peerManager.GetPeerIds() {
		// we don't need to call ourselves, so skip same node
		if peerID == rgo.nodeID {
			continue
		}

		peerClient, cErr := rgo.peerManager.GetClient(peerID)
		if cErr != nil {
			log.Printf("failed to connect to node:[%s] for AskForVote API (err: %v), skipping this vote",
				peerID, cErr)

			continue
		}

		args := &raftpb.AskForVoteRequest{
			Term:         currentTerm,
			CandidateId:  proposedLeaderID,
			LastLogIndex: 0,
			LastLogTerm:  0,
			GroupName:    rgo.raftGroupName,
		}

		log.Printf("sending AskForVote to node:[%s], term:[%d]", peerID, args.Term)

		ctx, cancel := context.WithTimeout(rgo.ctx, 200*time.Millisecond)
		defer cancel()

		voteReply, vErr := peerClient.AskForVote(ctx, args)

		if vErr != nil {
			log.Printf("failed to call node:[%s] for AskForVote, err:[%v]", peerID, vErr)
			cancel()
			continue
		}
		cancel()

		// The other node is on a newer/larger term. This node is becoming a follower,
		// because the other node needs to be made the leader
		if voteReply.Term > currentTerm {
			electionFalied = true
			updatedTerm = voteReply.Term
			break
		}

		if voteReply.VoteGranted {
			votesReceived++
		}
	}

	if electionFalied {
		return state.Follower, updatedTerm, nil
	}

	// Did we get at least half the votes?
	if votesReceived > len(rgo.peerManager.GetPeerIds())/2 {
		return state.Leader, currentTerm, nil
	}

	return state.Follower, updatedTerm, nil
}

// SendHeartbeatToPeers sends heartbeat calls to peer nodes
func (rgo *raftGrpcOperations) SendHeartbeatToPeers(currentTerm uint64) (newState state.NodeStateEnum,
	updatedTerm uint64, updatedLeaderID string, err error) {

	updatedLeaderID = rgo.nodeID // if nothing changes, this node will continue to be the leader

	// loop over all the peers, and send them heartbeat messages
	for _, peerID := range rgo.peerManager.GetPeerIds() {
		// we don't need to call ourselves, so skip same node
		if peerID == rgo.nodeID {
			continue
		}

		peerClient, cErr := rgo.peerManager.GetClient(peerID)
		if cErr != nil {
			log.Printf("failed to connect to node:[%s] for HeartBeat API (err: %v), skipping this vote",
				peerID, cErr)

			continue
		}

		args := &raftpb.HeartBeatRequest{
			Term:      currentTerm,
			LeaderId:  rgo.nodeID,
			GroupName: rgo.raftGroupName,
		}

		log.Printf("sending SendHeartbeatToPeers to node:[%s], term:[%d]", peerID, args.Term)

		ctx, cancel := context.WithTimeout(rgo.ctx, 200*time.Millisecond)
		heartBeatReply, hErr := peerClient.HeartBeat(ctx, args)
		cancel()
		if hErr != nil {
			log.Printf("failed to call node:[%s] for HeartBeat, err:[%v]", peerID, hErr)

			continue
		}

		/*
			The node we sent a heartbeat to, is on a newer/larger term.
			This node is becoming a follower because current node might have been down for some time.

			The catch here is that the other node showing a higher term might also have been disconnected for a while,
			and kept trying to run back-to-back election. But even in that case, our node wil become a follower
			This is acceptable in our situation as we don't want to manage commit logs.
		*/
		if heartBeatReply.Term > currentTerm && !heartBeatReply.Success {
			log.Printf("HeartBeat reply from node:[%s] has higher term:[%d] than our term[%d], success=%t. "+
				"We will now change state to follower",
				peerID, heartBeatReply.Term, currentTerm, heartBeatReply.Success)

			return state.Follower, heartBeatReply.Term, peerID, state.ErrThisNodeIsNotTheLeader
		}

		log.Printf("HeartBeat reply from node:[%s] has term:[%d], success=%t. I will remain the leader",
			peerID, heartBeatReply.Term, heartBeatReply.Success)
	}

	currentTerm = rgo.raftStateData.IncrementTerm()

	return state.Leader, currentTerm, updatedLeaderID, nil
}

// AppendEntries implements RaftOperations.
func (rgo *raftGrpcOperations) AppendEntries(
	currentTerm uint64,
	logs []*raftpb.LogEntry,
) (
	newState state.NodeStateEnum, updatedTerm uint64, updatedLeaderID string, err error,
) {
	updatedLeaderID = rgo.nodeID // if nothing changes, this node will continue to be the leader

	// loop over all the peers, and send them heartbeat messages
	for _, peerID := range rgo.peerManager.GetPeerIds() {
		// we don't need to call ourselves, so skip same node
		if peerID == rgo.nodeID {
			continue
		}

		peerClient, cErr := rgo.peerManager.GetClient(peerID)
		if cErr != nil {
			log.Printf("failed to connect to node:[%s] for HeartBeat API (err: %v), skipping this vote",
				peerID, cErr)

			continue
		}

		args := &raftpb.AppendEntriesRequest{
			Term:         currentTerm,
			LeaderId:     rgo.nodeID,
			PrevLogIndex: 0,
			PrevLogTerm:  0,
			Entries:      logs,
			LeaderCommit: 0,
			GroupName:    rgo.raftGroupName,
		}

		log.Printf("sending Log Entries to node:[%s], term:[%d]", peerID, args.Term)

		ctx, cancel := context.WithTimeout(rgo.ctx, 200*time.Millisecond)
		appendEntriesReply, hErr := peerClient.AppendEntries(ctx, args)
		cancel()
		if hErr != nil {
			log.Printf("failed to call node:[%s] for HeartBeat, err:[%v]", peerID, hErr)
			continue
		}

		/*
			The node we sent a heartbeat to, is on a newer/larger term.
			This node is becoming a follower because current node might have been down for some time.

			The catch here is that the other node showing a higher term might also have been disconnected for a while,
			and kept trying to run back-to-back election. But even in that case, our node wil become a follower
			This is acceptable in our situation as we don't want to manage commit logs.
		*/
		if appendEntriesReply.Term > currentTerm && !appendEntriesReply.Success {
			log.Printf("appendEntries reply from node:[%s] has higher term:[%d] than our term[%d], success=%t. "+
				"We will now change state to follower",
				peerID, appendEntriesReply.Term, currentTerm, appendEntriesReply.Success)

			return state.Follower, appendEntriesReply.Term, peerID, ErrThisNodeIsNotTheLeader
		}

		log.Printf("appendEntries reply from node:[%s] has term:[%d], success=%t. I will remain the leader",
			peerID, appendEntriesReply.Term, appendEntriesReply.Success)
	}

	currentTerm = rgo.raftStateData.IncrementTerm()

	return state.Leader, currentTerm, updatedLeaderID, nil
}
