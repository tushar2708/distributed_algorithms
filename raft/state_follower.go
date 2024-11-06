package raft

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"time"

	"distributed_algorithms/raft/config"
	"distributed_algorithms/raft/state"
)

type FollowerState struct {
	*BaseNodeState
	stateTransitionCtx context.Context
}

// NewFollowerState creates a new instance of FollowerState
func NewFollowerState(ctx context.Context, raftGroup RaftGroup, electionData *state.RaftStateData,
	raftOperations RaftOperations) state.NodeState {

	return &FollowerState{
		stateTransitionCtx: context.Background(),
		BaseNodeState: &BaseNodeState{
			ctx:            ctx,
			RaftGroup:      raftGroup,
			raftOperations: raftOperations,
			State:          state.Follower,
			ElectionData:   electionData,
		},
	}
}

// Enter implements state.NodeState.
func (f *FollowerState) Enter(term uint64) {
	f.mu.Lock()
	f.stateTransitionCtx = context.Background()
	f.mu.Unlock()

	f.ElectionData.SetStateAndTerm(state.Follower, term)
	f.ElectionData.ResetElectionTimer()
	f.ElectionData.ResetVote()

	go f.waitToBecomeACandidate()

	f.RaftGroup.NotifyStateChange(state.Follower)
}

// Exit implements state.NodeState.
func (f *FollowerState) Exit() {
	f.mu.Lock()
	defer f.mu.Unlock()

	log.Printf("node:[%s] exiting state: [%s] ", f.RaftGroup.NodeID(), f.StateName())

	if f.stateTransitionCtx != nil {
		f.stateTransitionCtx.Done()
	}
	f.stateTransitionCtx = nil
}

// EvaluateTerm implements state.NodeState.
func (f *FollowerState) EvaluateTerm(offeredTerm uint64, offeredLeader string) (newTerm uint64, termAccepted bool, err error) {
	currentTerm := f.ElectionData.GetCurrentTerm()

	if currentTerm > offeredTerm {

		f.RaftGroup.Transition(NewCandidateState(f.ctx, f.RaftGroup, f.ElectionData, f.raftOperations), currentTerm)

		return currentTerm, false, fmt.Errorf("current term: %d, offered term: %d, err:%w",
			currentTerm, offeredTerm, ErrCurrentNodeHasHigherTerm)
	}

	f.ElectionData.SetTerm(offeredTerm)
	f.ElectionData.SetCurrentLeader(offeredLeader)
	return offeredTerm, true, fmt.Errorf("current term: %d, offered term: %d, err: %w",
		currentTerm, offeredTerm, ErrCurrentNodeHasEqualOrLowerTerm)
}

// waitToBecomeACandidate runs an timer,
//
//	towards becoming a candidate in the next term.
//
// This is a blocking function, that should be launched in a separate goroutine;
// It's a one time waiting opertion, and it exits as soon as the state changes from follower/candidate or the term changes.
func (f *FollowerState) waitToBecomeACandidate() {

	ticker := time.NewTicker(config.WaitForElection)
	defer ticker.Stop()

	for {
		timeoutDuration := electionTimeout()

		termWhenElectionStarted := f.ElectionData.GetCurrentTerm()

		log.Printf("timer to attempt election started, current term:[%d],"+
			" election will be attempted in (%d ms)",
			termWhenElectionStarted, timeoutDuration.Nanoseconds()/1e6)

		err := f.waitForSingleElectionTerm(ticker, termWhenElectionStarted, timeoutDuration)
		log.Printf("waitForSingleElectionTerm returned err:[%v]", err)
		if !errors.Is(err, ErrRetryElectionInNextTerm) {
			// stop running election
			log.Printf("no need to wait for election any longer, err:[%v]", err)
			break
		}
		log.Printf("waitForSingleElectionTerm returned err:[%v], try again...", err)

	}
}

func (f *FollowerState) waitForSingleElectionTerm(ticker *time.Ticker,
	termWhenElectionStarted uint64, timeoutDuration time.Duration) error {

	// This loops until either:
	// - we discover the election timer is no longer needed, or
	// - the election timer expires and this CS becomes a candidate
	// In a follower, this typically keeps running in the background for the
	// duration of the CS's lifetime.

	for {
		// if stateTransitionCtx is nil, it means node had exited from follower state
		f.mu.Lock()
		if f.stateTransitionCtx == nil {
			f.mu.Unlock()
			return nil
		}
		stTransitionCtx := f.stateTransitionCtx
		f.mu.Unlock()

		select {
		case <-stTransitionCtx.Done():
			log.Printf("this node has exited from the follower state already,currentState:[%s], cleaning up ",
				f.ElectionData.GetState().String())
			return nil
		case <-ticker.C: // block & wait for the ticker

			// State could have changed, while we were waiting for the ticker
			// If state is leader or dead, we can abort and return
			if f.ElectionData.GetState() != state.Candidate && f.ElectionData.GetState() != state.Follower {
				log.Printf("woke up for election, but state=%s is not Candidate/Follower, bailing out",
					f.ElectionData.GetState().String())

				return nil
			}
		}

		// has the term changed while we were looping here?
		// It would have changed because we received a heartbeat from the leader node
		// We have a leader, so no need to go for an election right now
		if termWhenElectionStarted < f.ElectionData.GetCurrentTerm() {
			log.Printf("woke up for election, but term has changed from %d to %d,"+
				" no need to go for an election right now",
				termWhenElectionStarted, f.ElectionData.GetCurrentTerm())

			// return appropriate error response to retry in the next term
			return ErrRetryElectionInNextTerm
		}

		// Start an election if we haven't heard from a leader or haven't voted for
		// someone for the duration of the timeout.
		if elapsed := time.Since(f.ElectionData.GetElectionResetTime()); elapsed >= timeoutDuration {
			log.Printf("woke up for election, %d ms have been passed since last reset, "+
				"which is greater than timeout duration %d ms, time to become a candidate",
				elapsed.Nanoseconds()/1e6, timeoutDuration.Nanoseconds()/1e6)

			// let's become a candidate, and try to become a leader
			f.RaftGroup.Transition(NewCandidateState(f.ctx, f.RaftGroup, f.ElectionData, f.raftOperations),
				f.ElectionData.GetCurrentTerm())

			return nil
		}
	}
}

// electionTimeout generates a pseudo-random election timeout duration.
func electionTimeout() time.Duration {
	randBig, err := rand.Int(rand.Reader, big.NewInt(int64(
		config.DefaultConfig.RaftCfg.DeltaElectionTimeoutInMs)))
	if err != nil {
		log.Printf("unable to generate random number error:[%v]", err)

		// can't generate a random number, return a fixed timeout duration
		return time.Duration(config.DefaultConfig.RaftCfg.DeltaElectionTimeoutInMs) * time.Millisecond
	}

	return time.Duration(
		config.DefaultConfig.RaftCfg.ElectionTimeoutInMs+int(randBig.Int64())) * time.Millisecond
}
