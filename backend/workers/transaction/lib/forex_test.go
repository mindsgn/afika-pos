package lib

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestFetchHistoricalFXRate(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"date":"2024-01-02","base":"USD","quote":"ZAR","rate":18.5}`)
	}))
	t.Cleanup(server.Close)

	fetcher := &HistoricalFXFetcher{BaseURL: server.URL}
	got, err := fetcher.FetchHistoricalRate(context.Background(), "usd", "zar", "2024-01-02")
	if err != nil {
		t.Fatalf("FetchHistoricalRate: %v", err)
	}
	if got != "18.50000000" {
		t.Fatalf("rate = %q, want 18.50000000", got)
	}
	if gotPath != "/v2/rate/USD/ZAR" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotQuery != "date=2024-01-02" {
		t.Fatalf("query = %q", gotQuery)
	}
}

func TestFetchHistoricalFXRateErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		base    string
		quote   string
		date    string
		wantErr bool
	}{
		{name: "not found", status: http.StatusNotFound, body: `{}`, base: "USD", quote: "ZAR", date: "2024-01-02", wantErr: true},
		{name: "server error", status: http.StatusInternalServerError, body: `{}`, base: "USD", quote: "ZAR", date: "2024-01-02", wantErr: true},
		{name: "invalid json", status: http.StatusOK, body: `nope`, base: "USD", quote: "ZAR", date: "2024-01-02", wantErr: true},
		{name: "missing base", status: http.StatusOK, body: `{}`, quote: "ZAR", date: "2024-01-02", wantErr: true},
		{name: "missing quote", status: http.StatusOK, body: `{}`, base: "USD", date: "2024-01-02", wantErr: true},
		{name: "missing date", status: http.StatusOK, body: `{}`, base: "USD", quote: "ZAR", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()

			fetcher := &HistoricalFXFetcher{BaseURL: server.URL}
			_, err := fetcher.FetchHistoricalRate(context.Background(), tc.base, tc.quote, tc.date)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestDateFromTimestampMs(t *testing.T) {
	ms := time.Date(2024, 5, 6, 23, 59, 0, 0, time.UTC).UnixMilli()
	if got := DateFromTimestampMs(ms); got != "2024-05-06" {
		t.Fatalf("date = %q, want 2024-05-06", got)
	}
	if got := DateFromTimestampMs(0); got != time.Now().UTC().Format("2006-01-02") {
		t.Fatalf("date = %q, want today", got)
	}
}

type countingRateFetcher struct {
	mu      sync.Mutex
	calls   int
	rates   map[string]string
	errs    map[string]error
	lastKey string
}

func (f *countingRateFetcher) FetchHistoricalRate(_ context.Context, base, quote, date string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastKey = base + "/" + quote + "@" + date
	if err := f.errs[quote]; err != nil {
		return "", err
	}
	return f.rates[quote], nil
}

func TestCachedRateFetcher(t *testing.T) {
	source := &countingRateFetcher{rates: map[string]string{"ZAR": "18", "EUR": "0.92"}}
	cached := &CachedRateFetcher{Source: source}
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		got, err := cached.FetchHistoricalRate(ctx, "USD", "ZAR", "2024-01-02")
		if err != nil {
			t.Fatalf("FetchHistoricalRate: %v", err)
		}
		if got != "18" {
			t.Fatalf("rate = %q, want 18", got)
		}
	}
	if source.calls != 1 {
		t.Fatalf("source calls = %d, want 1", source.calls)
	}

	if _, err := cached.FetchHistoricalRate(ctx, "USD", "ZAR", "2024-01-03"); err != nil {
		t.Fatalf("FetchHistoricalRate: %v", err)
	}
	if source.calls != 2 {
		t.Fatalf("source calls = %d, want 2 after a new date", source.calls)
	}
}

func TestCachedRateFetcherDoesNotCacheErrors(t *testing.T) {
	source := &countingRateFetcher{
		rates: map[string]string{"ZAR": "18"},
		errs:  map[string]error{"EUR": errors.New("eur down")},
	}
	cached := &CachedRateFetcher{Source: source}
	ctx := context.Background()

	if _, err := cached.FetchHistoricalRate(ctx, "USD", "EUR", "2024-01-02"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := cached.FetchHistoricalRate(ctx, "USD", "EUR", "2024-01-02"); err == nil {
		t.Fatal("expected error")
	}
	if source.calls != 2 {
		t.Fatalf("source calls = %d, want 2", source.calls)
	}
}
