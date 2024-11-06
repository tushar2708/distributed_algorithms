package config

import "time"

type RaftElectionConfig struct {
	ElectionTimeoutInMs      int
	DeltaElectionTimeoutInMs int
}

type Config struct {
	ElectionPort int
	PeerNodesMap map[string]string
	RaftCfg      *RaftElectionConfig
}

const (
	defaultQuorumPort              = 7000
	heartbeatInterval              = 2000 * time.Millisecond
	defaultElectionDurationMs      = 4000
	defaultElectionDurationDeltaMs = 3000
)

// Message types
const (
	ElectionTimeout            = 150 * time.Millisecond
	WaitForElection            = 200 * time.Millisecond
	DeltaBeforeElectionTimeout = 100 * time.Millisecond
	BaseElectionTimeout        = 400 * time.Millisecond
)

var (
	// DefaultConfig indicates default configurations for raft election
	DefaultConfig = newDefaultConfig()
)

func newDefaultConfig() Config {
	return Config{
		ElectionPort: defaultQuorumPort,
		RaftCfg: &RaftElectionConfig{
			ElectionTimeoutInMs:      defaultElectionDurationMs,
			DeltaElectionTimeoutInMs: defaultElectionDurationDeltaMs,
		},
	}
}
