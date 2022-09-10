package handler

// wshandler is code for handling websockets for subscriptions.  It supports both commonly used WS protocols
// * subscriptions-transport-ws: early protocol from Apollo for subscriptions (sub-protocol name:graphql-ws)
// * graphql-ws is newer (official?) ws transport which can handle query/mutation/subscription (sub-protocol name:graphql-transport-ws).

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"reflect"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/validator"
)

const (
	initTimeout   = 2 * time.Second
	pingFrequency = 10 * time.Second
	pongTimeout   = 2 * time.Second
)

type (
	wsConnection struct {
		h               *Handler // we need this for the schema etc
		*websocket.Conn          // handle for WS communications

		// cancelSubscription keeps track of the cancel function(s) associated with each operation.
		// Typically, there is just one entry in the map which is the ID associated with the subscription operation.
		//  map key = ID that identifies the operation
		//  map value = context.CancelFunc that will terminate the operation (ie kill all subscription processing)
		cancelSubscription map[string]context.CancelFunc

		newProtocol bool // default to old
	}

	wsMessage struct {
		Type    string `json:"type"`
		ID      string `json:"id,omitempty"`
		Payload struct {
			// Used for request (subscribe)
			OperationName string                 `json:"operationName,omitempty"`
			Query         string                 `json:"query,omitempty"` // required for request
			Variables     map[string]interface{} `json:"variables,omitempty"`
			Extensions    map[string]interface{} `json:"extensions,omitempty"`
			// Used for replies (next)
			Data interface{} `json:"data,omitempty"`
		} `json:"payload,omitempty"`
	}
)

var upgrader = websocket.Upgrader{
	//ReadBufferSize:    4096,
	//WriteBufferSize:   4096,
	//EnableCompression: true,
	CheckOrigin:  func(r *http.Request) bool { return true },
	Subprotocols: []string{"graphql-ws", "graphql-transport-ws"},
}

// serverWS is called in response to a GraphQL HTTP request wanting to upgrade to a WS.
func (h *Handler) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("wsConnection upgrade error:", err)
		// nothing else required here as w's HTTP status has already been set
		return
	}
	c := wsConnection{
		Conn:               conn,
		h:                  h,
		cancelSubscription: make(map[string]context.CancelFunc, 1),
	}

	if !c.init() {
		c.Close()
		return
	}

	if !c.newProtocol {
		if err := c.WriteJSON(wsMessage{Type: "ka"}); err != nil {
			log.Println("wsConnection: ka error:", err)
		}
	}

	// timer ensures the connection is kept alive by sending a "ka" or "ping" message even if there is no other traffic
	var timer *time.Timer

	// Create a channel that returns the messages read from the web socket
	ch := make(chan *wsMessage)
	go func() {
		message := c.read()
		if message == nil {
			close(ch)
			return
		}
		ch <- message
	}()

	defer func() {
		c.stopAll()
		err := c.Close()
		if err != nil {
			log.Println("wsConnection close error:", err)
		}
		for range ch {
			// nothing needed here - just draining ch
		}
	}()

loop:
	for {
		timer = time.NewTimer(pingFrequency)
		select {
		case message, ok := <-ch:
			if !ok {
				// TODO: check if we send "complete" in this case (read error)
				if err := c.WriteJSON(wsMessage{Type: "complete"}); err != nil {
					log.Println("wsConnection complete error:", err)
				}
				return
			}

			switch message.Type {
			case "subscribe", "start":
				c.start(r.Context(), message)

			case "complete", "stop":
				c.stop(message.ID)

			case "ping":
				if err := c.WriteJSON(wsMessage{Type: "pong"}); err != nil {
					log.Println("wsConnection pong error:", err)
					return
				}

			case "pong":
				// sent in response to our ping but nothing is required here

			case "connection_terminate":
				return // TODO: drain ch?

			default:
				log.Println("wsConnection unexpected message type:", message.Type)
				return
			}

		case <-r.Context().Done():
			break loop // TODO: drain ch?

		case <-timer.C:
			if c.newProtocol {
				// Send a "ping" expecting a reply ("pong" or other message) within a certain time
				c.setTimeout(pongTimeout)
				if err := c.WriteJSON(wsMessage{Type: "ping"}); err != nil {
					log.Println("wsConnection: ping error:", err)
				}
			} else {
				// Old protocol just has the server send a "keep alive" message
				if err := c.WriteJSON(wsMessage{Type: "ka"}); err != nil {
					log.Println("wsConnection: ka error:", err)
				}
			}

		}
		_ = timer.Stop() // don't try to drain the chan - no need and it could be empty
	}
}

// setTimeout sets the maximum allowed time before a response is expected. If no response is forthcoming then the websocket
// enters an error state whence the WS can no longer be used. For example, this may be used to check that a connection is
// alive by sending a "ping" and expecting a "pong" reply within the timeout period.  (Of course, a different message
// type may be received before the "pong" but that's OK as long as it is received before the timeout ends.)
func (c wsConnection) setTimeout(timeout time.Duration) {
	if err := c.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		log.Println("wsConnection set deadline error:", err)
	}
}

// start extracts subscription(s) from a JSON message and begins the processing of them
func (c wsConnection) start(ctx context.Context, message *wsMessage) {
	// Add to our map of operations active in this ws (first checking that the ID is not in use)
	if _, ok := c.cancelSubscription[message.ID]; ok {
		log.Println("wsConnection duplicate ID:", message.ID)
		// TODO: send 4409: Subscriber for <id> already exists
		return
	}
	ctx, c.cancelSubscription[message.ID] = context.WithCancel(ctx)

	FixNumberVariables(message.Payload.Variables)

	query, errors := gqlparser.LoadQuery(c.h.schema, message.Payload.Query)
	if errors != nil {
		// TODO
		return
	}

	for _, operation := range query.Operations {
		op := gqlOperation{enums: c.h.enums, enumsReverse: c.h.enumsReverse}

		if len(operation.VariableDefinitions) > 0 {
			var pgqlError *gqlerror.Error
			if op.variables, pgqlError = validator.VariableValues(c.h.schema, operation, message.Payload.Variables); pgqlError != nil {
				// TODO
				continue // skip this op if we can't get the vars
			}
		}

		var data []interface{}
		switch operation.Operation {
		case ast.Query:
			// TODO
		case ast.Mutation:
			op.isMutation = true
			// TODO
		case ast.Subscription:
			op.isSubscription = true
			data = c.h.subscriptionData
		default:
			// TODO
		}

		for _, d := range data {
			result, err := op.GetSelections(ctx, operation.SelectionSet, reflect.ValueOf(d), nil)
			if err != nil {
				// TODO
				break
			}
			if len(result.Order) > 0 {
				// start processing for each subscription
				for _, k := range result.Order {
					go c.process(ctx, message.ID, k, result.Data[k])
				}
				break // don't look for the same selection(s) in the next data
			}
		}
	}
}

func (c wsConnection) process(ctx context.Context, ID string, k string, i interface{}) {
	messageType := "next"
	if !c.newProtocol {
		messageType = "data"
	}

	ch := getConvertedChannel(i)
	for ok := true; ok; {
		var v interface{} // value returned from the channel
		select {
		case v, ok = <-ch:
			if !ok {
				if err := c.WriteJSON(wsMessage{Type: "complete", ID: ID}); err != nil {
					log.Println("wsConnection complete error:", err)
				}
				return
			}
			out := wsMessage{Type: messageType, ID: ID}
			out.Payload.Data = map[string]interface{}{k: v}
			if err := c.WriteJSON(out); err != nil {
				log.Println("wsConnection write error:", err)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func getConvertedChannel(i interface{}) <-chan interface{} {
	v := reflect.ValueOf(i)
	ch := make(chan interface{})
	go func() {
		for {
			x, ok := v.Recv()
			if !ok {
				close(ch)
				return
			}
			ch <- x.Interface()
		}
	}()
	return ch
}

// stop kills processing of one operation (eg subscription) by calling the cancel function of the operation's context
func (c wsConnection) stop(ID string) {
	if c.cancelSubscription[ID] == nil {
		log.Println("wsConnection ID not found or already cancelled:", ID)
		return
	}
	c.cancelSubscription[ID]()     // call context cancel func to stop the subscription
	c.cancelSubscription[ID] = nil // remember that it's been cancelled
}

// stopAll kills processing of all operations (eg before closing the websocket)
func (c wsConnection) stopAll() {
	for ID, cancel := range c.cancelSubscription {
		if cancel != nil {
			cancel()
			c.cancelSubscription[ID] = nil
		}
	}
}

// init handles the initial (high level) handshake by receiving an "init" message and sending an "ack"
func (c wsConnection) init() bool {
	c.newProtocol = c.Subprotocol() == "graphql-transport-ws" // else assume it's the "old" (graphql-ws) WS sub-protocol

	// Get connection_init and send connection_ack or error
	c.setTimeout(initTimeout)
	message := c.read()
	if message == nil || message.Type != "connection_init" {
		log.Println("wsConnection init error")
		_ = c.WriteMessage(1, []byte("connection_error"))
		return false
	}
	if err := c.WriteMessage(1, []byte("connection_ack")); err != nil {
		log.Println("wsConnection error sending ack:", err)
		_ = c.WriteMessage(1, []byte("connection_error"))
		return false
	}
	return true
}

func (c wsConnection) read() *wsMessage {
	_, reader, err := c.NextReader()
	if err != nil {
		log.Println("wsConnection read error:", err)
		return nil
	}
	// clear deadline - as we may not care how long before the next read arrives
	if err := c.SetReadDeadline(time.Time{}); err != nil {
		log.Println("wsConnection clear deadline error:", err)
	}

	r := &wsMessage{}
	decoder := json.NewDecoder(reader)
	decoder.UseNumber() // allows us to distinguish ints from floats in Variables map (see also FixNumberVariables())
	err = decoder.Decode(r)
	if err != nil {
		log.Println("wsConnection decode error:", err)
		return nil
	}

	return r
}
