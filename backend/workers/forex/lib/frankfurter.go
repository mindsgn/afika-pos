package lib

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultFrankfurterBaseURL = "https://api.frankfurter.dev"

// frankfurterResp is the subset of the Frankfurter response this worker needs.
type frankfurterResp struct {
	Base  string             `json:"base"`
	Rates map[string]float64 `json:"rates"`
}

// RateFetcher returns rates keyed by uppercase pair (for example "USD/ZAR")
// with the value formatted as a decimal string.
type RateFetcher interface {
	FetchRates(ctx context.Context) (map[string]string, error)
}

// FrankfurterFetcher fetches latest rates from the Frankfurter API, one request
// per base currency.
type FrankfurterFetcher struct {
	// Client defaults to an http.Client with a 10s timeout.
	Client *http.Client
	// BaseURL defaults to https://api.frankfurter.dev. Tests point this at an
	// httptest server.
	BaseURL string
	// Bases defaults to SupportedCurrencies.
	Bases []string
}

func NewFrankfurterFetcher() *FrankfurterFetcher {
	return &FrankfurterFetcher{}
}

func (f *FrankfurterFetcher) httpClient() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (f *FrankfurterFetcher) bases() []string {
	if len(f.Bases) == 0 {
		return SupportedCurrencies
	}
	out := make([]string, 0, len(f.Bases))
	for _, base := range f.Bases {
		if base = strings.ToUpper(strings.TrimSpace(base)); base != "" {
			out = append(out, base)
		}
	}
	return out
}

func (f *FrankfurterFetcher) baseURL() string {
	base := strings.TrimSpace(f.BaseURL)
	if base == "" {
		base = defaultFrankfurterBaseURL
	}
	return strings.TrimRight(base, "/")
}

// FetchRates queries every configured base and combines the quote maps into a
// single pair -> rate map. Any provider failure aborts the run so a partial set
// of rates is never written as if it were complete.
func (f *FrankfurterFetcher) FetchRates(ctx context.Context) (map[string]string, error) {
	combined := make(map[string]string, len(f.bases())*2)

	for _, base := range f.bases() {
		quotes, err := f.fetchBase(ctx, base)
		if err != nil {
			return nil, err
		}
		for quote, rate := range quotes {
			quote = strings.ToUpper(strings.TrimSpace(quote))
			if quote == "" {
				continue
			}
			pair := base + "/" + quote
			combined[pair] = strconv.FormatFloat(rate, 'f', 8, 64)
		}
	}
	return combined, nil
}

func (f *FrankfurterFetcher) fetchBase(ctx context.Context, base string) (map[string]float64, error) {
	base = strings.ToUpper(strings.TrimSpace(base))
	endpoint := f.baseURL() + "/v1/latest?base=" + url.QueryEscape(base)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("frankfurter %s: create request: %w", base, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("frankfurter %s: %w", base, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("frankfurter %s: HTTP %d", base, resp.StatusCode)
	}

	var parsed frankfurterResp
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("frankfurter %s: decode response: %w", base, err)
	}
	return parsed.Rates, nil
}
