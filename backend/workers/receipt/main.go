package main

import (
	"context"
	"log"
	"time"

	"github.com/mindsgn-studio/afika-backend/workers/receipt/lib"
)

const logPrefix = "[worker-receipt]"

// maxPendingPerRun caps how many payments one cron run inspects. Each pending
// payment costs one D1 read, so a large backlog is worked off over several runs
// instead of blowing the request's D1 limits.
const maxPendingPerRun = 200

// Result is the per-run summary logged as JSON.
type Result struct {
	Expired int64 `json:"expired"`
	Checked int   `json:"checked"`
	Matched int   `json:"matched"`
	Failed  int   `json:"failed"`
}

// processPayments expires stale links, then settles every pending payment that
// has a matching incoming transfer. nowMs == 0 means "use the current time".
//
// Bitcoin is not watched yet: no current worker writes BTC rows into
// `wallet_transactions`, so BTC payments stay pending until they expire. They
// are still counted as checked, and the merchant can confirm them manually from
// the app.
func processPayments(ctx context.Context, store lib.PaymentStore, nowMs int64) (Result, error) {
	if nowMs == 0 {
		nowMs = time.Now().UnixMilli()
	}

	expired, err := store.ExpirePending(ctx, nowMs)
	if err != nil {
		return Result{}, err
	}
	result := Result{Expired: expired}

	pending, err := store.ListPending(ctx, nowMs, maxPendingPerRun)
	if err != nil {
		return result, err
	}
	result.Checked = len(pending)

	for _, payment := range pending {
		symbol := lib.AssetSymbol(payment.Asset)
		networks := lib.NetworkAliases(payment.Network)
		if symbol == "" || len(networks) == 0 {
			result.Failed++
			log.Printf("%s skip %s: unknown asset %q/network %q", logPrefix, payment.ID, payment.Asset, payment.Network)
			continue
		}

		credits, err := store.ListCredits(ctx, lib.CreditQuery{
			Address:  payment.Address,
			Networks: networks,
			Symbol:   symbol,
			SinceMs:  payment.CreatedAt,
		})
		if err != nil {
			result.Failed++
			log.Printf("%s credits %s: %v", logPrefix, payment.ID, err)
			continue
		}

		credit, ok := lib.MatchCredit(payment.CryptoAmount, credits)
		if !ok {
			continue
		}
		if err := store.MarkPaid(ctx, payment.ID, credit.TxHash, nowMs); err != nil {
			result.Failed++
			log.Printf("%s mark paid %s: %v", logPrefix, payment.ID, err)
			continue
		}
		result.Matched++
		log.Printf("%s paid %s receipt=%s tx=%s", logPrefix, payment.ID, payment.ReceiptNo, credit.TxHash)
	}

	return result, nil
}
