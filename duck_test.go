package goduck

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestParseResponse(t *testing.T) {
	t.Parallel()

	body := `<!doctype html>
<html>
  <body>
    <div class="zci-wrapper">
      <div class="zci">
        <h1 class="zci__heading"><a href="https://en.wikipedia.org/wiki/Go_(programming_language)">Go (programming language)</a></h1>
        <div class="zci__result">
          Go is expressive, concise, clean, and efficient.
          <a href="https://en.wikipedia.org/wiki/Go_(programming_language)">More at <q>Wikipedia</q></a>
        </div>
      </div>
    </div>
    <div id="links" class="results">
      <div class="result results_links results_links_deep result--ad">
        <a class="result__a" href="https://duckduckgo.com/y.js?ad=1">Ad result</a>
      </div>
      <div class="result results_links results_links_deep web-result">
        <h2 class="result__title"><a class="result__a" href="https://go.dev/">The Go Programming Language</a></h2>
        <div class="result__extras"><a class="result__url" href="https://go.dev/">go.dev</a></div>
        <a class="result__snippet" href="https://go.dev/">Go makes it simple to build secure, scalable systems.</a>
      </div>
      <div class="result results_links results_links_deep web-result">
        <h2 class="result__title"><a class="result__a" href="https://pkg.go.dev/">pkg.go.dev</a></h2>
        <div class="result__extras"><a class="result__url" href="https://pkg.go.dev/">pkg.go.dev</a></div>
        <a class="result__snippet" href="https://pkg.go.dev/">Go package discovery and docs.</a>
      </div>
      <div class="nav-link">
        <form action="/html/" method="post">
          <input type="hidden" name="s" value="10" />
        </form>
      </div>
    </div>
  </body>
</html>`

	got, err := parseResponse([]byte(body), "golang", 1)
	if err != nil {
		t.Fatalf("parseResponse() error = %v", err)
	}

	if got.Query != "golang" {
		t.Fatalf("Query = %q, want %q", got.Query, "golang")
	}
	if got.InstantAnswer == nil {
		t.Fatal("InstantAnswer = nil, want value")
	}
	if got.InstantAnswer.Title != "Go (programming language)" {
		t.Fatalf("InstantAnswer.Title = %q", got.InstantAnswer.Title)
	}
	if got.InstantAnswer.Source != "Wikipedia" {
		t.Fatalf("InstantAnswer.Source = %q", got.InstantAnswer.Source)
	}
	if len(got.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(got.Results))
	}
	if got.Results[0].Title != "The Go Programming Language" {
		t.Fatalf("Results[0].Title = %q", got.Results[0].Title)
	}
	if got.Results[0].Source != "go.dev" {
		t.Fatalf("Results[0].Source = %q", got.Results[0].Source)
	}
	if !got.HasMore {
		t.Fatal("HasMore = false, want true")
	}
	if got.NextPage != 2 {
		t.Fatalf("NextPage = %d, want 2", got.NextPage)
	}
}

func TestClientSearch(t *testing.T) {
	t.Parallel()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("Method = %s, want POST", req.Method)
		}
		if got := req.Header.Get("User-Agent"); got != "test-agent" {
			t.Fatalf("User-Agent = %q, want test-agent", got)
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("ReadAll(req.Body) error = %v", err)
		}

		form := string(body)
		for _, want := range []string{"q=golang", "kl=us-en", "kp=1", "df=m", "s=10"} {
			if !strings.Contains(form, want) {
				t.Fatalf("request body %q does not contain %q", form, want)
			}
		}

		responseBody := `<html><body><div id="links"><div class="result web-result"><a class="result__a" href="https://go.dev/">Go</a><a class="result__url" href="https://go.dev/">go.dev</a><a class="result__snippet" href="https://go.dev/">Official site.</a></div></div></body></html>`
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(responseBody)),
			Request:    req,
		}, nil
	})

	client := New(
		WithUserAgent("test-agent"),
		WithHTTPClient(&http.Client{Timeout: time.Second, Transport: transport}),
		WithBaseURL("https://example.com/html/"),
	)

	got, err := client.Search(
		context.Background(),
		"golang",
		WithRegion("us-en"),
		WithSafeSearch(SafeSearchStrict),
		WithTimeLimit(TimeMonth),
		WithPage(2),
	)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(got.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(got.Results))
	}
	if got.Results[0].Title != "Go" {
		t.Fatalf("Results[0].Title = %q, want Go", got.Results[0].Title)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	t.Parallel()

	_, err := Search(context.Background(), "   ")
	if err == nil {
		t.Fatal("Search() error = nil, want ErrEmptyQuery")
	}
	if !strings.Contains(err.Error(), ErrEmptyQuery.Error()) {
		t.Fatalf("Search() error = %v, want %v", err, ErrEmptyQuery)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
