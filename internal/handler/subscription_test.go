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
		"init_bad": {
			actions: []wsAction{
				{actionSend, `bad`},
				//{actionRecv, `"connection_error"`}, // TODO check this
				{actionError, websocket.CloseUnsupportedData},
				{actionPause, 20}, // this is needed to detect possible residual websocket close problems
			},
		},
		"init_term": {
			actions: []wsAction{
				{actionSend, `{"type": "connection_terminate"}`},
				{actionError, websocket.CloseNormalClosure},
			},
		},
		"init_start": {
			actions: []wsAction{
				{actionSend, `{"type": "start"}`},
				{actionRecv, `"connection_error"`},
			},
		},
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
		"2nd_ka": {
			pingFrequency: 5 * time.Millisecond,
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{actionRecv, `"ka"`},
				{actionRecv, `"ka"`},
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
		"send_ping": {
			protocol: "graphql-transport-ws",
			actions: []wsAction{
				{actionSend, `{"type": "connection_init"}`},
				{actionRecv, `"connection_ack"`},
				{actionSend, `{"type":"ping"}`},
				{actionRecv, `"type":"pong"`},
				{actionPause, 20}, // this is needed to detect residual websocket close problems
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
					_, _, err2 := conn.ReadMessage()
					if err2 == nil {
						Assertf(t, false, "%12s: read (%d) an error, got nil", n, i)
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
