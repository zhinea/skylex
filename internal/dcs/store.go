package dcs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	defaultDialTimeout = 5 * time.Second
	defaultOpTimeout   = 10 * time.Second

	keyPrefix = "/skylex"
	leaderKey = keyPrefix + "/leader"
	memberKey = keyPrefix + "/members"
	primaryKey = keyPrefix + "/primary"
)

type Store struct {
	client *clientv3.Client
	log    *slog.Logger
}

type Config struct {
	Endpoints   []string
	DialTimeout time.Duration
	Username    string
	Password    string
}

func New(cfg Config, log *slog.Logger) (*Store, error) {
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = defaultDialTimeout
	}

	clientCfg := clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: cfg.DialTimeout,
	}

	if cfg.Username != "" {
		clientCfg.Username = cfg.Username
		clientCfg.Password = cfg.Password
	}

	client, err := clientv3.New(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("etcd client: %w", err)
	}

	log.Info("etcd connected", "endpoints", cfg.Endpoints)

	return &Store{
		client: client,
		log:    log,
	}, nil
}

func (s *Store) Close() error {
	return s.client.Close()
}

func (s *Store) Client() *clientv3.Client {
	return s.client
}

func (s *Store) Get(ctx context.Context, key string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return "", fmt.Errorf("etcd get %s: %w", key, err)
	}
	if len(resp.Kvs) == 0 {
		return "", fmt.Errorf("key %s not found", key)
	}
	return string(resp.Kvs[0].Value), nil
}

func (s *Store) Put(ctx context.Context, key, value string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	_, err := s.client.Put(ctx, key, value)
	if err != nil {
		return fmt.Errorf("etcd put %s: %w", key, err)
	}
	return nil
}

func (s *Store) PutWithLease(ctx context.Context, key, value string, leaseID clientv3.LeaseID) error {
	ctx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	_, err := s.client.Put(ctx, key, value, clientv3.WithLease(leaseID))
	if err != nil {
		return fmt.Errorf("etcd put %s with lease: %w", key, err)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	_, err := s.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("etcd delete %s: %w", key, err)
	}
	return nil
}

func (s *Store) GetPrefix(ctx context.Context, prefix string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("etcd get prefix %s: %w", prefix, err)
	}

	result := make(map[string]string, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		result[string(kv.Key)] = string(kv.Value)
	}
	return result, nil
}

func (s *Store) Watch(ctx context.Context, key string) clientv3.WatchChan {
	return s.client.Watch(ctx, key)
}

func (s *Store) WatchPrefix(ctx context.Context, prefix string) clientv3.WatchChan {
	return s.client.Watch(ctx, prefix, clientv3.WithPrefix())
}

func (s *Store) GrantLease(ctx context.Context, ttl int64) (clientv3.LeaseID, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	resp, err := s.client.Grant(ctx, ttl)
	if err != nil {
		return 0, fmt.Errorf("grant lease: %w", err)
	}
	return resp.ID, nil
}

func (s *Store) KeepAlive(ctx context.Context, leaseID clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error) {
	return s.client.KeepAlive(ctx, leaseID)
}

func (s *Store) RevokeLease(ctx context.Context, leaseID clientv3.LeaseID) error {
	ctx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	_, err := s.client.Revoke(ctx, leaseID)
	return err
}