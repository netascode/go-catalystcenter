package cc

import (
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

// TestClientGet_FirstNonArray tests the Client.Get against non-array attribute "response".
func TestClientGet_FirstNonArray(t *testing.T) {
	defer gock.Off()
	client := authenticatedTestClient()

	gock.New(testURL).Get("/url").
		Reply(200).
		BodyString(`{"response":"a string"}`)

	res, err := client.Get("/url")
	assert.NoError(t, err)
	assert.Equal(t, "a string", res.Get("response").String())
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
