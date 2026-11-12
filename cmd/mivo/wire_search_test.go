package main

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// searchTitles searches the running server and returns the titles of the hits in rank order.
func searchTitles(t *testing.T, baseURL string, values url.Values) []string {
	t.Helper()

	var results apigen.SearchResults

	getJSON(t, baseURL+"/api/v1/search?"+values.Encode(), &results)

	if results.Items == nil {
		t.Fatalf("search %v answered no items array", values)
	}

	return itemTitles(results.Items)
}

// TestSearchIsWired checks that the server answers searches from the real index instead of
// 501: a server without libraries finds nothing, and a short query is refused.
func TestSearchIsWired(t *testing.T) {
	t.Parallel()

	server := startTestServer(t, isolatedEnvironment(t), runHooks{})

	if titles := searchTitles(t, server.baseURL, url.Values{"q": {"matr"}}); len(titles) != 0 {
		t.Errorf("search without libraries = %v, want no hits", titles)
	}

	if titles := searchTitles(t, server.baseURL, url.Values{"q": {"matr"}, "type": {"movies"}}); len(titles) != 0 {
		t.Errorf("movies search without libraries = %v, want no hits", titles)
	}

	if status, body := call(t, http.MethodGet, server.baseURL+"/api/v1/search?q=m", ""); status != http.StatusBadRequest {
		t.Errorf("search with one character = %d (%s), want 400", status, body)
	}

	server.stop(t)
}
