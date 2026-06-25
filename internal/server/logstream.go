package server

import (
	"sync"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
)

// logSubscriberBuffer is the per-subscriber channel depth. A slow consumer that
// fills this buffer is dropped rather than allowed to block the publisher
// (back-pressure isolation): one stuck browser tab must never stall agent log
// ingestion for everyone else.
const logSubscriberBuffer = 256

// logSubscriber is a single live stream consumer (one SSE connection).
type logSubscriber struct {
	id        uint64
	clusterID string // filter; empty means match any cluster
	nodeID    string // filter; empty means match any node
	commandID string // filter; empty means match any command
	ch        chan *skylexv1.CommandLog
}

func (s *logSubscriber) matches(log *skylexv1.CommandLog) bool {
	if s.commandID != "" {
		return s.commandID == log.GetCommandId()
	}
	if s.nodeID != "" {
		return s.nodeID == log.GetNodeId()
	}
	// clusterID filtering is applied at publish time (the publisher knows the
	// cluster for each log batch); an empty-filter subscriber matches all.
	return true
}

// LogBroker is an in-memory pub/sub hub fanning command-log entries out to live
// SSE subscribers. It is intentionally single-instance: streaming is correct for
// the one control-plane process this binary runs. Horizontal scale-out would
// require an external bus (e.g. Redis pub/sub) — out of scope here, see
// StreamNodeCommandLogs docs. Durable history always comes from SQLite, so a
// dropped/missed live message is only a latency hit, never data loss.
type LogBroker struct {
	mu          sync.RWMutex
	subscribers map[uint64]*logSubscriber
	nextID      uint64
}

func NewLogBroker() *LogBroker {
	return &LogBroker{
		subscribers: make(map[uint64]*logSubscriber),
	}
}

// Subscribe registers a new consumer. The returned channel delivers matching
// logs; unsubscribe (always, via defer) releases it. The filter args mirror the
// ListNodeCommandLogs selectors: pass empty strings for "any".
func (b *LogBroker) Subscribe(clusterID, nodeID, commandID string) (uint64, <-chan *skylexv1.CommandLog) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	sub := &logSubscriber{
		id:        b.nextID,
		clusterID: clusterID,
		nodeID:    nodeID,
		commandID: commandID,
		ch:        make(chan *skylexv1.CommandLog, logSubscriberBuffer),
	}
	b.subscribers[sub.id] = sub
	return sub.id, sub.ch
}

func (b *LogBroker) Unsubscribe(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if sub, ok := b.subscribers[id]; ok {
		delete(b.subscribers, id)
		close(sub.ch)
	}
}

// Publish fans a batch of logs out to all matching subscribers. clusterID is the
// cluster the whole batch belongs to (used for cluster-scoped subscribers, since
// individual CommandLog protos don't carry it). Delivery is non-blocking: if a
// subscriber's buffer is full the message is dropped for that subscriber only.
func (b *LogBroker) Publish(clusterID string, logs []*skylexv1.CommandLog) {
	if len(logs) == 0 {
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.subscribers) == 0 {
		return
	}

	for _, sub := range b.subscribers {
		if sub.clusterID != "" && sub.clusterID != clusterID {
			continue
		}
		for _, log := range logs {
			if !sub.matches(log) {
				continue
			}
			select {
			case sub.ch <- log:
			default:
				// Subscriber is too slow; drop to protect the publisher. The
				// browser reconciles missed entries from the durable backlog on
				// its next poll/reconnect.
			}
		}
	}
}

// SubscriberCount reports active live subscribers (used by tests/metrics).
func (b *LogBroker) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}
