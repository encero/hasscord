package hass

import (
	"context"
	"log"
	"math"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Manager handles the Home Assistant connection lifecycle including reconnection.
type Manager struct {
	url          string
	token        string
	eventChannel chan Event
	client       *Client
	clientMu     sync.RWMutex
	connected    bool
	connectedMu  sync.RWMutex

	// Reconnection settings
	initialBackoff time.Duration
	maxBackoff     time.Duration
	maxRetries     int // 0 = unlimited
}

// ManagerConfig holds configuration for the connection manager.
type ManagerConfig struct {
	URL            string
	Token          string
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	MaxRetries     int // 0 = unlimited retries
}

// DefaultManagerConfig returns sensible defaults for the manager.
func DefaultManagerConfig(url, token string) ManagerConfig {
	return ManagerConfig{
		URL:            url,
		Token:          token,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     5 * time.Minute,
		MaxRetries:     0, // unlimited
	}
}

// NewManager creates a new Home Assistant connection manager.
func NewManager(cfg ManagerConfig) *Manager {
	return &Manager{
		url:            cfg.URL,
		token:          cfg.Token,
		eventChannel:   make(chan Event, 100),
		initialBackoff: cfg.InitialBackoff,
		maxBackoff:     cfg.MaxBackoff,
		maxRetries:     cfg.MaxRetries,
	}
}

// Events returns the event channel. This channel remains open across reconnections.
func (m *Manager) Events() <-chan Event {
	return m.eventChannel
}

// IsConnected returns whether the manager currently has an active connection.
func (m *Manager) IsConnected() bool {
	m.connectedMu.RLock()
	defer m.connectedMu.RUnlock()
	return m.connected
}

// setConnected updates the connection status.
func (m *Manager) setConnected(connected bool) {
	m.connectedMu.Lock()
	defer m.connectedMu.Unlock()
	m.connected = connected
}

// GetClient returns the current client (may be nil if not connected).
func (m *Manager) GetClient() *Client {
	m.clientMu.RLock()
	defer m.clientMu.RUnlock()
	return m.client
}

// connect establishes a connection, authenticates, and subscribes to events.
func (m *Manager) connect() error {
	log.Println("Connecting to Home Assistant...")

	conn, _, err := websocket.DefaultDialer.Dial(m.url, nil)
	if err != nil {
		return err
	}

	client := &Client{
		Conn:         conn,
		Token:        m.token,
		MessageID:    1,
		pending:      make(map[int]chan<- Message),
		eventChannel: m.eventChannel, // Use the manager's long-lived channel
	}

	// Authenticate
	if err := client.Authenticate(); err != nil {
		conn.Close()
		return err
	}

	// Subscribe to events
	id := client.NextMessageID()
	req := map[string]interface{}{
		"id":   id,
		"type": "subscribe_events",
	}

	if err := client.Conn.WriteJSON(req); err != nil {
		conn.Close()
		return err
	}
	log.Printf("Sent subscribe_events request with ID: %d", id)

	// Read the subscription acknowledgement inline before the main listen loop starts.
	var result Message
	if err := client.Conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		conn.Close()
		return err
	}
	if err := client.Conn.ReadJSON(&result); err != nil {
		conn.Close()
		return &subscriptionError{message: err.Error()}
	}
	if err := client.Conn.SetReadDeadline(time.Time{}); err != nil {
		conn.Close()
		return err
	}
	if result.Type != "result" || result.ID != id {
		conn.Close()
		return &subscriptionError{message: "unexpected subscription response"}
	}
	if !result.Success {
		errMsg := "unknown error"
		if result.Error != nil {
			errMsg = result.Error.Message
		}
		conn.Close()
		return &subscriptionError{message: errMsg}
	}
	log.Println("Successfully subscribed to Home Assistant events")

	m.clientMu.Lock()
	m.client = client
	m.clientMu.Unlock()

	m.setConnected(true)
	log.Println("Connected to Home Assistant successfully")
	return nil
}

type subscriptionError struct {
	message string
}

func (e *subscriptionError) Error() string {
	return "subscription failed: " + e.message
}

// calculateBackoff returns the backoff duration for a given attempt.
func (m *Manager) calculateBackoff(attempt int) time.Duration {
	backoff := float64(m.initialBackoff) * math.Pow(2, float64(attempt))
	if backoff > float64(m.maxBackoff) {
		backoff = float64(m.maxBackoff)
	}
	return time.Duration(backoff)
}

// Run starts the connection manager and runs until the context is cancelled.
// It handles initial connection, listening, and reconnection with exponential backoff.
func (m *Manager) Run(ctx context.Context) {
	defer close(m.eventChannel)

	attempt := 0

	for {
		select {
		case <-ctx.Done():
			log.Println("Home Assistant manager shutting down...")
			m.closeCurrentConnection()
			return
		default:
		}

		client := m.GetClient()
		if !m.IsConnected() || client == nil {
			err := m.connect()
			if err != nil {
				m.setConnected(false)
				attempt++

				if m.maxRetries > 0 && attempt > m.maxRetries {
					log.Printf("Home Assistant: max retries (%d) exceeded, giving up", m.maxRetries)
					return
				}

				backoff := m.calculateBackoff(attempt - 1)
				log.Printf("Home Assistant connection failed: %v. Retrying in %v (attempt %d)", err, backoff, attempt)

				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
					continue
				}
			}

			// Reset attempt counter on successful connection
			attempt = 0
		}

		// Run the listen loop - this blocks until error or context cancellation
		m.listen(ctx)

		// If we get here, connection was lost
		m.setConnected(false)
		m.closeCurrentConnection()

		select {
		case <-ctx.Done():
			return
		default:
			log.Println("Home Assistant connection lost, will attempt to reconnect...")
			// Small delay before reconnecting
			select {
			case <-ctx.Done():
				return
			case <-time.After(m.initialBackoff):
			}
		}
	}
}

// listen processes messages from the WebSocket connection.
func (m *Manager) listen(ctx context.Context) {
	client := m.GetClient()
	if client == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			var msg Message
			err := client.Conn.ReadJSON(&msg)
			if err != nil {
				log.Printf("Home Assistant WebSocket error: %v", err)
				return
			}

			client.mutex.Lock()
			if ch, ok := client.pending[msg.ID]; ok {
				ch <- msg
				delete(client.pending, msg.ID)
			} else if msg.Type == "event" && msg.Event != nil {
				select {
				case m.eventChannel <- *msg.Event:
				case <-ctx.Done():
					client.mutex.Unlock()
					return
				default:
					// Channel full, log and drop event to prevent blocking
					log.Println("Warning: Event channel full, dropping event")
				}
			}
			client.mutex.Unlock()
		}
	}
}

// closeCurrentConnection safely closes the current connection.
func (m *Manager) closeCurrentConnection() {
	m.clientMu.Lock()
	defer m.clientMu.Unlock()

	if m.client != nil && m.client.Conn != nil {
		m.client.Conn.Close()
	}
	m.client = nil
}

// TryConnectOnce attempts to connect once without retrying.
// Returns true if connection was successful.
func (m *Manager) TryConnectOnce() bool {
	err := m.connect()
	if err != nil {
		log.Printf("Home Assistant initial connection failed: %v", err)
		return false
	}
	return true
}
