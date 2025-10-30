package cc

import (
	"net/http"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Body wraps SJSON for building JSON body strings.
// Usage example:
//
//	Body{}.Set("name", "ABC").Str
type Body struct {
	Str string
}

// Set sets a JSON path to a value.
func (body Body) Set(path, value string) Body {
	res, _ := sjson.Set(body.Str, path, value)
	body.Str = res
	return body
}

// SetRaw sets a JSON path to a raw string value.
// This is primarily used for building up nested structures, e.g.:
//
//	Body{}.SetRaw("children", Body{}.Set("name", "New").Str).Str
func (body Body) SetRaw(path, rawValue string) Body {
	res, _ := sjson.SetRaw(body.Str, path, rawValue)
	body.Str = res
	return body
}

// Delete deletes a JSON path.
func (body Body) Delete(path string) Body {
	res, _ := sjson.Delete(body.Str, path)
	body.Str = res
	return body
}

// Res creates a Res object, i.e. a GJSON result object.
func (body Body) Res() Res {
	return gjson.Parse(body.Str)
}

// Req wraps http.Request for API requests.
type Req struct {
	// HttpReq is the *http.Request obejct.
	HttpReq *http.Request
	// LogPayload indicates whether logging of payloads should be enabled.
	LogPayload bool
	// Synchronous indicates whether the request should be performed synchronously.
	Synchronous bool
	// MaxAsyncWaitTime is the maximum time to wait for an asynchronous operation.
	MaxAsyncWaitTime int
	// NoWait indicates whether to wait for the task or not. If True, the WaitTask function will not be executed.
	NoWait bool
	// UseMutex indicates whether to use the writingMutex for this request
	UseMutex bool
	// UseMutex indicates whether request already tried to reauthenticate in case of 401
	ReAuthAttempted bool
}

// NoLogPayload prevents logging of payloads.
// Primarily used by the Login and Refresh methods where this could expose secrets.
func NoLogPayload(req *Req) {
	req.LogPayload = false
}

// Asynchronous operation.
// This is only relevant for POST, PUT or DELETE requests.
func Asynchronous(req *Req) {
	req.Synchronous = false
}

// Maximum Asynchronous operation wait time.
// This is only relevant for POST, PUT or DELETE requests.
func MaxAsyncWaitTime(seconds int) func(*Req) {
	return func(req *Req) {
		req.MaxAsyncWaitTime = seconds
	}
}

// NoWait operation
func NoWait(req *Req) {
	req.NoWait = true
}

// UseMutex enables the use of the writingMutex for this request.
func UseMutex(req *Req) {
	req.UseMutex = true
}
