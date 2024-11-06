package raft

import (
	"context"
	"log"

	"distributed_algorithms/raft/state"
)

func WaitForFollowerToLeaderToFollowerCycle(
	ctx context.Context,
	cancelFunc context.CancelFunc,
	service *RaftService,
	raftGroup string,
) {
	stateChangeCh, _ := service.GetStateNotifierChan(raftGroup)
	for {
		var nodeState state.NodeStateEnum
		// if we get anything on the channel, update the nodeState
		select {
		case ns := <-stateChangeCh:
			nodeState = ns
		case <-ctx.Done():
			return
		}

		if nodeState == state.Leader {

			log.Printf("This node became the leader from being a follower in the raft group:[%s]. Yaaayyyyyy.....!!", raftGroup)

			// start a goroutine to monitor when this node turns from a leader to a follower
			go WaitTillNodeMovesFromLeaderToFollower(ctx, cancelFunc, service, raftGroup)

			return
		}
	}
}

func WaitTillNodeMovesFromLeaderToFollower(
	ctx context.Context, cancelFunc context.CancelFunc,
	service *RaftService, raftGroup string,
) {

	notifierChannel, _ := service.GetStateNotifierChan(raftGroup)
	for {
		var nodeState state.NodeStateEnum
		// if we got anything on the channel, update the nodeState
		select {
		case ns := <-notifierChannel:
			nodeState = ns
		case <-ctx.Done():
			return
		}

		if nodeState != state.Follower {

			log.Printf("This node became the follower in the raft group:[%s]. Boooooo.....!!", raftGroup)

			cancelFunc()

			return
		}
	}
}
