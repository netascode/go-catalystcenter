package cc

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

// TestClientGet_PagesBasic is like TestClientGet, but with basic pagination.
func TestClientGet_PagesBasic(t *testing.T) {
	defer gock.Off()
	client := authenticatedTestClient()
	var err error

	// For pagination tests to be readable, we use dummy page size of 3 instead of 500.
	// Since we are changing a package-level var, this test cannot be run on t.Parallel().
	maxItems = 3

	gock.New(testURL).Get("/url").
		Reply(200).
		BodyString(`{"response":["1","2","3"]}`)
	gock.New(testURL).Get("/url").MatchParam("offset", "4").
		Reply(200).
		BodyString(`{"response":["4","5","6"]}`)
	gock.New(testURL).Get("/url").MatchParam("offset", "7").
		Reply(200).
		BodyString(`{"response":["7","8"]}`)

	res, err := client.Get("/url")
	assert.NoError(t, err)
	assert.Equal(t, `{"response":["1","2","3","4","5","6","7","8"]}`, res.Raw)
}

// TestClientGet_PagesExplicit is like TestClientGet_PagesBasic, but with explicit limit parameter.
func TestClientGet_PagesExplicit(t *testing.T) {
	defer gock.Off()
	client := authenticatedTestClient()

	// For pagination tests to be readable, we use dummy page size of 3 instead of 500.
	// Since we are changing a package-level var, this test cannot be run on t.Parallel().
	maxItems = 3

	// For now parameter must be equal to the max limit.
	gock.New(testURL).Get("/url").MatchParam("limit", "3").
		Reply(200).
		BodyString(`{"response":[1,2,3]}`)
	gock.New(testURL).Get("/url").MatchParam("limit", "3").MatchParam("offset", "4").
		Reply(200).
		BodyString(`{"response":[4]}`)

	res, err := client.Get("/url?limit=3")
	assert.NoError(t, err)
	assert.Equal(t, `{"response":[1,2,3,4]}`, res.Raw)
}

// TestClientGet_PagesWithExtras is like TestClientGet_PagesBasic and ensures that the extra attributes from the last
// page prevail.
func TestClientGet_PagesWithExtras(t *testing.T) {
	defer gock.Off()
	client := authenticatedTestClient()

	// For pagination tests to be readable, we use dummy page size of 3 instead of 500.
	// Since we are changing a package-level var, this test cannot be run on t.Parallel().
	maxItems = 3

	gock.New(testURL).Get("/url").
		Reply(200).
		BodyString(`{"response":[1,2,3],"extra":42}`)
	gock.New(testURL).Get("/url").MatchParam("offset", "4").
		Reply(200).
		BodyString(`{"response":[4],"extra":"x"}`)

	res, err := client.Get("/url")
	assert.NoError(t, err)
	assert.Equal(t, `{"response":[1,2,3,4],"extra":"x"}`, res.Raw)
}

// TestClientGet_ArrayVaries tests the Client.Get against a corner case when response varies between
// array and non-array.
func TestClientGet_ArrayVaries(t *testing.T) {
	defer gock.Off()
	client := authenticatedTestClient()
	var err error

	// For pagination tests to be readable, we use dummy page size of 3 instead of 500.
	// Since we are changing a package-level var, this test cannot be run on t.Parallel().
	maxItems = 3

	gock.New(testURL).Get("/url").
		Reply(200).
		BodyString(`{"response":["1","2","3"]}`)
	gock.New(testURL).Get("/url").MatchParam("offset", "4").
		Reply(200).
		BodyString(`{"response":"a string"}`)

	_, err = client.Get("/url")
	assert.Error(t, err)
}

// TestClientGet_LastPageEmpty is like TestClientGet_PagesBasic, but when the last page is empty.
func TestClientGet_LastPageEmpty(t *testing.T) {
	defer gock.Off()
	client := authenticatedTestClient()

	// For pagination tests to be readable, we use dummy page size of 3 instead of 500.
	// Since we are changing a package-level var, this test cannot be run on t.Parallel().
	maxItems = 3

	gock.New(testURL).Get("/url").
		Reply(200).
		BodyString(`{"response":["1","2","3"]}`)

	gock.New(testURL).Get("/url").MatchParam("offset", "4").
		Reply(200).
		BodyString(`{"response":[]}`)

	res, err := client.Get("/url")
	assert.NoError(t, err)
	assert.Equal(t, `{"response":["1","2","3"]}`, res.Raw)
}

// TestClientGet_PageTooBig is like TestClientGet_PagesBasic, but when too many items are returned.
// Let's assume that just means the path does not know about pagination.
func TestClientGet_PageTooBig(t *testing.T) {
	defer gock.Off()
	client := authenticatedTestClient()

	// For pagination tests to be readable, we use dummy page size of 3 instead of 500.
	// Since we are changing a package-level var, this test cannot be run on t.Parallel().
	maxItems = 3

	gock.New(testURL).Get("/url").
		Reply(200).
		BodyString(`{"response":["1","2","3","4"]}`)
	gock.New(testURL).Get("/url").MatchParam("offset", ".*").
		Reply(400) // The important part: avoid further queries.

	res, err := client.Get("/url")
	assert.NoError(t, err)
	assert.Equal(t, `{"response":["1","2","3","4"]}`, res.Raw)
}

// TestClientGet_Concurrent test against a concurrent Post/Put/Delete modifying how data
// is divided into the pages. As Get glues the desired pages, the modifying methods shouldn't
// interrupt.
func TestClientGet_Concurrent(t *testing.T) {
	defer gock.Off()
	client := authenticatedTestClient()

	recorder := make(chan string)
	cleanup := make(chan bool)

	// Stochastic test: the more, the merrier.
	const (
		posters  = 10
		putters  = 10
		deleters = 10
		taskers  = 10
		getters  = 1
	)

	gock.New(testURL).Post("/url/insert").
		Times(posters).
		Reply(200).
		Map(func(resp *http.Response) *http.Response {
			recorder <- "post"
			return resp
		})

	for i := 0; i < posters; i++ {
		go func() {
			_, err := client.Post("/url/insert", "{}")
			assert.NoError(t, err)
			cleanup <- true
		}()
	}

	gock.New(testURL).Put("/url").MatchParam("id", "5").
		Times(putters).
		Reply(200).
		Map(func(resp *http.Response) *http.Response {
			recorder <- "put"
			return resp
		})

	for i := 0; i < putters; i++ {
		go func() {
			_, err := client.Put("/url?id=5", "{}")
			assert.NoError(t, err)
			cleanup <- true
		}()
	}

	gock.New(testURL).Delete("/url/5").
		Times(deleters).
		Reply(200).
		Map(func(resp *http.Response) *http.Response {
			recorder <- "delete"
			return resp
		})

	for i := 0; i < deleters; i++ {
		go func() {
			_, err := client.Delete("/url/5")
			assert.NoError(t, err)
			cleanup <- true
		}()
	}

	gock.New(testURL).Post("/taskable").
		Times(taskers).
		Reply(200).
		BodyString(`{"response": {"taskId": "123"}}`)
	gock.New(testURL).Get("/api/v1/task/123").
		Times(taskers).
		Reply(200).
		BodyString(`{"response": {"endTime": "1", "isError": false}}`).
		Map(func(resp *http.Response) *http.Response {
			recorder <- "task"
			return resp
		})

	for i := 0; i < taskers; i++ {
		go func() {
			_, err := client.Post("/taskable", "{}")
			assert.NoError(t, err)
			cleanup <- true
		}()
	}

	// For pagination tests to be readable, we use dummy page size of 3 instead of 500.
	// Since we are changing a package-level var, this test cannot be run on t.Parallel().
	maxItems = 3

	// GET that glues 3 pages.
	go func() {
		gock.New(testURL).Get("/url").
			Reply(200).
			BodyString(`{"response":[{},{},{}]}`).
			Map(func(resp *http.Response) *http.Response {
				recorder <- "get"
				return resp
			})
		gock.New(testURL).Get("/url").MatchParam("offset", "4").
			Reply(200).
			BodyString(`{"response":[{},{},{}]}`).
			Map(func(resp *http.Response) *http.Response {
				recorder <- "get"
				return resp
			})
		gock.New(testURL).Get("/url").MatchParam("offset", "7").
			Reply(200).
			BodyString(`{"response":[{},{}]}`).
			Map(func(resp *http.Response) *http.Response {
				recorder <- "get"
				return resp
			})

		res, err := client.Get("/url")
		assert.NoError(t, err)
		assert.Equal(t, `{"response":[{},{},{},{},{},{},{},{}]}`, res.Raw)
		cleanup <- true
	}()

	var got strings.Builder
	// Record the sequence of events in a serialized manner. The 3-page Get should show
	// as a sequence "get,get,get", uninterrupted by any random post/delete/etc.
	go func() {
		for v := range recorder {
			got.WriteString(",")
			got.WriteString(v)
		}
		cleanup <- true
	}()

	// Do not leak goroutines; do not leak t.Error calls made from inside goroutines.
	for i := 0; i < getters+posters+putters+deleters+taskers; i++ {
		<-cleanup
	}

	close(recorder)
	<-cleanup // the serializer itself

	assert.Contains(t, got.String(), "get,get,get")
}
