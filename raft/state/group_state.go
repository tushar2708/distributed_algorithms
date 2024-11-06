package state

// GroupState represent raft group's state, with respect to the current node
type GroupState struct {
	IsLeader      bool
	CurrentLeader string
	CurrentTerm   uint64
}

