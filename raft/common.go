package raft

import "errors"

var (
	ErrCurrentNodeHasHigherTerm        = errors.New("current node has higher term")
	ErrCurrentNodeHasLowerTerm         = errors.New("current node has lower term")
	ErrCurrentNodeHasEqualOrHigherTerm = errors.New("current node has equal orhigher term")
	ErrCurrentNodeHasEqualOrLowerTerm  = errors.New("current node has equal or lower term")
	ErrCurrentNodeHasSameTerm          = errors.New("current node has the same term")
	ErrRetryElectionInNextTerm         = errors.New("didn't attempt election in this term, try again later")
	ErrThisNodeIsNotTheLeader          = errors.New("this node is not the leader")
	ErrThisNodeIsMarkedDead            = errors.New("this node is marked dead")
)
