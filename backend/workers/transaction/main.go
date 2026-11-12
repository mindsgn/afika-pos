package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mindsgn-studio/afika-backend/workers/transaction/lib"
)

const logPrefix = "[worker-transaction]"

// transferFetcher is the on-chain transfer source used by processTransactions.
// One call per wallet keeps the worker inside the subrequest budget.
type transferFetcher interface {
	FetchWalletTransfers(ctx context.Context, network, address string, fromBlock uint64) ([]lib.TransactionItem, error)
}

// processTransactions syncs every wallet from its last cursor. nowMs == 0
// means "use the current time" for fetchedAt bookkeeping.
func processTransactions(
	ctx context.Context,
	store lib.TransactionStore,
	transfers transferFetcher,
	rates lib.RateFetcher,
	nowMs int64,
) (lib.WorkerResult, error) {
	wallets, err := store.FetchWallets(ctx)
	if err != nil {
		return lib.WorkerResult{}, err
	}
	if nowMs == 0 {
		nowMs = time.Now().UnixMilli()
	}
	wallets = walletsForRun(wallets)

	var saved, failed int
	remaining := maxTransactionsPerRun
	for _, wallet := range wallets {
		// D1 calls are internal subrequests, and Workers Free allows 1,000 per
		// invocation. Stop before the ceiling so the run always finishes and
		// advances cursors instead of dying part-way through a busier wallet.
		if remaining <= 0 {
			log.Printf("%s transaction budget exhausted, remaining wallets continue next run", logPrefix)
			break
		}

		network, err := lib.ResolveNetwork(wallet.Network, wallet.Testnet)
		if err != nil {
			log.Printf("%s wallet %s network: %v", logPrefix, wallet.ID, err)
			failed++
			continue
		}

		walletAddress := lib.WalletKey(wallet)
		if walletAddress == "" {
			log.Printf("%s wallet %s has no address or id", logPrefix, wallet.ID)
			failed++
			continue
		}
		fromBlock := wallet.LatestBlock

		items, err := transfers.FetchWalletTransfers(ctx, network, walletAddress, fromBlock)
		if err != nil {
			log.Printf("%s wallet %s transfer fetch: %v", logPrefix, walletAddress, err)
			failed++
			continue
		}

		all := transactionsForRun(items, min(remaining, maxTransactionsPerWallet))
		if len(all) < len(items) {
			log.Printf("%s wallet %s has %d transfers, processing %d up to block %d this run",
				logPrefix, walletAddress, len(items), len(all), all[len(all)-1].BlockNumber)
		}
		remaining -= len(all)
		maxBlock := fromBlock
		walletFailed := false

		for _, tx := range all {
			tx.FetchedAtMs = nowMs
			if tx.TimestampMs == 0 {
				tx.TimestampMs = nowMs
			}
			if tx.BlockNumber > maxBlock {
				maxBlock = tx.BlockNumber
			}

			if err := deriveAmounts(ctx, rates, &tx); err != nil {
				log.Printf("%s derive amounts %s: %v", logPrefix, tx.TxHash, err)
			}
			if err := deriveFeeAmounts(ctx, rates, &tx); err != nil {
				log.Printf("%s derive fee amounts %s: %v", logPrefix, tx.TxHash, err)
			}

			if err := store.UpsertTransaction(ctx, tx); err != nil {
				log.Printf("%s upsert %s %s: %v", logPrefix, tx.TxHash, tx.Direction, err)
				failed++
				walletFailed = true
				continue
			}
			saved++
		}

		// Only advance the cursor when every transfer in the batch was saved.
		// Otherwise the next run would start above the failed rows.
		if maxBlock > fromBlock && !walletFailed {
			if err := store.UpdateWalletLatestBlock(ctx, walletAddress, maxBlock); err != nil {
				log.Printf("%s update wallet latest block %s: %v", logPrefix, walletAddress, err)
			}
		}
	}

	return lib.WorkerResult{Saved: saved, Failed: failed, Total: saved + failed}, nil
}

// maxWalletsPerRun leaves headroom below the Workers Free limit of 50
// external subrequests per invocation. Each wallet costs one batched Alchemy
// request. Wallets are ordered least-recently-synced first by Store.FetchWallets,
// so larger registries make progress without needing an offset rotation.
const maxWalletsPerRun = 45

// maxTransactionsPerWallet bounds how many rows one wallet can write in a run.
// A wallet with hundreds of transfers would otherwise consume the whole run
// and later wallets would never be reached.
const maxTransactionsPerWallet = 150

// maxTransactionsPerRun bounds total D1 writes. D1 calls count as internal
// subrequests (1,000 per invocation on Workers Free), and each saved transfer
// costs at least one. Staying well below the ceiling leaves room for wallet
// reads and cursor updates.
const maxTransactionsPerRun = 600

func walletsForRun(wallets []lib.Wallet) []lib.Wallet {
	if len(wallets) <= maxWalletsPerRun {
		return wallets
	}
	log.Printf("%s %d wallets registered, processing %d this run",
		logPrefix, len(wallets), maxWalletsPerRun)
	return wallets[:maxWalletsPerRun]
}

// transactionsForRun caps a wallet's transfers at limit. The list is ordered by
// block ascending and the cutoff block is kept whole, so the cursor can advance
// to it without skipping transfers that share that block. The next run starts
// from the cursor (inclusive) and continues where this one stopped.
func transactionsForRun(items []lib.TransactionItem, limit int) []lib.TransactionItem {
	if limit <= 0 {
		return nil
	}
	if len(items) <= limit {
		return items
	}

	sorted := make([]lib.TransactionItem, len(items))
	copy(sorted, items)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].BlockNumber < sorted[j].BlockNumber
	})

	cutoff := sorted[limit-1].BlockNumber
	selected := make([]lib.TransactionItem, 0, limit)
	for _, tx := range sorted {
		if tx.BlockNumber > cutoff {
			break
		}
		selected = append(selected, tx)
	}
	return selected
}

func deriveAmounts(ctx context.Context, rates lib.RateFetcher, tx *lib.TransactionItem) error {
	symbol := strings.ToUpper(strings.TrimSpace(tx.TokenSymbol))
	amount := strings.TrimSpace(tx.Amount)
	tx.USDAmount = ""
	tx.ZARAmount = ""
	tx.EURAmount = ""

	switch symbol {
	case "USDC", "USDT", "DAI":
		tx.USDAmount = amount
		if amount == "" {
			return nil
		}

		zarAmount, zarErr := convertToCurrency(ctx, rates, amount, tx.TimestampMs, "ZAR")
		eurAmount, eurErr := convertToCurrency(ctx, rates, amount, tx.TimestampMs, "EUR")
		tx.ZARAmount = zarAmount
		tx.EURAmount = eurAmount
		if zarErr != nil {
			return zarErr
		}
		return eurErr
	case "ZARP":
		tx.ZARAmount = amount
	}
	return nil
}

// convertToCurrency converts a USD-denominated amount using the historical
// USD/{quote} rate for the transaction date.
func convertToCurrency(ctx context.Context, rates lib.RateFetcher, amount string, timestampMs int64, quote string) (string, error) {
	rate, err := rates.FetchHistoricalRate(ctx, "USD", quote, lib.DateFromTimestampMs(timestampMs))
	if err != nil {
		return "", fmt.Errorf("fetch historical USD/%s rate: %w", quote, err)
	}

	converted, err := multiplyAmountByRate(amount, rate)
	if err != nil {
		return "", fmt.Errorf("calculate %s amount: %w", quote, err)
	}
	return converted, nil
}

func deriveFeeAmounts(ctx context.Context, rates lib.RateFetcher, tx *lib.TransactionItem) error {
	tx.FeeUSD = strings.TrimSpace(tx.FeeUSD)
	tx.FeeZAR = strings.TrimSpace(tx.FeeZAR)
	tx.FeeEUR = strings.TrimSpace(tx.FeeEUR)

	if tx.FeeUSD == "" && strings.TrimSpace(tx.FeeNative) != "" {
		// Treat FeeNative as a USD-denominated fee when FeeUSD is not available.
		tx.FeeUSD = strings.TrimSpace(tx.FeeNative)
	}

	if tx.FeeUSD == "" {
		return nil
	}

	zarFee, zarErr := convertToCurrency(ctx, rates, tx.FeeUSD, tx.TimestampMs, "ZAR")
	eurFee, eurErr := convertToCurrency(ctx, rates, tx.FeeUSD, tx.TimestampMs, "EUR")
	tx.FeeZAR = zarFee
	tx.FeeEUR = eurFee
	if zarErr != nil {
		return zarErr
	}
	return eurErr
}

func multiplyAmountByRate(amount, rate string) (string, error) {
	usdValue, err := strconv.ParseFloat(strings.TrimSpace(amount), 64)
	if err != nil {
		return "", fmt.Errorf("parse amount %q: %w", amount, err)
	}

	rateValue, err := strconv.ParseFloat(strings.TrimSpace(rate), 64)
	if err != nil {
		return "", fmt.Errorf("parse rate %q: %w", rate, err)
	}

	return fmt.Sprintf("%.8f", usdValue*rateValue), nil
}
