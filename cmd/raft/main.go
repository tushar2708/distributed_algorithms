package main

import (
	"context"
	"distributed_algorithms/raft"
	"distributed_algorithms/raft/config"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {

	ctx := context.Background()

	nodeID := os.Getenv("NODE_ID")
	nodeAddr := os.Getenv("NODE_ADDR")
	peerNodesEnv := os.Getenv("PEER_NODES")

	if nodeID == "" || nodeAddr == "" || peerNodesEnv == "" {
		panic("NODE_ID, NODE_ADDR and PEER_NODES environment variables must be set")
	}

	nodeArr := strings.Split(nodeAddr, ":")
	// nodeIPAddress := nodeArr[0]
	grpcListenPort, err := strconv.Atoi(nodeArr[1])
	if err != nil {
		panic("invalid node address")
	}

	// Parse PEER_NODES JSON string into a map
	var peerNodes map[string]string
	if peerNodesEnv != "" {
		err := json.Unmarshal([]byte(peerNodesEnv), &peerNodes)
		if err != nil {
			panic(fmt.Errorf("failed to parse PEER_NODES[%s], err: %v", peerNodesEnv, err))
		}
	} else {
		panic(fmt.Errorf("emoty PEER_NODES env variable: %v", err))
	}

	raftService, err := raft.NewRaftService(ctx, nodeID,
		raft.WithGrpcPort(grpcListenPort),
		raft.WithRaftConfig(config.DefaultConfig.RaftCfg),
		raft.WithPeerNodes(peerNodes),
	)
	if err != nil {
		panic(err)
	}

	raftGroupName := "raft-group"
	raftService.RegisterRaftGroupWithName(raftGroupName)

	raftService.Start()

	for {
		cancelCtx, cancelFunc := context.WithCancel(ctx)
		raft.WaitForFollowerToLeaderToFollowerCycle(cancelCtx, cancelFunc, raftService, raftGroupName)

		// Wait till Follower->Leader->Follower cycle completes
		<-cancelCtx.Done()
	}
}
