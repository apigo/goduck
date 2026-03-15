# GoDuck

GoDuck is a small DuckDuckGo search client for Go.

It uses DuckDuckGo's HTML search endpoint and exposes a small API that feels like regular Go:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/apigo/goduck"
)

func main() {
	resp, err := goduck.Search(context.Background(), "golang generics", goduck.WithMaxResults(5))
	if err != nil {
		log.Fatal(err)
	}

	for _, result := range resp.Results {
		fmt.Printf("%s\n%s\n\n", result.Title, result.URL)
	}
}
```

You can also create a reusable client:

```go
client := goduck.New()

resp, err := client.Search(
	context.Background(),
	"site:go.dev slog",
	goduck.WithRegion("us-en"),
	goduck.WithSafeSearch(goduck.SafeSearchStrict),
	goduck.WithTimeLimit(goduck.TimeMonth),
)
```

Search responses include:

- `InstantAnswer` when DuckDuckGo shows a zero-click answer.
- `Results` with title, URL, snippet, display URL, and source host.
- `HasMore` and `NextPage` when another results page is available.
