package codexrpc

import "encoding/json"

// JSON-RPC 2.0 messages without the {"jsonrpc":"2.0"} field (omitted on the wire).

type Request struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

type Notification struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func idKey(raw json.RawMessage) string {
	// raw is JSON scalar; keep exact bytes to round-trip.
	return string(raw)
}

