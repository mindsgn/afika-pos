package main

import (
	"context"
	"errors"

	"github.com/mindsgn-studio/afika-backend/workers/balance/lib"
)

type fakeStore struct {
	wallets    []lib.Wallet
	walletsErr error

	networks    map[string]lib.NetworkConfig
	tokens      map[string][]lib.TokenConfig
	registryErr error

	stocks    []lib.TokenConfig
	stocksErr error

	quotes map[string]lib.StockQuote
	rates  map[string]string

	saved   []lib.Balance
	saveErr error

	portfolios   map[string]lib.PortfolioSummary
	portfolioErr error

	snapshots      map[string][]lib.PortfolioSnapshot
	savedSnapshots map[string][]lib.PortfolioSnapshot
	snapshotsErr   error
	snapshotErr    error
}

func (f *fakeStore) FetchWallets(context.Context) ([]lib.Wallet, error) {
	return f.wallets, f.walletsErr
}

func (f *fakeStore) FetchTokenRegistry(context.Context) (map[string]lib.NetworkConfig, map[string][]lib.TokenConfig, error) {
	return f.networks, f.tokens, f.registryErr
}

func (f *fakeStore) FetchStocks(context.Context) ([]lib.TokenConfig, error) {
	return f.stocks, f.stocksErr
}

func (f *fakeStore) FetchStockQuotes(context.Context) (map[string]lib.StockQuote, error) {
	return f.quotes, nil
}

func (f *fakeStore) LatestFXRate(_ context.Context, pair string) (string, error) {
	return f.rates[pair], nil
}

func (f *fakeStore) SaveBalance(_ context.Context, b lib.Balance) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, b)
	return nil
}

func (f *fakeStore) SavePortfolio(_ context.Context, wallet string, p lib.PortfolioSummary) error {
	if f.portfolioErr != nil {
		return f.portfolioErr
	}
	if f.portfolios == nil {
		f.portfolios = map[string]lib.PortfolioSummary{}
	}
	f.portfolios[wallet] = p
	return nil
}

func (f *fakeStore) SavePortfolioSnapshot(_ context.Context, wallet string, at int64, netWorth string) error {
	if f.snapshotErr != nil {
		return f.snapshotErr
	}
	if f.savedSnapshots == nil {
		f.savedSnapshots = map[string][]lib.PortfolioSnapshot{}
	}
	f.savedSnapshots[wallet] = append(f.savedSnapshots[wallet], lib.PortfolioSnapshot{At: at, NetWorth: netWorth})
	return nil
}

func (f *fakeStore) FetchPortfolioSnapshots(_ context.Context, wallet string) ([]lib.PortfolioSnapshot, error) {
	if f.snapshotsErr != nil {
		return nil, f.snapshotsErr
	}
	return f.snapshots[wallet], nil
}

func (f *fakeStore) Close() error {
	return nil
}

type fakeFetcher struct {
	balances map[string][]lib.Balance
	errs     map[string]error
	calls    int
}

func (f *fakeFetcher) FetchAllBalances(
	_ context.Context,
	wallet lib.Wallet,
	_ map[string]lib.NetworkConfig,
	_ map[string][]lib.TokenConfig,
) ([]lib.Balance, error) {
	f.calls++
	if err := f.errs[wallet.ID]; err != nil {
		return nil, err
	}
	return f.balances[wallet.ID], nil
}

func findBalance(balances []lib.Balance, symbol string) (lib.Balance, bool) {
	for _, b := range balances {
		if b.TokenSymbol == symbol {
			return b, true
		}
	}
	return lib.Balance{}, false
}

var errFetch = errors.New("alchemy unavailable")
