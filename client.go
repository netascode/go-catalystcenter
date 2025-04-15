// Package cc is a Cisco Catalyst Center REST client library for Go.
package cc

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const DefaultMaxRetries int = 3
const DefaultBackoffMinDelay int = 2
const DefaultBackoffMaxDelay int = 60
const DefaultBackoffDelayFactor float64 = 3
const DefaultDefaultMaxAsyncWaitTime int = 30

var SynchronousApiEndpoints = [...]string{
	"/dna/intent/api/v1/site",
	"/dna/intent/api/v1/global-pool",
}

// Client is an HTTP Catalyst Center client.
// Always use NewClient to construct it, otherwise requests will panic.
type Client struct {
	// HttpClient is the *http.Client used for API requests.
	HttpClient *http.Client
	// Url is the Catalyst Center IP or hostname, e.g. https://10.0.0.1:443 (port is optional).
	Url string
	// Token is the current authentication token
	Token string
	// Usr is the Catalyst Center username.
	Usr string
	// Pwd is the Catalyst Center password.
	Pwd string
	// Maximum number of retries
	MaxRetries int
	// Minimum delay between two retries
	BackoffMinDelay int
	// Maximum delay between two retries
	BackoffMaxDelay int
	// Backoff delay factor
	BackoffDelayFactor float64
	// Maximum async operations wait time
	DefaultMaxAsyncWaitTime int
	// Authentication mutex ensures that API login is non-concurrent
	AuthenticationMutex *sync.Mutex
	readers             chan int
	writers             chan int
	// writingMutex protects against concurrent DELETE/POST/PUT requests towards the API.
	writingMutex *sync.Mutex
}

// NewClient creates a new Catalyst Center HTTP client.
// Pass modifiers in to modify the behavior of the client, e.g.
//
//	client, _ := NewClient("https://cc1.cisco.com", "user", "password", RequestTimeout(120))
func NewClient(url, usr, pwd string, mods ...func(*Client)) (Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	cookieJar, _ := cookiejar.New(nil)
	httpClient := http.Client{
		Timeout:   60 * time.Second,
		Transport: tr,
		Jar:       cookieJar,
	}

	client := Client{
		HttpClient:              &httpClient,
		Url:                     url,
		Usr:                     usr,
		Pwd:                     pwd,
		MaxRetries:              DefaultMaxRetries,
		BackoffMinDelay:         DefaultBackoffMinDelay,
		BackoffMaxDelay:         DefaultBackoffMaxDelay,
		BackoffDelayFactor:      DefaultBackoffDelayFactor,
		DefaultMaxAsyncWaitTime: DefaultDefaultMaxAsyncWaitTime,
		AuthenticationMutex:     &sync.Mutex{},
		readers:                 make(chan int),
		writers:                 make(chan int),
		writingMutex:            &sync.Mutex{},
	}

	go func() {
		var readerNum, writerNum int

		for {
			select {
			case delta := <-client.readers:
				readerNum += delta
				for readerNum != 0 {
					readerNum += <-client.readers
				}

			case delta := <-client.writers:
				writerNum += delta
				for writerNum != 0 {
					writerNum += <-client.writers
				}
			}
		}
	}()

	for _, mod := range mods {
		mod(&client)
	}
	return client, nil
}

// Insecure determines if insecure https connections are allowed. Default value is true.
func Insecure(x bool) func(*Client) {
	return func(client *Client) {
		client.HttpClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = x
	}
}

// RequestTimeout modifies the HTTP request timeout from the default of 60 seconds.
func RequestTimeout(x time.Duration) func(*Client) {
	return func(client *Client) {
		client.HttpClient.Timeout = x * time.Second
	}
}

// MaxRetries modifies the maximum number of retries from the default of 3.
func MaxRetries(x int) func(*Client) {
	return func(client *Client) {
		client.MaxRetries = x
	}
}

// BackoffMinDelay modifies the minimum delay between two retries from the default of 2.
func BackoffMinDelay(x int) func(*Client) {
	return func(client *Client) {
		client.BackoffMinDelay = x
	}
}

// BackoffMaxDelay modifies the maximum delay between two retries from the default of 60.
func BackoffMaxDelay(x int) func(*Client) {
	return func(client *Client) {
		client.BackoffMaxDelay = x
	}
}

// BackoffDelayFactor modifies the backoff delay factor from the default of 3.
func BackoffDelayFactor(x float64) func(*Client) {
	return func(client *Client) {
		client.BackoffDelayFactor = x
	}
}

// DefaultMaxAsyncWaitTime modifies the maximum wait time for async operations from the default of 30 seconds.
func DefaultMaxAsyncWaitTime(x int) func(*Client) {
	return func(client *Client) {
		client.DefaultMaxAsyncWaitTime = x
	}
}

// NewReq creates a new Req request for this client.
func (client Client) NewReq(method, uri string, body io.Reader, mods ...func(*Req)) Req {
	httpReq, _ := http.NewRequest(method, client.Url+uri, body)
	req := Req{
		HttpReq:          httpReq,
		LogPayload:       true,
		Synchronous:      true,
		MaxAsyncWaitTime: client.DefaultMaxAsyncWaitTime,
		NoWait:           false,
	}
	for _, mod := range mods {
		mod(&req)
	}
	if req.Synchronous && contains(SynchronousApiEndpoints[:], uri) && contains([]string{"POST", "PUT", "DELETE"}, strings.ToUpper(method)) {
		req.HttpReq.Header.Add("__runsync", "true")
	}
	return req
}

// Do makes a request.
// Requests for Do are built ouside of the client, e.g.
//
//	req := client.NewReq("GET", "/dna/intent/api/v2/site", nil)
//	res, _ := client.Do(req)
func (client *Client) Do(req Req) (Res, error) {
	// add token
	req.HttpReq.Header.Add("X-Auth-Token", client.Token)
	req.HttpReq.Header.Add("Content-Type", "application/json")
	// retain the request body across multiple attempts
	var body []byte
	if req.HttpReq.Body != nil {
		body, _ = io.ReadAll(req.HttpReq.Body)
	}

	var res Res

	defer func() {
		log.Printf("[DEBUG] Exit from Do method: %s, %s", req.HttpReq.Method, req.HttpReq.URL)
	}()

	if req.HttpReq.Method == "DELETE" || req.HttpReq.Method == "POST" || req.HttpReq.Method == "PUT" {
		if req.UseMutex {
			client.writingMutex.Lock()
			defer client.writingMutex.Unlock()
		}

		client.writers <- +1
		defer func() { client.writers <- -1 }()
	}

	for attempts := 0; ; attempts++ {
		req.HttpReq.Body = io.NopCloser(bytes.NewBuffer(body))
		if req.LogPayload {
			log.Printf("[DEBUG] HTTP Request: %s, %s, %s", req.HttpReq.Method, req.HttpReq.URL, req.HttpReq.Body)
		} else {
			log.Printf("[DEBUG] HTTP Request: %s, %s", req.HttpReq.Method, req.HttpReq.URL)
		}

		httpRes, err := client.HttpClient.Do(req.HttpReq)
		if err != nil {
			if ok := client.Backoff(attempts); !ok {
				log.Printf("[ERROR] HTTP Connection error occured: %+v", err)
				return Res{}, err
			} else {
				log.Printf("[ERROR] HTTP Connection failed: %s, retries: %v", err, attempts)
				continue
			}
		}

		defer httpRes.Body.Close()
		bodyBytes, err := io.ReadAll(httpRes.Body)
		if err != nil {
			if ok := client.Backoff(attempts); !ok {
				log.Printf("[ERROR] Cannot decode response body: %+v", err)
				return Res{}, err
			} else {
				log.Printf("[ERROR] Cannot decode response body: %s, retries: %v", err, attempts)
				continue
			}
		}
		res = Res(gjson.ParseBytes(bodyBytes))
		if req.LogPayload {
			log.Printf("[DEBUG] HTTP Response: %s", res.Raw)
		}

		if httpRes.StatusCode >= 200 && httpRes.StatusCode <= 299 {
			break
		} else {
			if ok := client.Backoff(attempts); !ok {
				log.Printf("[ERROR] HTTP Request failed: StatusCode %v", httpRes.StatusCode)
				return res, fmt.Errorf("HTTP Request failed: StatusCode %v", httpRes.StatusCode)
			} else if httpRes.StatusCode == 429 {
				retryAfter := httpRes.Header.Get("Retry-After")
				retryAfterDuration := time.Duration(0)
				if retryAfter == "0" {
					retryAfterDuration = time.Second
				} else if retryAfter != "" {
					retryAfterDuration, _ = time.ParseDuration(retryAfter + "s")
				} else {
					retryAfterDuration = 15 * time.Second
				}
				log.Printf("[WARNING] HTTP Request rate limited, waiting %v seconds, Retries: %v", retryAfterDuration.Seconds(), attempts)
				time.Sleep(retryAfterDuration)
				continue
			} else if httpRes.StatusCode == 408 || (httpRes.StatusCode >= 502 && httpRes.StatusCode <= 504) {
				log.Printf("[ERROR] HTTP Request failed: StatusCode %v, Retries: %v", httpRes.StatusCode, attempts)
				continue
			} else {
				log.Printf("[ERROR] HTTP Request failed: StatusCode %v", httpRes.StatusCode)
				return res, fmt.Errorf("HTTP Request failed: StatusCode %v", httpRes.StatusCode)
			}
		}
	}

	if !req.NoWait && req.Synchronous && req.HttpReq.Method != "GET" && req.HttpReq.Method != "" {
		return client.WaitTask(&req, &res)
	}

	return res, nil
}

// WaitTask waits for an asynchronous task to complete.
func (client *Client) WaitTask(req *Req, res *Res) (Res, error) {
	var asyncOp, id string
	if res.Get("response.taskId").Exists() {
		asyncOp = "task"
		id = res.Get("response.taskId").String()
	} else if res.Get("executionId").Exists() {
		asyncOp = "execution"
		id = res.Get("executionId").String()
	}
	if asyncOp != "" {
		startTime := time.Now()
		for attempts := 0; ; attempts++ {
			sleep := 0.5 * float64(attempts)
			if sleep > 2 {
				sleep = 2
			}
			time.Sleep(time.Duration(sleep * float64(time.Second)))
			var taskReq *http.Request
			if asyncOp == "task" {
				taskReq, _ = http.NewRequest("GET", client.Url+"/api/v1/task/"+id, nil)
			} else {
				taskReq, _ = http.NewRequest("GET", client.Url+"/dna/platform/management/business-api/v1/execution-status/"+id, nil)
			}
			taskReq.Header.Add("X-Auth-Token", client.Token)
			httpTaskRes, err := client.HttpClient.Do(taskReq)
			if err != nil {
				return Res{}, err
			}
			defer httpTaskRes.Body.Close()
			taskBodyBytes, err := io.ReadAll(httpTaskRes.Body)
			if err != nil {
				log.Printf("[ERROR] Cannot decode response body: %+v", err)
				log.Printf("[DEBUG] Exit from Do method")
				return Res{}, err
			}
			taskRes := Res(gjson.ParseBytes(taskBodyBytes))
			log.Printf("[DEBUG] task response %v", taskRes.String())
			if taskRes.Get("response.isError").Bool() {
				log.Printf("[ERROR] Task '%s' failed: %s, %s", id, taskRes.Get("response.progress").String(), taskRes.Get("response.failureReason").String())
				log.Printf("[DEBUG] Exit from Do method")
				return taskRes, fmt.Errorf("task '%s' failed: %s, %s", id, taskRes.Get("response.progress").String(), taskRes.Get("response.failureReason").String())
			}
			if !taskRes.Get("response.isError").Bool() && taskRes.Get("response.endTime").Exists() {
				log.Printf("[DEBUG] Exit from Do method")
				return taskRes, nil
			}
			if taskRes.Get("status").String() == "FAILURE" {
				log.Printf("[ERROR] Task '%s' failed: %s", id, taskRes.Get("bapiError").String())
				log.Printf("[DEBUG] Exit from Do method")
				return taskRes, fmt.Errorf("task '%s' failed: %s", id, taskRes.Get("bapiError").String())
			}
			if taskRes.Get("status").String() == "SUCCESS" {
				log.Printf("[DEBUG] Exit from Do method")
				return taskRes, nil
			}
			log.Printf("[DEBUG] Waiting for task '%s' to complete.", id)
			if time.Since(startTime) > time.Duration(req.MaxAsyncWaitTime)*time.Second {
				log.Printf("[DEBUG] Maximum waiting time reached for task '%s'.", id)
				return taskRes, fmt.Errorf("maximum waiting time for task '%s' reached", id)
			}
		}
	}
	return *res, nil
}

var maxItems = 500

// Get makes a GET request and returns a gjson result.
// Before GET is issued, the func ensures that no writing (DELETE/POST/PUT) would run concurrently with it
// on the entire client (on any path).
//
// Results will be the raw data structure as returned by Catalyst Center, except when it contains an array named
// "response" with 500 items. In that case the func continues with more GET requests until it can return a concatenation
// of all the retrieved items from all the pages.
//
// With multiple GETs, the concurrency protection is uninterrupted from the first page until the last page.
// Protection from concurrent POST or DELETE helps against boundary items being shifted between the pages.
// Protection from concurrent PUT helps against items moving between pages, when sort becomes unstable due to
// modification of items.
// Unfortunately, the protection does not cover any requests from other clients/processes/systems.
func (client *Client) Get(path string, mods ...func(*Req)) (Res, error) {
	// This channel operation will wait for any writers to complete first.
	// Improvement idea: optimistic GET without any waiting. But then if it returns 500 items,
	// throw its result away, wait for lock, restart with pagination?
	client.readers <- +1
	defer func() { client.readers <- -1 }()

	offset := 1
	gather := gatherer{}
	gather.WriteByte('[')

	for {
		raw, err := client.get(pathWithOffset(path, offset), mods...)
		if err != nil {
			return raw, err
		}

		response := raw.Get("response")
		if !response.Exists() {
			return raw, err
		}

		if !response.IsArray() {
			if offset != 1 {
				return gjson.Parse("null"), fmt.Errorf("expected `response` to be an array, but got: %s", response.Type)
			}
			return raw, err
		}

		items := response.Array()

		if len(items) != maxItems {
			if offset == 1 {
				return raw, err
			} else {
				gather.Grow(len(raw.Raw) - 1) // hot path optimization
				gather.GatherJSON(items, ',')
				gather.WriteByte(']')

				s, err := sjson.SetRawBytes([]byte(raw.Raw), "response", gather.Bytes())
				if err != nil {
					return gjson.Parse("null"), err
				}

				log.Printf("[DEBUG] All GET pages combined: %s", s)

				return gjson.ParseBytes(s), nil
			}
		}

		gather.GatherJSON(items, ',')
		offset += len(items)
	}
}

// get is like Get but without pagination.
func (client *Client) get(path string, mods ...func(*Req)) (Res, error) {
	req := client.NewReq("GET", path, nil, mods...)
	err := client.Authenticate()
	if err != nil {
		return Res{}, err
	}

	return client.Do(req)
}

func pathWithOffset(path string, offset int) string {
	if offset <= 1 {
		return path
	}

	sep := "?"
	if strings.Contains(path, sep) {
		sep = "&"
	}

	return fmt.Sprintf("%s%soffset=%d", path, sep, offset)
}

type gatherer struct {
	bytes.Buffer
}

// GatherJSON appends additional JSON bytes. It puts a separator character between items.
func (g *gatherer) GatherJSON(items []gjson.Result, separator byte) {
	for _, item := range items {
		if len(g.Bytes()) > 1 {
			_ = g.WriteByte(separator)
		}
		_, _ = g.Write([]byte(item.Raw))
	}
}

// Delete makes a DELETE request.
func (client *Client) Delete(path string, mods ...func(*Req)) (Res, error) {
	req := client.NewReq("DELETE", path, nil, mods...)
	err := client.Authenticate()
	if err != nil {
		return Res{}, err
	}
	return client.Do(req)
}

// Post makes a POST request and returns a GJSON result.
// Hint: Use the Body struct to easily create POST body data.
func (client *Client) Post(path, data string, mods ...func(*Req)) (Res, error) {
	req := client.NewReq("POST", path, strings.NewReader(data), mods...)
	err := client.Authenticate()
	if err != nil {
		return Res{}, err
	}
	return client.Do(req)
}

// Put makes a PUT request and returns a GJSON result.
// Hint: Use the Body struct to easily create PUT body data.
func (client *Client) Put(path, data string, mods ...func(*Req)) (Res, error) {
	req := client.NewReq("PUT", path, strings.NewReader(data), mods...)
	err := client.Authenticate()
	if err != nil {
		return Res{}, err
	}
	return client.Do(req)
}

// Login authenticates to the Catalyst Center device.
func (client *Client) Login() error {
	req := client.NewReq("POST", "/dna/system/api/v1/auth/token", strings.NewReader(""), NoLogPayload)
	req.HttpReq.SetBasicAuth(client.Usr, client.Pwd)
	httpRes, err := client.HttpClient.Do(req.HttpReq)
	if err != nil {
		return err
	}
	if httpRes.StatusCode != 200 {
		log.Printf("[ERROR] Authentication failed: StatusCode %v", httpRes.StatusCode)
		return errors.New("authentication failed")
	}
	defer httpRes.Body.Close()
	body, _ := io.ReadAll(httpRes.Body)
	token := gjson.GetBytes(body, "Token").String()
	if string(token) == "" {
		log.Print("[ERROR] Token retrieval failed: no token in payload")
		return errors.New("authentication failed")
	}
	client.Token = string(token)
	log.Printf("[DEBUG] Authentication successful")
	return nil
}

// Login if no token available.
func (client *Client) Authenticate() error {
	var err error
	client.AuthenticationMutex.Lock()
	if client.Token == "" {
		err = client.Login()
	}
	client.AuthenticationMutex.Unlock()
	return err
}

// Backoff waits following an exponential backoff algorithm
func (client *Client) Backoff(attempts int) bool {
	log.Printf("[DEBUG] Begining backoff method: attempts %v on %v", attempts, client.MaxRetries)
	if attempts >= client.MaxRetries {
		log.Printf("[DEBUG] Exit from backoff method with return value false")
		return false
	}

	minDelay := time.Duration(client.BackoffMinDelay) * time.Second
	maxDelay := time.Duration(client.BackoffMaxDelay) * time.Second

	min := float64(minDelay)
	backoff := min * math.Pow(client.BackoffDelayFactor, float64(attempts))
	if backoff > float64(maxDelay) {
		backoff = float64(maxDelay)
	}
	backoff = (rand.Float64()/2+0.5)*(backoff-min) + min
	backoffDuration := time.Duration(backoff)
	log.Printf("[TRACE] Starting sleeping for %v", backoffDuration.Round(time.Second))
	time.Sleep(backoffDuration)
	log.Printf("[DEBUG] Exit from backoff method with return value true")
	return true
}
