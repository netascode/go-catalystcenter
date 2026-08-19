package cc

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

// captureUserAgent installs a gock observer that records the User-Agent header
// of every outgoing request keyed by the request path, so a test can assert the
// header reached a specific request site.
func captureUserAgent(seen map[string]string) {
	gock.Observe(func(req *http.Request, _ gock.Mock) {
		if req != nil && req.URL != nil {
			seen[req.URL.Path] = req.Header.Get("User-Agent")
		}
	})
}

// TestUserAgentLogin asserts the configured User-Agent reaches the auth/token
// (login) request, which builds via NewReq but is sent directly via HttpClient.Do.
func TestUserAgentLogin(t *testing.T) {
	defer gock.Off()
	defer gock.Observe(nil)

	client, _ := NewClient(testURL, "usr", "pwd", MaxRetries(0), UserAgent("CatalystCenterTerraform/9.9.9 Cisco"))
	gock.InterceptClient(client.HttpClient)

	seen := map[string]string{}
	captureUserAgent(seen)

	gock.New(testURL).Post("/dna/system/api/v1/auth/token").Reply(200).BodyString(`{"Token": "ABC"}`)
	assert.NoError(t, client.Login())

	assert.Equal(t, "CatalystCenterTerraform/9.9.9 Cisco", seen["/dna/system/api/v1/auth/token"])
}

// TestUserAgentAPI asserts the configured User-Agent reaches a regular API
// request, which builds via NewReq and is sent via Do.
func TestUserAgentAPI(t *testing.T) {
	defer gock.Off()
	defer gock.Observe(nil)

	client, _ := NewClient(testURL, "usr", "pwd", MaxRetries(0), UserAgent("CatalystCenterTerraform/9.9.9 Cisco"))
	gock.InterceptClient(client.HttpClient)
	client.Token = "ABC"

	seen := map[string]string{}
	captureUserAgent(seen)

	gock.New(testURL).Get("/url").Reply(200).BodyString(`{"response":"a string"}`)
	_, err := client.Get("/url")
	assert.NoError(t, err)

	assert.Equal(t, "CatalystCenterTerraform/9.9.9 Cisco", seen["/url"])
}

// TestUserAgentWaitTask asserts the configured User-Agent reaches the async
// task-polling request, which is built with raw http.NewRequest (bypassing NewReq).
func TestUserAgentWaitTask(t *testing.T) {
	defer gock.Off()
	defer gock.Observe(nil)

	client, _ := NewClient(testURL, "usr", "pwd", MaxRetries(0), UserAgent("CatalystCenterTerraform/9.9.9 Cisco"))
	gock.InterceptClient(client.HttpClient)
	client.Token = "ABC"

	seen := map[string]string{}
	captureUserAgent(seen)

	gock.New(testURL).Get("/api/v1/task/123").Reply(200).BodyString(`{"response": {"endTime": "1", "isError": false}}`)
	_, err := client.WaitTask(&Req{}, &Res{Raw: `{"response": {"taskId": "123"}}`})
	assert.NoError(t, err)

	assert.Equal(t, "CatalystCenterTerraform/9.9.9 Cisco", seen["/api/v1/task/123"])
}
