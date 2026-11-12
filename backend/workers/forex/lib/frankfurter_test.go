package lib

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newFrankfurterServer(t *testing.T, rates map[string]map[string]float64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/latest" {
			t.Errorf("path = %q, want /v1/latest", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}

		base := strings.ToUpper(r.URL.Query().Get("base"))
		quotes, ok := rates[base]
		if !ok {
			t.Errorf("unexpected base %q", base)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(frankfurterResp{Base: base, Rates: quotes})
	}))
}

func TestFrankfurterFetcherFetchRates(t *testing.T) {
	server := newFrankfurterServer(t, map[string]map[string]float64{
		"USD": {"EUR": 0.92, "ZAR": 18.0},
		"EUR": {"USD": 1.08, "ZAR": 19.6},
		"ZAR": {"USD": 0.055, "EUR": 0.051},
	})
	defer server.Close()

	fetcher := &FrankfurterFetcher{BaseURL: server.URL}
	got, err := fetcher.FetchRates(context.Background())
	if err != nil {
		t.Fatalf("FetchRates: %v", err)
	}

	want := map[string]string{
		"USD/EUR": "0.92000000",
		"USD/ZAR": "18.00000000",
		"EUR/USD": "1.08000000",
		"EUR/ZAR": "19.60000000",
		"ZAR/USD": "0.05500000",
		"ZAR/EUR": "0.05100000",
	}
	for pair, rate := range want {
		if got[pair] != rate {
			t.Errorf("rate %s = %q, want %q", pair, got[pair], rate)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d rates, want %d (%v)", len(got), len(want), got)
	}
}

func TestFrankfurterFetcherRequestsEverySupportedCurrency(t *testing.T) {
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := strings.ToUpper(r.URL.Query().Get("base"))
		requested = append(requested, base)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(frankfurterResp{Base: base, Rates: map[string]float64{}})
	}))
	defer server.Close()

	fetcher := &FrankfurterFetcher{BaseURL: server.URL}
	if _, err := fetcher.FetchRates(context.Background()); err != nil {
		t.Fatalf("FetchRates: %v", err)
	}

	for _, currency := range SupportedCurrencies {
		found := false
		for _, base := range requested {
			if base == currency {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected provider call for base %s, got %v", currency, requested)
		}
	}
	if len(requested) != len(SupportedCurrencies) {
		t.Errorf("made %d requests, want %d", len(requested), len(SupportedCurrencies))
	}
}

func TestFrankfurterFetcherCustomBasesAndTrailingSlash(t *testing.T) {
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := strings.ToUpper(r.URL.Query().Get("base"))
		requested = append(requested, base)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(frankfurterResp{Base: base, Rates: map[string]float64{"USD": 1.27}})
	}))
	defer server.Close()

	fetcher := &FrankfurterFetcher{BaseURL: server.URL + "/", Bases: []string{"gbp"}}
	got, err := fetcher.FetchRates(context.Background())
	if err != nil {
		t.Fatalf("FetchRates: %v", err)
	}
	if len(requested) != 1 || requested[0] != "GBP" {
		t.Fatalf("requested = %v, want [GBP]", requested)
	}
	if got["GBP/USD"] != "1.27000000" {
		t.Fatalf("GBP/USD = %q, want 1.27000000", got["GBP/USD"])
	}
}

func TestFrankfurterFetcherErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "bad status", status: http.StatusBadGateway, body: "upstream", wantErr: "HTTP 502"},
		{name: "invalid json", status: http.StatusOK, body: "{not json", wantErr: "decode"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			fetcher := &FrankfurterFetcher{BaseURL: server.URL, Bases: []string{"USD"}}
			_, err := fetcher.FetchRates(context.Background())
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestFrankfurterFetcherStopsOnBaseFailure(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fetcher := &FrankfurterFetcher{BaseURL: server.URL, Bases: []string{"USD", "EUR", "ZAR"}}
	if _, err := fetcher.FetchRates(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (stop after first failure)", calls)
	}
}

func TestFrankfurterFetcherHTTPErrorIncludesPair(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	fetcher := &FrankfurterFetcher{BaseURL: server.URL, Bases: []string{"USD"}}
	_, err := fetcher.FetchRates(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "USD") {
		t.Fatalf("error = %v, want base in message", err)
	}
}
