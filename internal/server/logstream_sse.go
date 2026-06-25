package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
)

// sseHeartbeat is how often we send a comment line to keep proxies/load
// balancers from idle-closing a quiet stream and to detect dead connections.
const sseHeartbeat = 25 * time.Second

// handleStreamCommandLogs serves command logs over Server-Sent Events.
//
// Flow: it first replays a recent backlog (so a freshly opened tab is never
// blank) then switches to live push from the in-memory LogBroker. Auth is
// already enforced by connectInterceptors before this handler runs, so the
// caller is a verified user; viewers are allowed (read-only).
//
// Filters come from the query string and mirror ListNodeCommandLogs:
// ?clusterId=..., ?nodeId=..., ?commandId=... (at least one required).
//
// Limitation: the live half is backed by an in-memory broker, so it only
// streams events produced by THIS server process. The durable backlog (SQLite)
// is always authoritative; a multi-instance deployment would need an external
// pub/sub bus to fan live events across processes.
func (s *Server) handleStreamCommandLogs(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	q := r.URL.Query()
	clusterID := q.Get("clusterId")
	nodeID := q.Get("nodeId")
	commandID := q.Get("commandId")
	if clusterID == "" && nodeID == "" && commandID == "" {
		http.Error(w, "clusterId, nodeId, or commandId is required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	ctx := r.Context()

	// Subscribe BEFORE loading the backlog so no event emitted during backlog
	// load is missed. Duplicates between backlog and live are de-duped by id on
	// the client.
	subID, ch := s.logBroker.Subscribe(clusterID, nodeID, commandID)
	defer s.logBroker.Unsubscribe(subID)

	// Replay backlog newest-window in chronological order.
	backlog, err := s.nodeService.ListNodeCommandLogs(ctx, &skylexv1.ListNodeCommandLogsRequest{
		ClusterId: clusterID,
		NodeId:    nodeID,
		CommandId: commandID,
		Page:      1,
		PageSize:  200,
	})
	if err == nil {
		for _, l := range backlog.GetLogs() {
			if !writeLogEvent(w, l) {
				return
			}
		}
		flusher.Flush()
	} else {
		s.log.Warn("sse backlog load failed", "error", err)
	}

	heartbeat := time.NewTicker(sseHeartbeat)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case log, ok := <-ch:
			if !ok {
				return
			}
			if !writeLogEvent(w, log) {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeLogEvent serializes one log as an SSE "data:" frame. Returns false if the
// connection is no longer writable.
func writeLogEvent(w http.ResponseWriter, log *skylexv1.CommandLog) bool {
	payload := struct {
		ID          string `json:"id"`
		CommandID   string `json:"commandId"`
		NodeID      string `json:"nodeId"`
		Hostname    string `json:"hostname"`
		Level       string `json:"level"`
		Message     string `json:"message"`
		TimestampMs int64  `json:"timestampMs"`
	}{
		ID:          log.GetId(),
		CommandID:   log.GetCommandId(),
		NodeID:      log.GetNodeId(),
		Hostname:    log.GetHostname(),
		Level:       log.GetLevel(),
		Message:     log.GetMessage(),
		TimestampMs: log.GetTimestampMs(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return true // skip a single bad row, keep the stream alive
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err == nil
}
