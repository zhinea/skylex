package server

import (
	"testing"
	"time"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
)

func TestLogBroker_DeliversMatchingLogs(t *testing.T) {
	b := NewLogBroker()
	id, ch := b.Subscribe("", "node-1", "")
	defer b.Unsubscribe(id)

	b.Publish("cluster-1", []*skylexv1.CommandLog{
		{Id: "1", NodeId: "node-1", Message: "match"},
		{Id: "2", NodeId: "node-2", Message: "skip"},
	})

	select {
	case got := <-ch:
		if got.GetId() != "1" {
			t.Fatalf("expected log id 1, got %q", got.GetId())
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for matching log")
	}

	select {
	case got := <-ch:
		t.Fatalf("expected no further logs, got %q", got.GetId())
	case <-time.After(50 * time.Millisecond):
	}
}

func TestLogBroker_CommandFilterTakesPrecedence(t *testing.T) {
	b := NewLogBroker()
	id, ch := b.Subscribe("", "", "cmd-9")
	defer b.Unsubscribe(id)

	b.Publish("cluster-1", []*skylexv1.CommandLog{
		{Id: "1", CommandId: "cmd-1", NodeId: "node-1"},
		{Id: "2", CommandId: "cmd-9", NodeId: "node-2"},
	})

	got := <-ch
	if got.GetId() != "2" {
		t.Fatalf("expected log id 2 (cmd-9), got %q", got.GetId())
	}
}

func TestLogBroker_ClusterFilterIsolatesSubscribers(t *testing.T) {
	b := NewLogBroker()
	id, ch := b.Subscribe("cluster-A", "", "")
	defer b.Unsubscribe(id)

	b.Publish("cluster-B", []*skylexv1.CommandLog{{Id: "1", NodeId: "n"}})

	select {
	case got := <-ch:
		t.Fatalf("expected no cross-cluster delivery, got %q", got.GetId())
	case <-time.After(50 * time.Millisecond):
	}

	b.Publish("cluster-A", []*skylexv1.CommandLog{{Id: "2", NodeId: "n"}})
	if got := <-ch; got.GetId() != "2" {
		t.Fatalf("expected log id 2, got %q", got.GetId())
	}
}

func TestLogBroker_SlowSubscriberDoesNotBlockPublisher(t *testing.T) {
	b := NewLogBroker()
	id, _ := b.Subscribe("", "", "")
	defer b.Unsubscribe(id)

	// Publish far more than the buffer depth without draining; must not block.
	logs := make([]*skylexv1.CommandLog, logSubscriberBuffer*4)
	for i := range logs {
		logs[i] = &skylexv1.CommandLog{Id: "x", NodeId: "n"}
	}

	done := make(chan struct{})
	go func() {
		b.Publish("c", logs)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a full subscriber buffer")
	}
}

func TestLogBroker_UnsubscribeStopsDelivery(t *testing.T) {
	b := NewLogBroker()
	id, ch := b.Subscribe("", "", "")
	b.Unsubscribe(id)

	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed after unsubscribe")
	}
	if b.SubscriberCount() != 0 {
		t.Fatalf("expected 0 subscribers, got %d", b.SubscriberCount())
	}
}
