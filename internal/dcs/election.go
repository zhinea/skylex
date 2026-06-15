package dcs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

type LeaderElector struct {
	store    *Store
	session  *concurrency.Session
	election *concurrency.Election
	log      *slog.Logger
}

func NewLeaderElector(store *Store, log *slog.Logger) *LeaderElector {
	return &LeaderElector{
		store: store,
		log:   log,
	}
}

func (e *LeaderElector) Campaign(ctx context.Context, candidateID string, ttl int) (<-chan struct{}, error) {
	if e.session != nil {
		e.session.Close()
	}

	session, err := concurrency.NewSession(e.store.Client(), concurrency.WithTTL(ttl))
	if err != nil {
		return nil, fmt.Errorf("create election session: %w", err)
	}
	e.session = session

	e.election = concurrency.NewElection(session, leaderKey)

	e.log.Info("starting leader election campaign", "candidate_id", candidateID)

	if err := e.election.Campaign(ctx, candidateID); err != nil {
		e.session.Close()
		return nil, fmt.Errorf("campaign: %w", err)
	}

	e.log.Info("elected as leader", "candidate_id", candidateID)

	leaderCh := make(chan struct{})
	go func() {
		<-session.Done()
		close(leaderCh)
		e.log.Warn("leader session expired", "candidate_id", candidateID)
	}()

	return leaderCh, nil
}

func (e *LeaderElector) Resign(ctx context.Context) error {
	if e.election != nil {
		if err := e.election.Resign(ctx); err != nil {
			return fmt.Errorf("resign: %w", err)
		}
	}
	if e.session != nil {
		e.session.Close()
	}
	e.log.Info("resigned from leader election")
	return nil
}

func (e *LeaderElector) GetLeader(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := e.store.Client().Get(ctx, leaderKey, clientv3.WithFirstCreate()...)
	if err != nil {
		return "", fmt.Errorf("get leader: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return "", fmt.Errorf("no leader elected")
	}
	return string(resp.Kvs[0].Value), nil
}

func (e *LeaderElector) Observe(ctx context.Context) clientv3.WatchChan {
	return e.store.Client().Watch(ctx, leaderKey, clientv3.WithPrefix())
}

func (e *LeaderElector) IsLeader() bool {
	if e.election == nil || e.session == nil {
		return false
	}
	select {
	case <-e.session.Done():
		return false
	default:
		return true
	}
}

func (e *LeaderElector) Close() error {
	if e.session != nil {
		return e.session.Close()
	}
	return nil
}