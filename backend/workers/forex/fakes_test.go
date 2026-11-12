package main

import (
	"context"
	"errors"

	"github.com/mindsgn-studio/afika-backend/workers/forex/lib"
)

type savedRate struct {
	Pair      string
	Rate      string
	FetchedAt int64
}

type fakeFXStore struct {
	saved   []savedRate
	errs    map[string]error
	upserts int
}

func (f *fakeFXStore) UpsertFXRate(_ context.Context, pair, rate string, fetchedAt int64) error {
	f.upserts++
	if err := f.errs[pair]; err != nil {
		return err
	}
	f.saved = append(f.saved, savedRate{Pair: pair, Rate: rate, FetchedAt: fetchedAt})
	return nil
}

type fakeRateFetcher struct {
	rates map[string]string
	err   error
	calls int
}

func (f *fakeRateFetcher) FetchRates(context.Context) (map[string]string, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.rates, nil
}

func findRate(rates []savedRate, pair string) (savedRate, bool) {
	for _, rate := range rates {
		if rate.Pair == pair {
			return rate, true
		}
	}
	return savedRate{}, false
}

var errFetchRates = errors.New("frankfurter unavailable")

// Compile-time check that the fake satisfies the production store contract.
var _ lib.FXStore = (*fakeFXStore)(nil)
