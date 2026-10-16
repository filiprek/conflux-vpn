package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
)

const maxMessage = 64 * 1024

var requestID atomic.Uint64

// App is an unprivileged Wails boundary. Privileged work belongs to the service.
type App struct{}
type request struct {
	Version int            `json:"version"`
	ID      string         `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}
type response struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   string          `json:"error,omitempty"`
}

func (a *App) Invoke(method string, params map[string]any) (json.RawMessage, error) {
	switch method {
	case "status", "configure", "connect", "disconnect", "recover", "testPath", "diagnostics", "importRelay", "importPath", "deletePath", "updatePath", "testRelay":
	default:
		return nil, errors.New("unsupported service command")
	}
	if params == nil {
		params = map[string]any{}
	}
	id := fmt.Sprint(requestID.Add(1))
	body, err := json.Marshal(request{Version: 1, ID: id, Method: method, Params: params})
	if err != nil {
		return nil, errors.New("invalid service parameters")
	}
	if len(body)+1 > maxMessage {
		return nil, errors.New("service request too large")
	}
	return callService(append(body, '\n'), id)
}
func decodeResponse(reader io.Reader, id string) (json.RawMessage, error) {
	frame, err := bufio.NewReaderSize(io.LimitReader(reader, maxMessage+1), maxMessage+1).ReadBytes('\n')
	if err != nil || len(frame) > maxMessage {
		return nil, errors.New("invalid service response")
	}
	var response response
	if err = json.Unmarshal(frame, &response); err != nil {
		return nil, errors.New("invalid service response")
	}
	if response.Version != 1 || response.ID != id {
		return nil, errors.New("service protocol mismatch")
	}
	if response.Error != "" {
		return nil, errors.New(publicError(response.Error))
	}
	if len(response.Result) == 0 {
		return json.RawMessage(`null`), nil
	}
	return response.Result, nil
}

// publicError forwards the service's short operator-facing reason so the user can act
// on it. Anything that could carry a link, credential or file path is replaced.
func publicError(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	if message == "" || len(message) > 300 || strings.ContainsAny(message, `@\`) || strings.Contains(message, "://") {
		return "service operation failed"
	}
	return message
}
