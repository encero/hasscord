package hass

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type wsScript func(t *testing.T, conn *websocket.Conn)

type scriptedWSServer struct {
	t           *testing.T
	server      *httptest.Server
	scripts     chan wsScript
	connections atomic.Int32
}

func newScriptedWSServer(t *testing.T) *scriptedWSServer {
	t.Helper()

	s := &scriptedWSServer{
		t:       t,
		scripts: make(chan wsScript, 8),
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		s.connections.Add(1)

		script, ok := <-s.scripts
		if !ok {
			t.Errorf("no script queued for connection")
			return
		}

		script(t, conn)
	}))

	t.Cleanup(func() {
		close(s.scripts)
		s.server.Close()
	})

	return s
}

func (s *scriptedWSServer) URL() string {
	return "ws" + strings.TrimPrefix(s.server.URL, "http")
}

func (s *scriptedWSServer) Enqueue(script wsScript) {
	s.scripts <- script
}

func (s *scriptedWSServer) ConnectionCount() int {
	return int(s.connections.Load())
}

func expectAuthOK(t *testing.T, conn *websocket.Conn, token string) {
	t.Helper()

	if err := conn.WriteJSON(Message{Type: "auth_required"}); err != nil {
		t.Fatalf("write auth_required: %v", err)
	}

	var auth map[string]string
	if err := conn.ReadJSON(&auth); err != nil {
		t.Fatalf("read auth request: %v", err)
	}

	if auth["type"] != "auth" {
		t.Fatalf("unexpected auth request type %q", auth["type"])
	}
	if auth["access_token"] != token {
		t.Fatalf("unexpected token %q", auth["access_token"])
	}

	if err := conn.WriteJSON(Message{Type: "auth_ok"}); err != nil {
		t.Fatalf("write auth_ok: %v", err)
	}
}

func expectAuthInvalid(t *testing.T, conn *websocket.Conn, token string, message string) {
	t.Helper()

	if err := conn.WriteJSON(Message{Type: "auth_required"}); err != nil {
		t.Fatalf("write auth_required: %v", err)
	}

	var auth map[string]string
	if err := conn.ReadJSON(&auth); err != nil {
		t.Fatalf("read auth request: %v", err)
	}

	if auth["type"] != "auth" {
		t.Fatalf("unexpected auth request type %q", auth["type"])
	}
	if auth["access_token"] != token {
		t.Fatalf("unexpected token %q", auth["access_token"])
	}

	if err := conn.WriteJSON(Message{
		Type: "auth_invalid",
		Error: &struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{
			Code:    "auth_invalid",
			Message: message,
		},
	}); err != nil {
		t.Fatalf("write auth_invalid: %v", err)
	}
}

func expectSubscribeRequest(t *testing.T, conn *websocket.Conn) int {
	t.Helper()

	var req struct {
		ID   int    `json:"id"`
		Type string `json:"type"`
	}

	if err := conn.ReadJSON(&req); err != nil {
		t.Fatalf("read subscribe request: %v", err)
	}

	if req.Type != "subscribe_events" {
		t.Fatalf("unexpected request type %q", req.Type)
	}

	return req.ID
}

func writeSubscribeResult(t *testing.T, conn *websocket.Conn, id int, success bool, message string) {
	t.Helper()

	result := Message{
		ID:      id,
		Type:    "result",
		Success: success,
	}
	if !success {
		result.Error = &struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{
			Code:    "subscribe_failed",
			Message: message,
		}
	}

	if err := conn.WriteJSON(result); err != nil {
		t.Fatalf("write subscribe result: %v", err)
	}
}

func writeEvent(t *testing.T, conn *websocket.Conn, entityID string) {
	t.Helper()

	data, err := json.Marshal(StateChangedData{
		EntityID: entityID,
		NewState: State{EntityID: entityID, State: "on"},
		OldState: State{EntityID: entityID, State: "off"},
	})
	if err != nil {
		t.Fatalf("marshal event data: %v", err)
	}

	if err := conn.WriteJSON(Message{
		Type: "event",
		Event: &Event{
			EventType: "state_changed",
			Data:      data,
			Origin:    "LOCAL",
			TimeFired: "2026-04-05T12:00:00Z",
		},
	}); err != nil {
		t.Fatalf("write event: %v", err)
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool, description string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", description)
}

func readEvent(t *testing.T, events <-chan Event) Event {
	t.Helper()

	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("event channel closed unexpectedly")
		}
		return event
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}

func TestManagerTryConnectOnceSuccess(t *testing.T) {
	t.Parallel()

	const token = "test-token"

	server := newScriptedWSServer(t)
	server.Enqueue(func(t *testing.T, conn *websocket.Conn) {
		expectAuthOK(t, conn, token)
		id := expectSubscribeRequest(t, conn)
		writeSubscribeResult(t, conn, id, true, "")
		_, _, _ = conn.ReadMessage()
	})

	manager := NewManager(ManagerConfig{
		URL:            server.URL(),
		Token:          token,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
	})

	if !manager.TryConnectOnce() {
		t.Fatal("expected initial connection to succeed")
	}

	if !manager.IsConnected() {
		t.Fatal("expected manager to report connected")
	}

	if manager.GetClient() == nil {
		t.Fatal("expected manager client to be available")
	}

	if got := server.ConnectionCount(); got != 1 {
		t.Fatalf("expected 1 connection, got %d", got)
	}

	manager.closeCurrentConnection()
}

func TestManagerTryConnectOnceAuthFailure(t *testing.T) {
	t.Parallel()

	const token = "test-token"

	server := newScriptedWSServer(t)
	server.Enqueue(func(t *testing.T, conn *websocket.Conn) {
		expectAuthInvalid(t, conn, token, "bad token")
	})

	manager := NewManager(ManagerConfig{
		URL:            server.URL(),
		Token:          token,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
	})

	if manager.TryConnectOnce() {
		t.Fatal("expected initial connection to fail")
	}

	if manager.IsConnected() {
		t.Fatal("manager should not report connected after auth failure")
	}

	if manager.GetClient() != nil {
		t.Fatal("manager client should be nil after auth failure")
	}
}

func TestManagerTryConnectOnceSubscriptionFailure(t *testing.T) {
	t.Parallel()

	const token = "test-token"

	server := newScriptedWSServer(t)
	server.Enqueue(func(t *testing.T, conn *websocket.Conn) {
		expectAuthOK(t, conn, token)
		id := expectSubscribeRequest(t, conn)
		writeSubscribeResult(t, conn, id, false, "subscription denied")
	})

	manager := NewManager(ManagerConfig{
		URL:            server.URL(),
		Token:          token,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
	})

	if manager.TryConnectOnce() {
		t.Fatal("expected initial connection to fail")
	}

	if manager.IsConnected() {
		t.Fatal("manager should not report connected after subscription failure")
	}

	if manager.GetClient() != nil {
		t.Fatal("manager client should be nil after subscription failure")
	}
}

func TestManagerRunStopsAfterMaxRetries(t *testing.T) {
	t.Parallel()

	const token = "test-token"

	server := newScriptedWSServer(t)
	server.Enqueue(func(t *testing.T, conn *websocket.Conn) {
		expectAuthInvalid(t, conn, token, "first failure")
	})
	server.Enqueue(func(t *testing.T, conn *websocket.Conn) {
		expectAuthInvalid(t, conn, token, "second failure")
	})

	manager := NewManager(ManagerConfig{
		URL:            server.URL(),
		Token:          token,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
		MaxRetries:     1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		manager.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("manager did not stop after exceeding max retries")
	}

	if got := server.ConnectionCount(); got != 2 {
		t.Fatalf("expected 2 connection attempts, got %d", got)
	}

	select {
	case _, ok := <-manager.Events():
		if ok {
			t.Fatal("expected event channel to be closed")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for event channel to close")
	}
}

func TestManagerRunReusesInitialConnectionAndRecovers(t *testing.T) {
	t.Parallel()

	const token = "test-token"

	server := newScriptedWSServer(t)
	sendFirstEvent := make(chan struct{})
	sendSecondEvent := make(chan struct{})
	secondConnectionReady := make(chan struct{})

	server.Enqueue(func(t *testing.T, conn *websocket.Conn) {
		expectAuthOK(t, conn, token)
		id := expectSubscribeRequest(t, conn)
		writeSubscribeResult(t, conn, id, true, "")

		<-sendFirstEvent
		writeEvent(t, conn, "binary_sensor.front_door")
		_ = conn.Close()
	})

	server.Enqueue(func(t *testing.T, conn *websocket.Conn) {
		expectAuthOK(t, conn, token)
		id := expectSubscribeRequest(t, conn)
		writeSubscribeResult(t, conn, id, true, "")
		close(secondConnectionReady)

		<-sendSecondEvent
		writeEvent(t, conn, "binary_sensor.back_door")

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})

	manager := NewManager(ManagerConfig{
		URL:            server.URL(),
		Token:          token,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
	})

	if !manager.TryConnectOnce() {
		t.Fatal("expected initial connection to succeed")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		manager.Run(ctx)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	if got := server.ConnectionCount(); got != 1 {
		t.Fatalf("expected Run to reuse the initial connection, got %d connections", got)
	}

	sendFirstEvent <- struct{}{}
	firstEvent := readEvent(t, manager.Events())
	if firstEvent.EventType != "state_changed" {
		t.Fatalf("unexpected first event type %q", firstEvent.EventType)
	}

	select {
	case <-secondConnectionReady:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for reconnect")
	}

	waitForCondition(t, 200*time.Millisecond, manager.IsConnected, "manager reconnect")

	sendSecondEvent <- struct{}{}
	secondEvent := readEvent(t, manager.Events())
	if secondEvent.EventType != "state_changed" {
		t.Fatalf("unexpected second event type %q", secondEvent.EventType)
	}

	if got := server.ConnectionCount(); got != 2 {
		t.Fatalf("expected 2 total connections after recovery, got %d", got)
	}

	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("manager did not stop after cancellation")
	}

	select {
	case _, ok := <-manager.Events():
		if ok {
			t.Fatal("expected event channel to be closed")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for event channel to close after cancellation")
	}
}
