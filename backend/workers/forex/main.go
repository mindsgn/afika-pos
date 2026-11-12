package main

import (
	"context"
	"log"
	"sort"
	"time"

	"github.com/mindsgn-studio/afika-backend/workers/forex/lib"
)

const logPrefix = "[worker-forex]"

// ForexResult is the summary returned by a worker run.
type ForexResult struct {
	Saved  int `json:"saved"`
	Failed int `json:"failed"`
	Total  int `json:"total"`
}

// rateFetcher is the FX rate source used by processRates.
type rateFetcher interface {
	FetchRates(ctx context.Context) (map[string]string, error)
}

// processRates fetches every supported pair and upserts it into the store.
// A failing pair is logged and counted, so one bad row does not discard the
// rest of the run. nowMs == 0 means "use the current time".
func processRates(ctx context.Context, store lib.FXStore, fetcher rateFetcher, nowMs int64) (ForexResult, error) {
	rates, err := fetcher.FetchRates(ctx)
	if err != nil {
		return ForexResult{}, err
	}
	if nowMs == 0 {
		nowMs = time.Now().UnixMilli()
	}

	pairs := make([]string, 0, len(rates))
	for pair := range rates {
		pairs = append(pairs, pair)
	}
	// Sort so logs and write order are reproducible between runs.
	sort.Strings(pairs)

	result := ForexResult{Total: len(pairs)}
	for _, pair := range pairs {
		if err := store.UpsertFXRate(ctx, pair, rates[pair], nowMs); err != nil {
			log.Printf("%s upsert %s: %v", logPrefix, pair, err)
			result.Failed++
			continue
		}
		log.Printf("%s saved %s = %s", logPrefix, pair, rates[pair])
		result.Saved++
	}
	return result, nil
}
