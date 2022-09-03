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

	"github.com/gorilla/websocket"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/validator"
)

type (
	wsConnection struct {
		*websocket.Conn // handle for WS communications

		h *Handler // we need this for the schema etc

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
// It only handles subscription request(s) and sends a stream of responses.
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
	defer func() {
		c.stopAll()
		err := c.Close()
		if err != nil {
			log.Println("wsConnection close error:", err)
		}
	}()
	// TODO call conn.SetReadDeadline() ??

	if !c.init() {
		return
	}

	if !c.newProtocol {
		_ = c.WriteMessage(1, []byte("ka"))
	}

	for {
		message := c.read()
		if message == nil {
			_ = c.WriteMessage(1, []byte("complete")) // TODO check this is correct message format
			return
		}

		switch message.Type {
		case "subscribe", "start":
			c.start(r.Context(), message)

		case "complete", "stop":
			c.stop(message.ID)

		case "ping":
			_ = c.WriteMessage(1, []byte("pong")) // TODO check this is correct message format

		case "connection_terminate":
			return

		default:
			log.Println("wsConnection unexpected message type:", message.Type)
			return
		}
	}
}

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
			go func(value reflect.Value) {
				messageType := "next"
				if !c.newProtocol {
					messageType = "data"
				}

				resultChans := op.GetSelectionChannels(ctx, operation.SelectionSet, value, nil)
				// assert(cap(resultChans) == 1)
				for _, ch := range resultChans {
				inner:
					for {
						select {
						case v, ok := <-ch:
							if !ok {
								break inner
							}
							if v.err != nil {
								// TODO: send v.err
								return
							}
							out := wsMessage{Type: messageType, ID: message.ID}
							out.Payload.Data = v.value
							log.Println("qqq", v.value)
							if err := c.WriteJSON(out); err != nil {
								log.Println("wsConnection write error:", err)
								return
							}
						case <-ctx.Done():
							return
						}
					}
				}
			}(reflect.ValueOf(d))
		}
	}
}

// stop kills processing of one operation (eg subscription) by calling the cancel function of the operation's context
func (c wsConnection) stop(ID string) {
	if c.cancelSubscription[ID] == nil {
		log.Println("wsConnection ID already cancelled:", ID)
		return
	}
	c.cancelSubscription[ID]()     // call context cancel func to stop the subscription
	c.cancelSubscription[ID] = nil // remember that it's been cancelled
}

// stopAll kills processing of all operations before closing the websocket
func (c wsConnection) stopAll() {
	for ID, cancel := range c.cancelSubscription {
		if cancel != nil {
			cancel()
			c.cancelSubscription[ID] = nil
		}
	}
}

func (c wsConnection) init() bool {
	c.newProtocol = c.Subprotocol() == "graphql-transport-ws"

	// Get connection_init and send connection_ack or error
	message := c.read()
	if message == nil || message.Type != "connection_init" {
		log.Println("wsConnection init error")
		_ = c.WriteMessage(1, []byte("connection_error"))
		return false
	}
	_ = c.WriteMessage(1, []byte("connection_ack"))
	return true
}

func (c wsConnection) read() *wsMessage {
	_, reader, err := c.NextReader()
	if err != nil {
		log.Println("wsConnection read error")
		return nil
	}

	var message wsMessage
	decoder := json.NewDecoder(reader)
	decoder.UseNumber() // allows us to distinguish ints from floats in Variables map (see also FixNumberVariables())
	err = decoder.Decode(&message)
	if err != nil {
		log.Println("wsConnection decode error")
		return nil
	}

	return &message
}
