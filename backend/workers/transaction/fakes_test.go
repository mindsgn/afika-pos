package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/mindsgn-studio/afika-backend/workers/transaction/lib"
)

type fakeStore struct {
	wallets    []lib.Wallet
	walletsErr error

	saved       []lib.TransactionItem
	saveErrs    map[string]error
	upsertCalls int

	cursors   map[string]uint64
	cursorErr error
}

func (f *fakeStore) FetchWallets(context.Context) ([]lib.Wallet, error) {
	if f.walletsErr != nil {
		return nil, f.walletsErr
	}
	return f.wallets, nil
}

func (f *fakeStore) UpsertTransaction(_ context.Context, tx lib.TransactionItem) error {
	f.upsertCalls++
	if err := f.saveErrs[tx.TxHash+"|"+tx.Direction]; err != nil {
		return err
	}
	f.saved = append(f.saved, tx)
	return nil
}

func (f *fakeStore) UpdateWalletLatestBlock(_ context.Context, address string, blockNumber uint64) error {
	if f.cursorErr != nil {
		return f.cursorErr
	}
	if f.cursors == nil {
		f.cursors = map[string]uint64{}
	}
	f.cursors[address] = blockNumber
	return nil
}

func (f *fakeStore) Close() error { return nil }

type fakeTransfers struct {
	assets map[string][]lib.TransactionItem // address -> items, both directions
	stocks map[string][]lib.TransactionItem

	// Errors are keyed by address.
	walletErrs map[string]error

	calls []string
}

func (f *fakeTransfers) FetchWalletTransfers(_ context.Context, network, address string, fromBlock uint64) ([]lib.TransactionItem, error) {
	f.calls = append(f.calls, fmt.Sprintf("wallet:%d", fromBlock))
	if err := f.walletErrs[address]; err != nil {
		return nil, err
	}
	return lib.MergeTransfers(
		filterDirection(f.assets[address], "credit"),
		filterDirection(f.assets[address], "debit"),
		filterDirection(f.stocks[address], "credit"),
		filterDirection(f.stocks[address], "debit"),
	), nil
}

func filterDirection(items []lib.TransactionItem, direction string) []lib.TransactionItem {
	out := make([]lib.TransactionItem, 0, len(items))
	for _, item := range items {
		if item.Direction == direction {
			out = append(out, item)
		}
	}
	return out
}

type fakeRates struct {
	rates map[string]string
	errs  map[string]error
	calls int
}

func (f *fakeRates) FetchHistoricalRate(_ context.Context, base, quote, date string) (string, error) {
	f.calls++
	if base != "USD" {
		return "", errors.New("unexpected base " + base)
	}
	if err := f.errs[quote]; err != nil {
		return "", err
	}
	rate, ok := f.rates[quote]
	if !ok {
		return "", errors.New("unexpected quote " + quote)
	}
	return rate, nil
}

func findTransaction(items []lib.TransactionItem, hash, direction string) (lib.TransactionItem, bool) {
	for _, item := range items {
		if item.TxHash == hash && item.Direction == direction {
			return item, true
		}
	}
	return lib.TransactionItem{}, false
}

var errFetch = errors.New("alchemy unavailable")
var errSave = errors.New("d1 write failed")

var _ lib.TransactionStore = (*fakeStore)(nil)
var _ transferFetcher = (*fakeTransfers)(nil)
var _ lib.RateFetcher = (*fakeRates)(nil)
