package lib

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultFrankfurterBaseURL = "https://api.frankfurter.dev"

// RateFetcher returns a historical FX rate as a decimal string.
type RateFetcher interface {
	FetchHistoricalRate(ctx context.Context, base, quote, date string) (string, error)
}

type frankfurterRateResponse struct {
	Date  string  `json:"date"`
	Base  string  `json:"base"`
	Quote string  `json:"quote"`
	Rate  float64 `json:"rate"`
}

// HistoricalFXFetcher reads a dated rate from the Frankfurter v2 API.
type HistoricalFXFetcher struct {
	// Client defaults to an http.Client with a 10s timeout.
	Client *http.Client
	// BaseURL defaults to https://api.frankfurter.dev. Tests point this at an
	// httptest server.
	BaseURL string
}

func NewHistoricalFXFetcher(client *http.Client) *HistoricalFXFetcher {
	return &HistoricalFXFetcher{Client: client}
}

func (f *HistoricalFXFetcher) httpClient() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (f *HistoricalFXFetcher) baseURL() string {
	base := strings.TrimSpace(f.BaseURL)
	if base == "" {
		base = defaultFrankfurterBaseURL
	}
	return strings.TrimRight(base, "/")
}

// FetchHistoricalRate requests /v2/rate/{base}/{quote}?date=YYYY-MM-DD.
func (f *HistoricalFXFetcher) FetchHistoricalRate(ctx context.Context, base, quote, date string) (string, error) {
	base = strings.ToUpper(strings.TrimSpace(base))
	quote = strings.ToUpper(strings.TrimSpace(quote))
	date = strings.TrimSpace(date)

	if base == "" || quote == "" || date == "" {
		return "", fmt.Errorf("FetchHistoricalRate: base, quote and date are required")
	}

	endpoint := fmt.Sprintf("%s/v2/rate/%s/%s?date=%s",
		f.baseURL(), url.PathEscape(base), url.PathEscape(quote), url.QueryEscape(date))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("FetchHistoricalRate: rate not found for %s/%s on %s", base, quote, date)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("FetchHistoricalRate: frankfurter HTTP %d", resp.StatusCode)
	}

	var parsed frankfurterRateResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}

	return strconv.FormatFloat(parsed.Rate, 'f', 8, 64), nil
}

// DateFromTimestampMs formats a transaction timestamp as a UTC date.
func DateFromTimestampMs(ts int64) string {
	if ts == 0 {
		return time.Now().UTC().Format("2006-01-02")
	}
	return time.UnixMilli(ts).UTC().Format("2006-01-02")
}

// CachedRateFetcher memoizes successful lookups for the lifetime of one worker
// run. Many transactions share the same date, so this keeps the number of
// Frankfurter subrequests bounded by the number of distinct quote/date pairs.
type CachedRateFetcher struct {
	Source RateFetcher

	mu    sync.Mutex
	cache map[string]string
}

func (f *CachedRateFetcher) FetchHistoricalRate(ctx context.Context, base, quote, date string) (string, error) {
	if f.Source == nil {
		return "", fmt.Errorf("CachedRateFetcher: source is required")
	}

	key := rateCacheKey(base, quote, date)
	f.mu.Lock()
	if rate, ok := f.cache[key]; ok {
		f.mu.Unlock()
		return rate, nil
	}
	f.mu.Unlock()

	rate, err := f.Source.FetchHistoricalRate(ctx, base, quote, date)
	if err != nil {
		return "", err
	}

	f.mu.Lock()
	if f.cache == nil {
		f.cache = map[string]string{}
	}
	f.cache[key] = rate
	f.mu.Unlock()
	return rate, nil
}

func rateCacheKey(base, quote, date string) string {
	return strings.ToUpper(strings.TrimSpace(base)) + "|" +
		strings.ToUpper(strings.TrimSpace(quote)) + "|" +
		strings.TrimSpace(date)
}
