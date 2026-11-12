package main

import (
	"context"
	"testing"
	"time"
)

func TestProcessRates(t *testing.T) {
	store := &fakeFXStore{}
	fetcher := &fakeRateFetcher{rates: map[string]string{
		"USD/EUR": "0.92000000",
		"USD/ZAR": "18.00000000",
		"EUR/ZAR": "19.60000000",
	}}

	result, err := processRates(context.Background(), store, fetcher, 1700000000000)
	if err != nil {
		t.Fatalf("processRates: %v", err)
	}
	if result.Saved != 3 || result.Failed != 0 || result.Total != 3 {
		t.Fatalf("result = %+v, want saved=3 failed=0 total=3", result)
	}
	if fetcher.calls != 1 {
		t.Fatalf("fetcher calls = %d, want 1", fetcher.calls)
	}
	if len(store.saved) != 3 {
		t.Fatalf("saved rates = %d, want 3", len(store.saved))
	}

	usdZar, ok := findRate(store.saved, "USD/ZAR")
	if !ok {
		t.Fatalf("USD/ZAR not saved: %+v", store.saved)
	}
	if usdZar.Rate != "18.00000000" {
		t.Fatalf("USD/ZAR rate = %q, want 18.00000000", usdZar.Rate)
	}
	if usdZar.FetchedAt != 1700000000000 {
		t.Fatalf("USD/ZAR fetchedAt = %d, want the supplied timestamp", usdZar.FetchedAt)
	}
}

func TestProcessRatesFetchError(t *testing.T) {
	store := &fakeFXStore{}
	fetcher := &fakeRateFetcher{err: errFetchRates}

	if _, err := processRates(context.Background(), store, fetcher, 1); err == nil {
		t.Fatal("expected fetch error")
	}
	if store.upserts != 0 {
		t.Fatalf("upserts = %d, want 0 after a fetch failure", store.upserts)
	}
}

func TestProcessRatesPartialSaveFailure(t *testing.T) {
	store := &fakeFXStore{errs: map[string]error{"USD/ZAR": errFetchRates}}
	fetcher := &fakeRateFetcher{rates: map[string]string{
		"USD/EUR": "0.92000000",
		"USD/ZAR": "18.00000000",
		"EUR/ZAR": "19.60000000",
	}}

	result, err := processRates(context.Background(), store, fetcher, 1)
	if err != nil {
		t.Fatalf("processRates: %v", err)
	}
	if result.Saved != 2 || result.Failed != 1 || result.Total != 3 {
		t.Fatalf("result = %+v, want saved=2 failed=1 total=3", result)
	}
	if len(store.saved) != 2 {
		t.Fatalf("saved rates = %d, want 2", len(store.saved))
	}
	if _, ok := findRate(store.saved, "USD/ZAR"); ok {
		t.Fatal("failed pair should not be recorded as saved")
	}
}

func TestProcessRatesEmpty(t *testing.T) {
	store := &fakeFXStore{}
	fetcher := &fakeRateFetcher{rates: map[string]string{}}

	result, err := processRates(context.Background(), store, fetcher, 1)
	if err != nil {
		t.Fatalf("processRates: %v", err)
	}
	if result != (ForexResult{}) {
		t.Fatalf("result = %+v, want zero value", result)
	}
	if store.upserts != 0 {
		t.Fatalf("upserts = %d, want 0", store.upserts)
	}
}

func TestProcessRatesDefaultTimestamp(t *testing.T) {
	store := &fakeFXStore{}
	fetcher := &fakeRateFetcher{rates: map[string]string{"USD/ZAR": "18.00000000"}}
	before := time.Now().UnixMilli()

	if _, err := processRates(context.Background(), store, fetcher, 0); err != nil {
		t.Fatalf("processRates: %v", err)
	}
	if len(store.saved) != 1 {
		t.Fatalf("saved rates = %d, want 1", len(store.saved))
	}
	if got := store.saved[0].FetchedAt; got < before {
		t.Fatalf("fetchedAt = %d, want >= %d", got, before)
	}
}
