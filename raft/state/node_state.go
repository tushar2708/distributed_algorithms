package state

// NodeStateEnum is a type that signifies Node's state in the raft's state machine
type NodeStateEnum int

// Different NodeStateEnum values
const (
	Uninitialized NodeStateEnum = iota
	Follower
	Candidate
	Leader
	Dead
)

// String returns string representation of NodeState
func (s NodeStateEnum) String() string {
	switch s {
	case Follower:
		return "Follower"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	case Dead:
		return "Dead"
	default:
		return "Invalid NodeState"
	}
}

// NodeState is the interface that every state in the leader election's state machine implements
type NodeState interface {
	Enter(term uint64)
	Exit()
	EvaluateTerm(offeredTerm uint64, offeredLeader string) (newTerm uint64, termAccepted bool, err error)
	StateName() NodeStateEnum
	GetRaftStateData() *RaftStateData
	SetRaftStateData(data *RaftStateData)
}
