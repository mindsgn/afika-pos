package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/mindsgn-studio/afika-backend/workers/balance/lib"
)

const logPrefix = "[worker-balance]"

// BalanceResult is the summary returned by a worker run.
type BalanceResult struct {
	Saved  int `json:"saved"`
	Failed int `json:"failed"`
	Total  int `json:"total"`
}

// balanceFetcher is the on-chain balance source used by processBalances.
type balanceFetcher interface {
	FetchAllBalances(ctx context.Context, wallet lib.Wallet, networks map[string]lib.NetworkConfig, tokens map[string][]lib.TokenConfig) ([]lib.Balance, error)
}

func processBalances(ctx context.Context, store lib.BalanceStore, fetcher balanceFetcher) (BalanceResult, error) {
	wallets, err := store.FetchWallets(ctx)
	if err != nil {
		return BalanceResult{}, err
	}
	networks, tokens, err := store.FetchTokenRegistry(ctx)
	if err != nil {
		log.Printf("%s token registry load failed, using defaults: %v", logPrefix, err)
		networks = lib.CloneNetworkConfigMap(lib.SupportedNetworks)
		tokens = lib.CloneTokenConfigMap(lib.ERC20Tokens)
	}

	tokens = lib.EnsureBaseStocks(tokens)
	stockTokens, err := store.FetchStocks(ctx)
	if err != nil {
		log.Printf("%s stocks load failed: %v", logPrefix, err)
	} else {
		tokens = lib.MergeBaseMainnetStocks(tokens, stockTokens)
	}
	quotes, _ := store.FetchStockQuotes(ctx)
	if quotes == nil {
		quotes = map[string]lib.StockQuote{}
	}

	stockContracts := stockContractSet(stockTokens)
	usdZar, _ := store.LatestFXRate(ctx, "USD/ZAR")
	usdEur, _ := store.LatestFXRate(ctx, "USD/EUR")
	now := time.Now().UnixMilli()
	wallets = walletsForRun(wallets, now)

	var saved, failed int

	for _, wallet := range wallets {
		balances, err := fetcher.FetchAllBalances(ctx, wallet, networks, tokens)
		if err != nil {
			log.Printf("%s fetch balances for wallet %s: %v", logPrefix, wallet.ID, err)
			failed++
			continue
		}

		valued := make([]lib.Balance, 0, len(balances))
		for _, balance := range balances {
			quote, isStock := quotes[strings.ToLower(strings.TrimSpace(balance.TokenAddress))]
			if isStock {
				mult := strings.TrimSpace(quote.Multiplier)
				if mult == "" {
					mult = "1"
				}
				balance.Multiplier = mult
				balance.UIAmount = lib.MultiplyDecimal(balance.Amount, mult)
			}
			usd, zar, eur := deriveAmounts(ctx, store, balance.TokenSymbol, balance.Amount, quote)
			balance.USDAmount = usd
			balance.ZARAmount = zar
			balance.EURAmount = eur

			if err := store.SaveBalance(ctx, balance); err != nil {
				log.Printf("%s save balance %s/%s: %v",
					logPrefix, wallet.ID, balance.TokenSymbol, err)
				failed++
				continue
			}

			valued = append(valued, balance)
			saved++
		}

		if err := saveWalletPortfolio(ctx, store, wallet, valued, stockContracts, usdZar, usdEur, now); err != nil {
			log.Printf("%s portfolio for wallet %s: %v", logPrefix, wallet.ID, err)
		}
	}

	return BalanceResult{
		Saved:  saved,
		Failed: failed,
		Total:  saved + failed,
	}, nil
}

// saveWalletPortfolio stores the wallet's net worth breakdown and a daily
// snapshot used to report performance over time.
func saveWalletPortfolio(
	ctx context.Context,
	store lib.BalanceStore,
	wallet lib.Wallet,
	balances []lib.Balance,
	stockContracts map[string]bool,
	usdZar, usdEur string,
	nowMs int64,
) error {
	key := lib.WalletKey(wallet)
	if key == "" {
		return fmt.Errorf("wallet has no address or id")
	}

	snapshots, err := store.FetchPortfolioSnapshots(ctx, key)
	if err != nil {
		log.Printf("%s snapshots for wallet %s: %v", logPrefix, wallet.ID, err)
		snapshots = nil
	}

	summary := lib.BuildPortfolioSummary(balances, stockContracts, usdZar, usdEur, snapshots, nowMs)
	if err := store.SavePortfolio(ctx, key, summary); err != nil {
		return err
	}
	return store.SavePortfolioSnapshot(ctx, key, nowMs, summary.NetWorth)
}

// maxWalletsPerRun keeps a run below two Workers Free limits at once. Each
// wallet costs one Alchemy request (50 external subrequests per invocation),
// but saving ~41 balances costs ~41 D1 calls, which count as internal
// subrequests (1,000 per invocation). Twenty wallets is ~820 writes, leaving
// room for registry reads and portfolio snapshots. Larger registries rotate
// hourly so every wallet is still refreshed.
const maxWalletsPerRun = 20

func walletsForRun(wallets []lib.Wallet, nowMs int64) []lib.Wallet {
	if len(wallets) <= maxWalletsPerRun {
		return wallets
	}

	start := int((nowMs / int64(time.Hour/time.Millisecond)) % int64(len(wallets)))
	selected := make([]lib.Wallet, 0, maxWalletsPerRun)
	for i := 0; i < maxWalletsPerRun; i++ {
		selected = append(selected, wallets[(start+i)%len(wallets)])
	}
	log.Printf("%s %d wallets registered, processing %d from offset %d this run",
		logPrefix, len(wallets), maxWalletsPerRun, start)
	return selected
}

func stockContractSet(tokens []lib.TokenConfig) map[string]bool {
	set := map[string]bool{}
	for _, token := range tokens {
		if contract := strings.ToLower(strings.TrimSpace(token.Contract)); contract != "" {
			set[contract] = true
		}
	}
	for _, token := range lib.BaseStockTokens {
		if contract := strings.ToLower(strings.TrimSpace(token.Contract)); contract != "" {
			set[contract] = true
		}
	}
	return set
}

func deriveAmounts(ctx context.Context, store lib.BalanceStore, symbol string, amount string, quote lib.StockQuote) (string, string, string) {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	switch s {
	case "USDC", "USDT", "DAI":
		usd := strings.TrimSpace(amount)
		return usd, convertAmount(ctx, store, "USD/ZAR", usd), convertAmount(ctx, store, "USD/EUR", usd)
	case "ZARP":
		return "", strings.TrimSpace(amount), ""
	default:
		price := strings.TrimSpace(quote.Price)
		if price == "" {
			return "", "", ""
		}
		usd := lib.MultiplyDecimal(amount, price)
		if usd == "" {
			return "", "", ""
		}
		return usd, convertAmount(ctx, store, "USD/ZAR", usd), convertAmount(ctx, store, "USD/EUR", usd)
	}
}

func convertAmount(ctx context.Context, store lib.BalanceStore, pair, amount string) string {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return ""
	}

	rate, err := store.LatestFXRate(ctx, pair)
	if err != nil {
		log.Printf("%s latest FX rate %s lookup failed: %v", logPrefix, pair, err)
		return ""
	}

	value, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		log.Printf("%s parse amount %q: %v", logPrefix, amount, err)
		return ""
	}

	rateValue, err := strconv.ParseFloat(strings.TrimSpace(rate), 64)
	if err != nil {
		log.Printf("%s parse %s rate %q: %v", logPrefix, pair, rate, err)
		return ""
	}

	return fmt.Sprintf("%.8f", value*rateValue)
}
