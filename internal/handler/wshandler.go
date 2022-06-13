package handler

import (
	"log"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{} // use default options

// serverWS receives a GraphQL query as an HTTP request, executes the
// query (or mutation) and generates an HTTP response or error message
func (h *Handler) serveWS(w http.ResponseWriter, r *http.Request) {
	var err error
	if r.Method != http.MethodGet {
		http.Error(w, "GraphQL subscriptions must use GET method", http.StatusMethodNotAllowed)
		return
	}
	// Check for websocket hijacking
	if len(r.Header["Origin"]) > 0 {
		if url, err := url.Parse(r.Header.Get("Origin")); err == nil {
			if url.Host != r.Host {
				http.Error(w, "Invalid origin for websocket", http.StatusForbidden)
			}
		}
	}
	h.conn, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer h.conn.Close()
}

// wsMessage is used to encode in JSON messages sent to/from the websocket for the graphql-transport-ws sub-protocol
type wsMessage struct {
	Type    string `json:"type"`
	ID      string `json:"id,omitempty"`
	Payload struct {
		// Used for request (subscribe)
		OperationName string                 `json:"operationName,omitempty"`
		Query         string                 `json:"query,omitempty"`
		Variables     map[string]interface{} `json:"variables,omitempty"`
		// Used for replies (next)
		Data interface{} `json:"data,omitempty"`
	} `json:"payload,omitempty"`
}
