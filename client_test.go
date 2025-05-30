package cc

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

const (
	testURL                  = "https://10.0.0.1"
	testURLWithTrailingSlash = "https://10.0.0.1/"
)

var testURLs = []string{
	testURL,
	testURLWithTrailingSlash,
}

func testClient() Client {
	client, _ := NewClient(testURL, "usr", "pwd", MaxRetries(0))
	gock.InterceptClient(client.HttpClient)
	return client
}

func authenticatedTestClient() Client {
	client := testClient()
	client.Token = "ABC"
	return client
}

// ErrReader implements the io.Reader interface and fails on Read.
type ErrReader struct{}

// Read mocks failing io.Reader test cases.
func (r ErrReader) Read(buf []byte) (int, error) {
	return 0, errors.New("fail")
}

// TestNewClient tests the NewClient function.
func TestNewClient(t *testing.T) {
	for _, testURL := range testURLs {
		t.Run(testURL, func(t *testing.T) {
			client, _ := NewClient(testURL, "usr", "pwd", RequestTimeout(120))
			assert.Equal(t, client.HttpClient.Timeout, 120*time.Second)
			// Verify the URL is sanitized correctly (trailing slash removed)
			expectedURL := strings.TrimSuffix(testURL, "/")
			assert.Equal(t, expectedURL, client.Url)
		})
	}
}

// TestClientLogin tests the Client.Login method.
func TestClientLogin(t *testing.T) {
	defer gock.Off()
	client := testClient()

	// Successful login
	gock.New(testURL).Post("/dna/system/api/v1/auth/token").Reply(200).BodyString(`{"Token": "ABC"}`)
	assert.NoError(t, client.Login())

	// Unsuccessful token retrieval
	gock.New(testURL).Post("/dna/system/api/v1/auth/token").Reply(200).BodyString(``)
	assert.Error(t, client.Login())

	// Invalid HTTP status code
	gock.New(testURL).Post("/dna/system/api/v1/auth/token").Reply(405)
	assert.Error(t, client.Login())
}

// TestClientGet tests the Client.Get method.
func TestClientGet(t *testing.T) {
	defer gock.Off()
	client := authenticatedTestClient()
	var err error

	// Success
	gock.New(testURL).Get("/url").Reply(200).BodyString(`{"response":"a string"}`)
	res, err := client.Get("/url")
	assert.NoError(t, err)
	assert.Equal(t, "a string", res.Get("response").String())

	// HTTP error
	gock.New(testURL).Get("/url").ReplyError(errors.New("fail"))
	_, err = client.Get("/url")
	assert.Error(t, err)

	// Invalid HTTP status code
	gock.New(testURL).Get("/url").Reply(405)
	_, err = client.Get("/url")
	assert.Error(t, err)

	// Error decoding response body
	gock.New(testURL).
		Get("/url").
		Reply(200).
		Map(func(res *http.Response) *http.Response {
			res.Body = io.NopCloser(ErrReader{})
			return res
		})
	_, err = client.Get("/url")
	assert.Error(t, err)
}

// TestClientDelete tests the Client.Delete method.
func TestClientDelete(t *testing.T) {
	defer gock.Off()
	client := authenticatedTestClient()

	// Success
	gock.New(testURL).
		Delete("/url").
		Reply(200)
	_, err := client.Delete("/url")
	assert.NoError(t, err)

	// HTTP error
	gock.New(testURL).
		Delete("/url").
		ReplyError(errors.New("fail"))
	_, err = client.Delete("/url")
	assert.Error(t, err)
}

// TestClientPost tests the Client.Post method.
func TestClientPost(t *testing.T) {
	defer gock.Off()
	client := authenticatedTestClient()

	var err error

	// Success
	gock.New(testURL).Post("/url").Reply(200)
	_, err = client.Post("/url", "{}")
	assert.NoError(t, err)

	// HTTP error
	gock.New(testURL).Post("/url").ReplyError(errors.New("fail"))
	_, err = client.Post("/url", "{}")
	assert.Error(t, err)

	// Invalid HTTP status code
	gock.New(testURL).Post("/url").Reply(405)
	_, err = client.Post("/url", "{}")
	assert.Error(t, err)

	// Error decoding response body
	gock.New(testURL).
		Post("/url").
		Reply(200).
		Map(func(res *http.Response) *http.Response {
			res.Body = io.NopCloser(ErrReader{})
			return res
		})
	_, err = client.Post("/url", "{}")
	assert.Error(t, err)
}

// TestClientPut tests the Client.Put method.
func TestClientPut(t *testing.T) {
	defer gock.Off()
	client := authenticatedTestClient()

	var err error

	// Success
	gock.New(testURL).Put("/url").Reply(200)
	_, err = client.Put("/url", "{}")
	assert.NoError(t, err)

	// HTTP error
	gock.New(testURL).Put("/url").ReplyError(errors.New("fail"))
	_, err = client.Put("/url", "{}")
	assert.Error(t, err)

	// Invalid HTTP status code
	gock.New(testURL).Put("/url").Reply(405)
	_, err = client.Put("/url", "{}")
	assert.Error(t, err)

	// Error decoding response body
	gock.New(testURL).
		Put("/url").
		Reply(200).
		Map(func(res *http.Response) *http.Response {
			res.Body = io.NopCloser(ErrReader{})
			return res
		})
	_, err = client.Put("/url", "{}")
	assert.Error(t, err)
}

// TestClientWaitTask tests the Client.WaitTask method.
func TestClientWaitTask(t *testing.T) {
	defer gock.Off()
	client := authenticatedTestClient()

	var err error

	// Task
	gock.New(testURL).Get("/api/v1/task/123").Reply(200).BodyString(`{"response": {"endTime": "1", "isError": false}}`)
	_, err = client.WaitTask(&Req{}, &Res{Raw: `{"response": {"taskId": "123"}}`})
	assert.NoError(t, err)

	// Task error
	gock.New(testURL).Get("/api/v1/task/123").Reply(200).BodyString(`{"response": {"endTime": "1", "isError": true}}`)
	_, err = client.WaitTask(&Req{}, &Res{Raw: `{"response": {"taskId": "123"}}`})
	assert.Error(t, err)

	// Execution
	gock.New(testURL).Get("/dna/platform/management/business-api/v1/execution-status/123").Reply(200).BodyString(`{"status": "SUCCESS"}`)
	_, err = client.WaitTask(&Req{}, &Res{Raw: `{"executionId": "123"}`})
	assert.NoError(t, err)

	// Execution error
	gock.New(testURL).Get("/dna/platform/management/business-api/v1/execution-status/123").Reply(200).BodyString(`{"status": "FAILURE"}`)
	_, err = client.WaitTask(&Req{}, &Res{Raw: `{"executionId": "123"}`})
	assert.Error(t, err)
}
