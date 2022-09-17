package handler

// wshandler is code for handling websockets for subscriptions.  It supports both commonly used WS protocols
// * subscriptions-transport-ws: early protocol from Apollo for subscriptions (sub-protocol name:graphql-ws)
// * graphql-ws is newer (official?) ws transport which can handle query/mutation/subscription (sub-protocol name:graphql-transport-ws).

// Note that I decided to not have separate "transport" objects (for graphql-ws and graphql-transport-ws sub-protocols) but instead
// handle the differences using a flag.  (The flag is called "newProtocol" and is set if the new sub-protocol is detected.)  The
// protocols are very similar and a few "if !c.newProtocol" tests will not slow things down noticeably. It also avoids a lot of
// duplicate/similar code.

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/validator"
)

const (
	initTimeout   = 2 * time.Second  // how long to wait for connection_init after the WS is opened
	pingFrequency = 10 * time.Second // how often to send a ping (ka in old protocol) message to the client
	pongTimeout   = 2 * time.Second  // how long to wait for a pong after sending a ping
)

type (
	wsConnection struct {
		h *Handler // we need this for the schema etc

		// writeMu is required to protect writes to the WS (*webscoket.Conn) which may come from different go-routines
		writeMu         *sync.Mutex // protect concurrent writes to the websocket
		*websocket.Conn             // handle for WS communications

		// cancelSubscription keeps track of the cancel function(s) associated with each operation.
		// In theory, a client can open multiple subscriptions (and queries/mutations) on a single WS, differentiated
		// by the ID field of most messages. Typically, I think there is just one subscription per WS, whence this
		// map has just one entry. We need to keep track of the cancel functions so that the subscription channel
		// can be closed when we receive a "complete" message ("stop" in old protocol) or the webscoket is closed.
		//  map key = ID that identifies the operation (subscription)
		//  map value = context.CancelFunc that will terminate the operation (ie kill all subscription processing)
		cancelSubscription map[string]context.CancelFunc

		// newProtocol is set to true if we are using the new WS sub-protocol (graphql-transport-ws)
		newProtocol bool // defaults to old protocol

	}

	// wsMessage is used to encode (or decode) the messages sent to (received from) the websocket as JSON
	wsMessage struct {
		Type    string   `json:"type"`
		ID      string   `json:"id,omitempty"`
		Payload *payload `json:"payload,omitempty"`
	}
	// payload is used to encode the variable part (payload) of messages sent to and from the websocket
	payload struct {
		// Used for decoding the request (subscribe/start message)
		OperationName string                 `json:"operationName,omitempty"`
		Query         string                 `json:"query,omitempty"` // required for request
		Variables     map[string]interface{} `json:"variables,omitempty"`
		Extensions    map[string]interface{} `json:"extensions,omitempty"`
		// Used for encoding replies (next/data message)
		Data interface{} `json:"data,omitempty"`
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
		h:                  h,
		writeMu:            &sync.Mutex{},
		Conn:               conn,
		cancelSubscription: make(map[string]context.CancelFunc, 1),
		newProtocol:        conn.Subprotocol() == "graphql-transport-ws", // assume it's "old" (graphql-ws) sub-protocol unless explicitly set to new
	}

	if !c.init() {
		c.Close()
		return
	}

	if !c.newProtocol {
		c.write(wsMessage{Type: "ka"})
	}

	// timer ensures the connection is kept alive by sending a "ka" or "ping" message even if there is no other traffic
	var timer *time.Timer

	// Create a channel that returns the messages read from the web socket
	ch := make(chan *wsMessage)
	go func() {
		for {
			var message *wsMessage
			if !c.newProtocol {
				message = c.read("start", "stop", "connection_terminate")
			} else {
				message = c.read("ping", "pong", "subscribe", "complete", "connection_init")
			}
			if message == nil {
				close(ch)
				return
			}
			ch <- message
		}
	}()

	defer func() {
		c.stopAll()
		err := c.Close()
		if err != nil {
			log.Println("wsConnection close error:", err)
		}
		for qqq := range ch {
			// nothing needed here - just draining ch
			log.Println("QQQ", qqq)
		}
	}()

	doneCh := r.Context().Done()
loop:
	for {
		timer = time.NewTimer(pingFrequency)
		select {
		case <-doneCh:
			break loop

		case message, ok := <-ch:
			if !ok {
				return
			}

			switch message.Type {
			case "connection_init":
				c.closeMessage(4429, "Too many initialisation requests")
				return

			case "connection_terminate":
				return

			case "subscribe", "start":
				if !c.start(r.Context(), message) {
					return
				}

			case "complete", "stop":
				c.stop(message.ID)

			case "ping":
				c.write(wsMessage{Type: "pong"})

			case "pong":
				// received in response to our ping (see write of ping in <-timer.C case below) - this code was suggested at:
				// https://stackoverflow.com/questions/37696527/go-gorilla-websockets-on-ping-pong-fail-user-disconnct-call-function
				c.setTimeout(0)

			default:
				panic("Unexpected WS message type")
			}

		case <-timer.C:
			if !c.newProtocol {
				// Old protocol just has the server send a "keep alive" message
				c.write(wsMessage{Type: "ka"})
			} else {
				// Send a "ping" expecting a reply ("pong") within a certain time
				c.setTimeout(pongTimeout)
				c.write(wsMessage{Type: "ping"})
			}

		}
		_ = timer.Stop() // don't drain the timer.C chan - no need and probably empty
	}
}

// init performs the high-level (sub-protocol) handshake by receiving an "init" message and sending an "ack"
func (c wsConnection) init() bool {
	// Get connection_init and send connection_ack or error
	c.setTimeout(initTimeout)
	var message *wsMessage
	if !c.newProtocol {
		message = c.read("connection_init", "connection_terminate", "start")
	} else {
		message = c.read("connection_init", "subscribe")
	}
	if message == nil {
		// At this point an error/ close message has been sent in c.read
		return false
	}
	if message.Type == "subscribe" {
		// New protocol: ERROR - subscribe received before connection_init
		c.closeMessage(4409, "Unauthorized")
		return false
	}
	if message.Type == "start" {
		// Old protocol: ERROR - start received before connection_init
		c.write(wsMessage{Type: "connection_error"})
		c.closeMessage(websocket.CloseProtocolError, "start received before connection_init")
		return false
	}
	if message.Type == "connection_terminate" {
		// Old protocol: OK - client is allowed tor terminate immediately
		c.closeMessage(websocket.CloseNormalClosure, "")
		return false
	}
	// at this point we're OK to continue (got a "connection_init")
	c.setTimeout(0) // clear timeout since we got the response before the deadline
	c.write(wsMessage{Type: "connection_ack"})
	return true
}

// start extract subscription from WS message payload (Query field) and starts its processing
// It returns false on error
//  - if the operation ID in the subscribe/start message is already in use
//  - if the query is invalid
func (c wsConnection) start(ctx context.Context, message *wsMessage) bool {
	if message.ID == "" {
		c.closeMessage(websocket.CloseProtocolError, "no ID provided for subscriber")
	}
	// Add to our map of operations active in this ws (first checking that the ID is not in use)
	if _, ok := c.cancelSubscription[message.ID]; ok {
		c.closeMessage(4409, "Subscriber for "+message.ID+" already exists")
		return false
	}
	ctx, c.cancelSubscription[message.ID] = context.WithCancel(ctx)

	FixNumberVariables(message.Payload.Variables)

	query, errors := gqlparser.LoadQuery(c.h.schema, message.Payload.Query)
	if errors != nil {
		out := wsMessage{
			Type: "error", ID: message.ID,
			Payload: &payload{
				Data: errors,
			},
		}
		c.write(out)
		return false
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
	return true
}

// process is called as a go routine to send the operation data to the websocket
// Parameters
// ctx = context that can be used to terminate the processing
// ID = client identifier for the operation from the "subscribe" (or start in old sub-protocol) message
// k = name or alias of the subscription query
// in = channel which outputs the data for the subscription
func (c wsConnection) process(ctx context.Context, ID string, k string, in interface{}) {
	messageType := "next"
	if !c.newProtocol {
		messageType = "data"
	}

	defer func() {
		ch := reflect.ValueOf(in)
		for {
			if _, ok := ch.Recv(); !ok {
				break
			}
		}
	}()

	for {
		// We use reflect.Select instead of a select statement because we don't know the type returned by the 'in' chan
		chosen, v, ok := reflect.Select([]reflect.SelectCase{
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(in)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())},
		})
		switch chosen {
		case 0:
			if !ok {
				c.write(wsMessage{Type: "complete", ID: ID})
				return
			}
			out := wsMessage{
				Type: messageType, ID: ID,
				Payload: &payload{
					Data: map[string]interface{}{k: v.Interface()},
				},
			}
			c.write(out) // TODO check if write fails?
		case 1:
			c.write(wsMessage{Type: "complete", ID: ID})
			// drain the channel in case it was written to just before the ctx cancel was received
			for ok = true; ok; {
				var qqq reflect.Value
				_, qqq, ok = reflect.Select([]reflect.SelectCase{
					{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(in)},
				})
				log.Println(qqq.Interface())
			}
			return
		}
	}
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

// write wraps the Gorilla WriteJSON method to allow concurrent writes
func (c wsConnection) write(v interface{}) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.WriteJSON(v); err != nil {
		log.Println("QQQ", v)
		log.Println("wsConnection: write error:", err)
	}
}

// closeMessage writes a WS close control message (presumably just before closing the websocket)
func (c wsConnection) closeMessage(closeCode int, text string) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(closeCode, text)); err != nil {
		log.Println("wsConnection: writeMessage (close) error:", err)
	}
}

// read gets a message from the websocket, decodes the JSON, and returns a pointer to the message
// If there is any sort of error it sends an appropriate response on the websocket and returns nil
// Note that concurrent reads are not needed so there is no mutex for WS reads.
// Parameter(s): all message types that are expected TODO: use a map[string] instead of a []string for faster lookup
func (c wsConnection) read(expected ...string) *wsMessage {
	// Get the message from the websocket
	messageType, reader, err := c.NextReader()
	if err != nil {
		// if we are dealing with initialisation then respond as per doc
		if len(expected) > 0 && expected[0] == "connection_init" {
			if !c.newProtocol {
				c.write(wsMessage{Type: "connection_error"})
				return nil
			}
			// TODO: we should only send this for a timeout
			c.closeMessage(4408, "Connection initialisation timeout")
		}
		c.closeMessage(websocket.CloseAbnormalClosure, "read error:"+err.Error())
		return nil
	}

	if messageType != websocket.TextMessage {
		c.closeMessage(websocket.CloseUnsupportedData, "Expected text message but got: "+strconv.Itoa(messageType))
		return nil
	}

	// Decode the websocket text - must be JSON
	r := &wsMessage{}
	decoder := json.NewDecoder(reader)
	decoder.UseNumber() // allows us to distinguish ints from floats in Variables map (see also FixNumberVariables())
	err = decoder.Decode(r)
	if err != nil {
		if !c.newProtocol {
			c.closeMessage(websocket.CloseUnsupportedData, "JSON error:"+err.Error())
		} else {
			c.closeMessage(4400, "JSON error:"+err.Error())
		}
		return nil
	}

	if len(expected) > 0 {
		// Check for expected message types
		found := false
		for _, e := range expected {
			if r.Type == e {
				found = true
				break
			}
		}
		if !found {
			if !c.newProtocol {
				c.closeMessage(websocket.CloseProtocolError, "unexpected message type:"+r.Type)
			} else {
				c.closeMessage(4400, "unexpected message type:"+r.Type)
			}
			return nil
		}
	}
	return r
}

// setTimeout sets the maximum allowed time before a response is expected, use a duration of zero for no timeout
// After setting a timeout, if no response is forthcoming NextReader() times out (returns an error) and the websocket
// enters an error state whence the WS can no longer be used. For example, this may be used to check that a connection is
// alive by sending a "ping" and expecting a "pong" reply within the timeout period.  (Of course, a different message
// type may be received before the "pong" but that's OK as long as it is received before the timeout ends.)
func (c wsConnection) setTimeout(timeout time.Duration) {
	if timeout == 0 {
		_ = c.SetReadDeadline(time.Time{})
		return
	}
	_ = c.SetReadDeadline(time.Now().Add(timeout))
}
