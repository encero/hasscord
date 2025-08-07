package hass

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client represents the Home Assistant WebSocket client.
type Client struct {
	Conn         *websocket.Conn
	Token        string
	MessageID    int
	mutex        sync.Mutex
	pending      map[int]chan<- Message
	eventChannel chan Event
}

// Message represents a message to/from Home Assistant.
type Message struct {
	ID      int             `json:"id,omitempty"`
	Type    string          `json:"type"`
	Success bool            `json:"success,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Event *Event `json:"event,omitempty"`
}

// Event represents a Home Assistant event.
type Event struct {
	EventType string          `json:"event_type"`
	Data      json.RawMessage `json:"data"`
	Origin    string          `json:"origin"`
	TimeFired string          `json:"time_fired"`
}

// StateChangedData represents the data for a state_changed event.
type StateChangedData struct {
	EntityID string `json:"entity_id"`
	NewState State  `json:"new_state"`
	OldState State  `json:"old_state"`
}

// State represents the state of an entity.
type State struct {
	EntityID    string                 `json:"entity_id"`
	State       string                 `json:"state"`
	Attributes  map[string]interface{} `json:"attributes"`
	LastChanged string                 `json:"last_changed"`
	LastUpdated string                 `json:"last_updated"`
	Context     struct {
		ID       string `json:"id"`
		ParentID string `json:"parent_id"`
		UserID   string `json:"user_id"`
	} `json:"context"`
}

// New creates a new Home Assistant client.
func New(url, token string) (*Client, error) {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}

	return &Client{
		Conn:         conn,
		Token:        token,
		MessageID:    1,
		pending:      make(map[int]chan<- Message),
		eventChannel: make(chan Event),
	}, nil
}

func (c *Client) NextMessageID() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	id := c.MessageID
	c.MessageID++
	return id
}

func (c *Client) RegisterPending(id int, ch chan<- Message) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.pending[id] = ch
}

// Authenticate authenticates the client with Home Assistant.
func (c *Client) Authenticate() error {
	// Expect "auth_required"
	var msg Message
	log.Printf("Waiting for auth_required...")
	err := c.Conn.ReadJSON(&msg)
	if err != nil {
		return fmt.Errorf("error reading auth_required: %w", err)
	}
	log.Printf("Received message type: %s", msg.Type)
	if msg.Type != "auth_required" {
		return fmt.Errorf("unexpected message type: %s", msg.Type)
	}

	// Send auth message
	authMsg := map[string]string{
		"type":         "auth",
		"access_token": c.Token,
	}
	err = c.Conn.WriteJSON(authMsg)
	if err != nil {
		return err
	}

	// Expect "auth_ok" or "auth_invalid"
	err = c.Conn.ReadJSON(&msg)
	if err != nil {
		return fmt.Errorf("error reading auth response: %w", err)
	}
	log.Printf("Received auth response type: %s", msg.Type)
	if msg.Type == "auth_invalid" {
		return fmt.Errorf("authentication failed: %s", msg.Error.Message)
	}
	if msg.Type != "auth_ok" {
		return fmt.Errorf("unexpected message type after auth: %s", msg.Type)
	}

	log.Printf("Authentication successful.")

	return nil
}

// SubscribeToEvents subscribes to all events.
func (c *Client) SubscribeToEvents() (<-chan Event, error) {
	c.mutex.Lock()
	id := c.MessageID
	c.MessageID++
	c.mutex.Unlock()

	req := map[string]interface{}{
		"id":   id,
		"type": "subscribe_events",
	}

	resultChan := make(chan Message, 1)
	c.pending[id] = resultChan

	err := c.Conn.WriteJSON(req)
	if err != nil {
		return nil, err
	}
	log.Printf("Sent subscribe_events request with ID: %d", id)

	select {
	case result := <-resultChan:
		log.Printf("Received subscribe_events response: %+v", result)
		if !result.Success {
			return nil, fmt.Errorf("failed to subscribe to events: %s", result.Error.Message)
		}
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout waiting for subscription result")
	}

	return c.eventChannel, nil
}

// Listen starts listening for messages from Home Assistant.
func (c *Client) Listen() {
	for {
		var msg Message
		err := c.Conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("Error reading from WebSocket: %v", err)
			close(c.eventChannel)
			return
		}

		c.mutex.Lock()
		if ch, ok := c.pending[msg.ID]; ok {
			ch <- msg
			delete(c.pending, msg.ID)
		} else if msg.Type == "event" {
			c.eventChannel <- *msg.Event
		}
		c.mutex.Unlock()
	}
}
