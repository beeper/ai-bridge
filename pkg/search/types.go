package search

// Request represents a normalized web search request.
type Request struct {
	Query      string
	Count      int
	Country    string
	SearchLang string
	UILang     string
	Freshness  string
}

// Result is a normalized search result.
type Result struct {
	Title       string
	URL         string
	Description string
	Published   string
	SiteName    string
}

// Response is a normalized search response.
type Response struct {
	Query      string
	Provider   string
	Count      int
	TookMs     int64
	Results    []Result
	Answer     string
	Summary    string
	Definition string
	Warning    string
	NoResults  bool
	Cached     bool
	Extras     map[string]any
}
