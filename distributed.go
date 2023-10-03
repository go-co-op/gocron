package gocron

import "context"

// Elector determines the leader from instances asking to be the leader. Only
// the leader runs jobs. If the leader goes down, a new leader will be elected.
type Elector interface {
	// IsLeader should return  nil if the job should be scheduled by the instance
	// making the request and an error if the job should not be scheduled.
	IsLeader(ctx context.Context) error
}
