// Package goduck provides a small DuckDuckGo search client.
package goduck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// DuckDuckGo regions: https://serpapi.com/duckduckgo-regions
const (
	defaultBaseURL   = "https://html.duckduckgo.com/html/"
	defaultRegion    = "wt-wt"
	defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.3.1 Safari/605.1.15"
	defaultTimeout   = 10 * time.Second
	defaultMaxResult = 10
)

// ErrEmptyQuery is returned when Search receives an empty query.
var ErrEmptyQuery = errors.New("goduck: empty query")

// TimeLimit restricts results to a recent time window.
type TimeLimit string

const (
	TimeAny   TimeLimit = ""
	TimeDay   TimeLimit = "d"
	TimeWeek  TimeLimit = "w"
	TimeMonth TimeLimit = "m"
	TimeYear  TimeLimit = "y"
)

// SafeSearch controls DuckDuckGo safe search filtering.
type SafeSearch string

const (
	SafeSearchOff      SafeSearch = "-2"
	SafeSearchModerate SafeSearch = "-1"
	SafeSearchStrict   SafeSearch = "1"
)

// Result is a single web search result.
type Result struct {
	Title   string
	URL     string
	Snippet string
	Display string
	Source  string
}

// InstantAnswer contains DuckDuckGo's zero-click answer when available.
type InstantAnswer struct {
	Title   string
	URL     string
	Snippet string
	Source  string
}

// Response is the parsed DuckDuckGo search response.
type Response struct {
	Query         string
	InstantAnswer *InstantAnswer
	Results       []Result
	HasMore       bool
	NextPage      int
}

// Client performs DuckDuckGo searches.
type Client struct {
	baseURL    string
	httpClient *http.Client
	userAgent  string
}

// Option configures a Client.
type Option func(*Client)

// New creates a reusable DuckDuckGo client.
func New(opts ...Option) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()

	client := &Client{
		baseURL:   defaultBaseURL,
		userAgent: defaultUserAgent,
		httpClient: &http.Client{
			Timeout:   defaultTimeout,
			Transport: transport,
		},
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// WithBaseURL overrides the DuckDuckGo HTML endpoint.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if strings.TrimSpace(baseURL) != "" {
			c.baseURL = baseURL
		}
	}
}

// WithHTTPClient overrides the HTTP client used for requests.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// WithUserAgent sets the User-Agent header used for requests.
func WithUserAgent(userAgent string) Option {
	return func(c *Client) {
		if strings.TrimSpace(userAgent) != "" {
			c.userAgent = userAgent
		}
	}
}

// SearchOption configures a single search request.
type SearchOption func(*searchOptions)

type searchOptions struct {
	region     string
	timeLimit  TimeLimit
	safeSearch SafeSearch
	page       int
	maxResults int
}

func defaultSearchOptions() searchOptions {
	return searchOptions{
		region:     defaultRegion,
		safeSearch: SafeSearchModerate,
		page:       1,
		maxResults: defaultMaxResult,
	}
}

// WithRegion sets the DuckDuckGo region code, such as us-en.
func WithRegion(region string) SearchOption {
	return func(o *searchOptions) {
		if strings.TrimSpace(region) != "" {
			o.region = region
		}
	}
}

// WithTimeLimit restricts results to the requested time window.
func WithTimeLimit(limit TimeLimit) SearchOption {
	return func(o *searchOptions) {
		o.timeLimit = limit
	}
}

// WithSafeSearch sets DuckDuckGo safe search behavior.
func WithSafeSearch(mode SafeSearch) SearchOption {
	return func(o *searchOptions) {
		if mode != "" {
			o.safeSearch = mode
		}
	}
}

// WithPage requests a specific results page starting at page 1.
func WithPage(page int) SearchOption {
	return func(o *searchOptions) {
		if page > 0 {
			o.page = page
		}
	}
}

// WithMaxResults limits the number of parsed results returned.
func WithMaxResults(maxResults int) SearchOption {
	return func(o *searchOptions) {
		if maxResults > 0 {
			o.maxResults = maxResults
		}
	}
}

// Search runs a search with a default client.
func Search(ctx context.Context, query string, opts ...SearchOption) (*Response, error) {
	return New().Search(ctx, query, opts...)
}

// Search runs a DuckDuckGo HTML search and parses the response.
func (c *Client) Search(ctx context.Context, query string, opts ...SearchOption) (*Response, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrEmptyQuery
	}

	searchOpts := defaultSearchOptions()
	for _, opt := range opts {
		opt(&searchOpts)
	}

	form := url.Values{}
	form.Set("q", query)
	form.Set("b", "")
	form.Set("kl", searchOpts.region)
	form.Set("kp", string(searchOpts.safeSearch))
	if searchOpts.timeLimit != "" {
		form.Set("df", string(searchOpts.timeLimit))
	}
	if start := pageStart(searchOpts.page); start > 0 {
		form.Set("s", strconv.Itoa(start))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search duckduckgo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search duckduckgo: unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	parsed, err := parseResponse(body, query, searchOpts.maxResults)
	if err != nil {
		return nil, err
	}

	return parsed, nil
}

func pageStart(page int) int {
	if page <= 1 {
		return 0
	}
	return 10 + (page-2)*15
}

func parseResponse(body []byte, query string, maxResults int) (*Response, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse response html: %w", err)
	}

	response := &Response{Query: query}

	if zci := findFirst(doc, hasClass("div", "zci")); zci != nil {
		response.InstantAnswer = parseInstantAnswer(zci)
		if response.InstantAnswer != nil && response.InstantAnswer.Title == "" && response.InstantAnswer.Snippet == "" {
			response.InstantAnswer = nil
		}
	}

	resultsRoot := findFirst(doc, hasAttr("id", "links"))
	if resultsRoot != nil {
		for _, node := range findAll(resultsRoot, hasClass("div", "result")) {
			result, ok := parseResult(node)
			if !ok {
				continue
			}
			response.Results = append(response.Results, result)
			if maxResults > 0 && len(response.Results) >= maxResults {
				break
			}
		}
	}

	if next := findFirst(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "input" && attrEquals(n, "name", "s")
	}); next != nil {
		if start, err := strconv.Atoi(attr(next, "value")); err == nil {
			response.HasMore = true
			response.NextPage = startToPage(start)
		}
	}

	return response, nil
}

func startToPage(start int) int {
	if start <= 0 {
		return 2
	}
	return ((start - 10) / 15) + 2
}

func parseInstantAnswer(node *html.Node) *InstantAnswer {
	titleNode := findFirst(node, hasClass("h1", "zci__heading"))
	linkNode := findFirst(node, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "a" && attr(n, "href") != ""
	})
	resultNode := findFirst(node, hasClass("div", "zci__result"))

	instant := &InstantAnswer{
		Title:   cleanText(textContent(titleNode)),
		Snippet: cleanText(ownText(resultNode)),
	}

	if linkNode != nil {
		instant.URL = attr(linkNode, "href")
	}

	if moreNode := findFirst(resultNode, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "q"
	}); moreNode != nil {
		instant.Source = cleanText(textContent(moreNode))
	}

	return instant
}

func parseResult(node *html.Node) (Result, bool) {
	if classContains(node, "result--ad") {
		return Result{}, false
	}

	link := findFirst(node, hasClass("a", "result__a"))
	if link == nil {
		return Result{}, false
	}

	href := strings.TrimSpace(attr(link, "href"))
	if href == "" || strings.Contains(href, "duckduckgo.com/y.js?") {
		return Result{}, false
	}

	title := cleanText(textContent(link))
	if title == "" {
		return Result{}, false
	}

	result := Result{
		Title:   title,
		URL:     href,
		Snippet: cleanText(textContent(findFirst(node, hasClass("a", "result__snippet")))),
		Display: cleanText(textContent(findFirst(node, hasClass("a", "result__url")))),
	}

	if host, err := url.Parse(result.URL); err == nil {
		result.Source = host.Hostname()
	}

	return result, true
}

func findFirst(node *html.Node, match func(*html.Node) bool) *html.Node {
	if node == nil {
		return nil
	}
	if match(node) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findFirst(child, match); found != nil {
			return found
		}
	}
	return nil
}

func findAll(node *html.Node, match func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current == nil {
			return
		}
		if match(current) {
			out = append(out, current)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return out
}

func textContent(node *html.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == html.TextNode {
		return node.Data
	}
	var parts []string
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if text := textContent(child); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

func ownText(node *html.Node) string {
	if node == nil {
		return ""
	}
	var parts []string
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			parts = append(parts, child.Data)
		}
		if child.Type == html.ElementNode && child.Data == "br" {
			parts = append(parts, " ")
		}
	}
	return strings.Join(parts, " ")
}

func cleanText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(html.UnescapeString(value))), " ")
}

func hasClass(tag, class string) func(*html.Node) bool {
	return func(node *html.Node) bool {
		return node.Type == html.ElementNode && node.Data == tag && classContains(node, class)
	}
}

func hasAttr(key, value string) func(*html.Node) bool {
	return func(node *html.Node) bool {
		return node.Type == html.ElementNode && attrEquals(node, key, value)
	}
}

func classContains(node *html.Node, class string) bool {
	return slices.Contains(strings.Fields(attr(node, "class")), class)
}

func attrEquals(node *html.Node, key, value string) bool {
	return attr(node, key) == value
}

func attr(node *html.Node, key string) string {
	if node == nil {
		return ""
	}
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val
		}
	}
	return ""
}
