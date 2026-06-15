package dcs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type MemberInfo struct {
	ID        string `json:"id"`
	Hostname  string `json:"hostname"`
	Address   string `json:"address"`
	Role      string `json:"role"`
	ClusterID string `json:"cluster_id"`
}

func (s *Store) RegisterMember(ctx context.Context, member MemberInfo, ttl int64) (clientv3.LeaseID, error) {
	leaseID, err := s.GrantLease(ctx, ttl)
	if err != nil {
		return 0, fmt.Errorf("grant member lease: %w", err)
	}

	data, err := json.Marshal(member)
	if err != nil {
		return 0, fmt.Errorf("marshal member: %w", err)
	}

	key := fmt.Sprintf("%s/%s", memberKey, member.ID)
	if err := s.PutWithLease(ctx, key, string(data), leaseID); err != nil {
		return 0, fmt.Errorf("register member: %w", err)
	}

	go s.keepMemberAlive(context.Background(), leaseID, member.ID)

	s.log.Info("member registered in etcd",
		"member_id", member.ID,
		"hostname", member.Hostname,
		"role", member.Role,
	)

	return leaseID, nil
}

func (s *Store) keepMemberAlive(ctx context.Context, leaseID clientv3.LeaseID, memberID string) {
	ch, err := s.KeepAlive(ctx, leaseID)
	if err != nil {
		s.log.Error("keep member alive failed", "member_id", memberID, "error", err)
		return
	}

	for range ch {
	}

	s.log.Warn("member lease expired", "member_id", memberID)
}

func (s *Store) GetMembers(ctx context.Context) ([]MemberInfo, error) {
	kv, err := s.GetPrefix(ctx, memberKey)
	if err != nil {
		return nil, fmt.Errorf("get members: %w", err)
	}

	var members []MemberInfo
	for _, v := range kv {
		var m MemberInfo
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			s.log.Warn("unmarshal member failed", "error", err)
			continue
		}
		members = append(members, m)
	}
	return members, nil
}

func (s *Store) DeregisterMember(ctx context.Context, memberID string) error {
	key := fmt.Sprintf("%s/%s", memberKey, memberID)
	return s.Delete(ctx, key)
}

func (s *Store) WatchMembers(ctx context.Context) clientv3.WatchChan {
	return s.WatchPrefix(ctx, memberKey)
}

type PrimaryInfo struct {
	ClusterID string    `json:"cluster_id"`
	NodeID    string    `json:"node_id"`
	Hostname  string    `json:"hostname"`
	Address   string    `json:"address"`
	Port      int       `json:"port"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Store) SetPrimary(ctx context.Context, clusterID string, info PrimaryInfo) error {
	key := fmt.Sprintf("%s/%s", primaryKey, clusterID)
	info.UpdatedAt = time.Now().UTC()

	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal primary: %w", err)
	}

	return s.Put(ctx, key, string(data))
}

func (s *Store) GetPrimary(ctx context.Context, clusterID string) (*PrimaryInfo, error) {
	key := fmt.Sprintf("%s/%s", primaryKey, clusterID)
	val, err := s.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get primary: %w", err)
	}

	var info PrimaryInfo
	if err := json.Unmarshal([]byte(val), &info); err != nil {
		return nil, fmt.Errorf("unmarshal primary: %w", err)
	}
	return &info, nil
}

func (s *Store) GetAllPrimaries(ctx context.Context) (map[string]PrimaryInfo, error) {
	kv, err := s.GetPrefix(ctx, primaryKey)
	if err != nil {
		return nil, fmt.Errorf("get all primaries: %w", err)
	}

	result := make(map[string]PrimaryInfo, len(kv))
	for _, v := range kv {
		var info PrimaryInfo
		if err := json.Unmarshal([]byte(v), &info); err != nil {
			s.log.Warn("unmarshal primary failed", "error", err)
			continue
		}
		result[info.ClusterID] = info
	}
	return result, nil
}