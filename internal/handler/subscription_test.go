package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andrewwphillips/eggql/internal/handler"
	"github.com/gorilla/websocket"
)

type wsActionType int

const (
	actionSend   wsActionType = iota // send WS message: data == message as JSON (string)
	actionRecv                       // receive WS message: data == relevant part of the expected message (string)
	actionError                      // WS close message: data == close code (int) or error message (string)
	actionCancel                     // cancel the context
	actionPause                      // sleep for a short time: data == milliseconds (int)
)

type (
	wsAction struct {
		action wsActionType
		data   interface{}
	}
)

// TestSubscriptions has a table of tests each of which tests a subscriptions scenario
func TestSubscriptions(t *testing.T) {
	subscriptionData := map[string]struct {
		delay                                      time.Duration // time between "hello" messages from the server (0 for no delay)
		protocol                                   string        // which WS subprotocol to use == "graphql-transport-ws" (new) or "graphql-ws" (old)
		initialTimeout, pingFrequency, pongTimeout time.Duration
		actions                                    []wsAction // list of actions to take
	}{
		"empty": {actions: []wsAction{}},
		"basic_old": {
			delay: time.Second,
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{actionRecv, `"ka"`},
				{actionSend, `{"type":"start","id":"ID-1","payload":{"query":"subscription {message}"}}`},
				{actionRecv, `{"type":"data","id":"ID-1","payload":{"data":{"message":"hello"}}}`},
				{actionSend, `{"type":"stop","id":"ID-1"}`},
				{actionRecv, `"type":"complete","id":"ID-1"`},
			},
		},
		"init_bad": {
			actions: []wsAction{
				{actionSend, `bad`},
				//{actionRecv, `"connection_error"`}, // TODO check this
				{actionError, websocket.CloseUnsupportedData},
				{actionPause, 10}, // this is needed to detect possible residual websocket close problems
			},
		},
		"init_term": {
			// can send connection_terminate instead of connection_init
			actions: []wsAction{
				{actionSend, `{"type": "connection_terminate"}`},
				{actionError, websocket.CloseNormalClosure},
			},
		},
		"start_term": {
			// send connection_terminate at very start (after handshake finished)
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{actionRecv, `"ka"`},
				{actionSend, `{"type": "connection_terminate"}`},
				{actionError, websocket.CloseNormalClosure},
				{actionPause, 20},
			},
		},
		"init_start": {
			actions: []wsAction{
				{actionSend, `{"type": "start"}`},
				{actionRecv, `"connection_error"`},
			},
		},
		"2nd_ka": {
			pingFrequency: 5 * time.Millisecond,
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{actionRecv, `"ka"`},
				{actionRecv, `"ka"`},
				{actionPause, 20},
			},
		},
		"dupe_ID": {
			delay: 500 * time.Millisecond,
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{actionRecv, `"ka"`},
				{actionSend, `{"type":"start","id":"x","payload":{"query":"subscription {message}"}}`},
				{actionRecv, `{"type":"data","id":"x","payload":{"data":{"message":"hello"}}}`},
				{actionSend, `{"type":"start","id":"x","payload":{"query":"xxx"}}`},
				{actionError, 4409}, // Subscriber for x already exists
				{actionPause, 20},
			},
		},
		// Using new sub-protocol -----------------
		"basic_new": {
			delay: time.Second, protocol: "graphql-transport-ws",
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{actionSend, `{"type":"subscribe","id":"ID-3","payload":{"query":"subscription {message}"}}`},
				{actionRecv, `{"type":"next","id":"ID-3","payload":{"data":{"message":"hello"}}}`},
				{actionSend, `{"type":"complete","id":"ID-3"}`},
				{actionRecv, `"type":"complete","id":"ID-3"`},
			},
		},
		"start_not_subscribe": {
			delay: time.Second, protocol: "graphql-transport-ws",
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{actionSend, `{"type":"start","id":"ID-4","payload":{"query":"subscription {message}"}}`},
				{actionError, 4400}, // unexpected message type
			},
		},
		"double_init": {
			protocol: "graphql-transport-ws",
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{actionSend, `{"type": "connection_init"}`},
				{actionError, 4429}, // too many init requests
				{actionPause, 20},
			},
		},
		"no_payload": {
			protocol: "graphql-transport-ws",
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{actionSend, `{"type":"subscribe","id":"ID-5"}`},
				{actionError, websocket.CloseInvalidFramePayloadData},
				{actionPause, 20},
			},
		},
		"bad_query": {
			protocol: "graphql-transport-ws",
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{actionSend, `{"type":"subscribe","id":"ID-6","payload":{"query":"bad"}}`},
				{actionRecv, `"error"`}, // Unexpected name "bad"
				{actionError, websocket.CloseAbnormalClosure},
				{actionPause, 20},
			},
		},
		"bad_vars": {
			protocol: "graphql-transport-ws",
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{
					actionSend,
					`{"type":"subscribe","id":"ID-8","payload":{"query":"subscription {message}", "variables":"bad"}}`,
				},
				{actionError, 4400}, // JSON error unmarshall struct
			},
		},
		"dupe_id_new": {
			delay:    500 * time.Millisecond,
			protocol: "graphql-transport-ws",
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{actionSend, `{"type":"subscribe","id":"dupe","payload":{"query":"subscription {message}"}}`},
				{actionRecv, `{"type":"next","id":"dupe","payload":{"data":{"message":"hello"}}}`},
				{actionSend, `{"type":"subscribe","id":"dupe","payload":{"query":"subscription {message}"}}`},
				{actionError, 4409}, // Subscriber for dupe already exists
				{actionPause, 20},
			},
		},
		"send_ping": {
			protocol: "graphql-transport-ws",
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{actionSend, `{"type":"ping"}`},
				{actionRecv, `"type":"pong"`},
			},
		},
		"reply_pong": {
			protocol:      "graphql-transport-ws",
			pingFrequency: 5 * time.Millisecond,
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{actionRecv, `"type":"ping"`},
				{actionSend, `{"type":"pong"}`},
			},
		},
		"no_pong": {
			pingFrequency: 100 * time.Millisecond, // bigger than pongTimeout to ensure we get the error before 2nd ping
			pongTimeout:   2 * time.Millisecond,
			protocol:      "graphql-transport-ws",
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{actionRecv, `"type":"ping"`},
				{actionError, "websocket: bad close code 1006"},
			},
		},
	}

	for name, data := range subscriptionData {
		n, d := name, data // retain loop value for capture in the closure below
		t.Run(n, func(t *testing.T) {
			t.Parallel()
			server := getServer(d.delay, d.initialTimeout, d.pingFrequency, d.pongTimeout)
			defer server.Close()

			header := make(http.Header)
			if d.protocol != "" {
				header.Add("Sec-WebSocket-Protocol", d.protocol)
			}
			conn, resp, err := websocket.DefaultDialer.Dial(strings.Replace(server.URL, "http://", "ws://", -1), header)
			if err != nil {
				Assertf(t, err == nil, "%12s: Expected no Dial error, got %v", n, err)
			}
			err = resp.Body.Close()
			if err != nil {
				Assertf(t, err == nil, "%12s: Expected no body close error, got %v", n, err)
			}

			for i, a := range d.actions {
				switch a.action {
				case actionSend:
					err2 := conn.WriteMessage(websocket.TextMessage, []byte(a.data.(string)))
					Assertf(t, err2 == nil, "%12s: write (%d) expected no error, got %v", n, i, err2)
				case actionRecv:
					messageType, p, err2 := conn.ReadMessage()
					if messageType != websocket.TextMessage {
						Assertf(t, messageType == websocket.TextMessage, "%12s: read (%d) expected to read text message type got %d", n, i, messageType)
					}
					toFind := a.data.(string)
					Assertf(t, strings.Contains(string(p), toFind), "%12s: read (%d) expected message containing <%s>, got <%s>", n, i, toFind, string(p))
					Assertf(t, err2 == nil, "%12s: read (%d) expected no error, got %v", n, i, err2)
				case actionError:
					_, _, err2 := conn.ReadMessage() // expecting an error so ignore the message
					if err2 == nil {
						Assertf(t, false, "%12s: read (%d) expected an error, got nil", n, i)
					} else if got, ok := err2.(*websocket.CloseError); ok {
						// websocket error - check the close code is correct
						Assertf(t, got.Code == a.data.(int), "%12s: read (%d) expected close code %d, got %d (error %v)", n, i, a.data.(int), got.Code, err2)
					} else {
						// close code not expected by Gorilla package - just check the message text
						Assertf(t, err2.Error() == a.data.(string), "%12s: read (%d) expected error %q, got %q", n, i, a.data.(string), err2.Error())
					}
				case actionCancel:
					// TODO
				case actionPause:
					t.Logf("      %12s: pause (%d) for %d milliseconds", n, i, a.data.(int))
					time.Sleep(time.Duration(a.data.(int)) * time.Millisecond)
				}
			}
		})
	}
}

// getServer creates a simples GraphQL server that keeps sending "hello" messages for a "message" subscription
func getServer(delay, initialTimeout, pingFrequency, pongTimeout time.Duration) *httptest.Server {
	// Create handler that has a single subscription that keeps sending "hello"
	h := handler.New(
		[]string{"type Subscription{ message: String! }"},
		nil,
		[3][]interface{}{
			nil, nil, {
				struct {
					Message func(context.Context) <-chan string
				}{
					func(ctx context.Context) <-chan string {
						ch := make(chan string)
						go func() {
							for {
								select {
								case <-ctx.Done():
									close(ch)
									return
								case ch <- "hello":
									if delay > 0 {
										time.Sleep(delay)
									}
								}
							}
						}()
						return ch
					},
				},
			},
		},
		handler.InitialTimeout(initialTimeout),
		handler.PingFrequency(pingFrequency),
		handler.PongTimeout(pongTimeout),
	)

	return httptest.NewServer(h)
}
