package main

import (
	"context"
	"testing"

	"github.com/mindsgn-studio/afika-backend/workers/receipt/lib"
)

const testAddress = "0x1111111111111111111111111111111111111111"

func baseStore() *fakeStore {
	return &fakeStore{
		pending: []lib.Payment{
			{
				ID:           "pay-1",
				ReceiptNo:    "POS-0001",
				Asset:        "usdc-base",
				Network:      "base",
				Address:      testAddress,
				CryptoAmount: "6.67",
				CreatedAt:    1000,
				ExpiresAt:    9000,
			},
		},
		credits: map[string][]lib.Credit{
			testAddress: {{TxHash: "0xabc", TokenSymbol: "USDC", Amount: "6.67", Network: "base-mainnet", TimestampMs: 2000}},
		},
	}
}

func TestProcessPaymentsSettlesMatch(t *testing.T) {
	store := baseStore()
	store.expired = 2

	result, err := processPayments(context.Background(), store, 5000)
	if err != nil {
		t.Fatalf("processPayments: %v", err)
	}
	if result.Expired != 2 || result.Checked != 1 || result.Matched != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(store.paid) != 1 || store.paid[0].ID != "pay-1" || store.paid[0].TxHash != "0xabc" {
		t.Fatalf("paid = %+v", store.paid)
	}
	if store.paid[0].Now != 5000 {
		t.Fatalf("paid at %d, want the run timestamp", store.paid[0].Now)
	}
	if store.lastQuery.Symbol != "USDC" || store.lastQuery.SinceMs != 1000 {
		t.Fatalf("credit query = %+v", store.lastQuery)
	}
	if len(store.lastQuery.Networks) != 2 || store.lastQuery.Networks[0] != "base-mainnet" {
		t.Fatalf("networks = %v", store.lastQuery.Networks)
	}
}

func TestProcessPaymentsIgnoresUnderpayment(t *testing.T) {
	store := baseStore()
	store.credits[testAddress] = []lib.Credit{
		{TxHash: "0xsmall", TokenSymbol: "USDC", Amount: "6.66", Network: "base-mainnet", TimestampMs: 2000},
	}

	result, err := processPayments(context.Background(), store, 5000)
	if err != nil {
		t.Fatalf("processPayments: %v", err)
	}
	if result.Matched != 0 || len(store.paid) != 0 {
		t.Fatalf("result = %+v paid = %+v", result, store.paid)
	}
}

func TestProcessPaymentsAcceptsOverpayment(t *testing.T) {
	store := baseStore()
	store.credits[testAddress] = []lib.Credit{
		{TxHash: "0xbig", TokenSymbol: "USDC", Amount: "7", Network: "base-mainnet", TimestampMs: 2000},
	}

	if _, err := processPayments(context.Background(), store, 5000); err != nil {
		t.Fatalf("processPayments: %v", err)
	}
	if len(store.paid) != 1 || store.paid[0].TxHash != "0xbig" {
		t.Fatalf("paid = %+v", store.paid)
	}
}

func TestProcessPaymentsSkipsBitcoin(t *testing.T) {
	store := baseStore()
	store.pending = []lib.Payment{{
		ID:           "pay-btc",
		Asset:        "btc",
		Network:      "bitcoin",
		Address:      "bc1qexample",
		CryptoAmount: "0.00010288",
		CreatedAt:    1000,
	}}
	store.credits["bc1qexample"] = []lib.Credit{
		{TxHash: "0xbtc", TokenSymbol: "BTC", Amount: "0.001", Network: "bitcoin", TimestampMs: 2000},
	}

	// Nothing writes BTC rows today, but the matcher is wired so a future
	// watcher works without code changes.
	if _, err := processPayments(context.Background(), store, 5000); err != nil {
		t.Fatalf("processPayments: %v", err)
	}
	if len(store.paid) != 1 || store.paid[0].ID != "pay-btc" {
		t.Fatalf("paid = %+v", store.paid)
	}
}

func TestProcessPaymentsUnknownAsset(t *testing.T) {
	store := baseStore()
	store.pending[0].Asset = "doge"

	result, err := processPayments(context.Background(), store, 5000)
	if err != nil {
		t.Fatalf("processPayments: %v", err)
	}
	if result.Failed != 1 || result.Matched != 0 {
		t.Fatalf("result = %+v", result)
	}
	if store.creditCalls != 0 {
		t.Fatalf("credit calls = %d, want 0", store.creditCalls)
	}
}

func TestProcessPaymentsContinuesAfterFailures(t *testing.T) {
	store := baseStore()
	store.pending = append(store.pending, lib.Payment{
		ID:           "pay-2",
		Asset:        "usdc-base",
		Network:      "base",
		Address:      "0x2222222222222222222222222222222222222222",
		CryptoAmount: "1.00",
		CreatedAt:    1000,
	})
	store.credits["0x2222222222222222222222222222222222222222"] = []lib.Credit{
		{TxHash: "0xdef", Amount: "1", Network: "base-mainnet", TimestampMs: 2000},
	}

	result, err := processPayments(context.Background(), store, 5000)
	if err != nil {
		t.Fatalf("processPayments: %v", err)
	}
	if result.Matched != 2 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestProcessPaymentsCreditError(t *testing.T) {
	store := baseStore()
	store.creditsErr = errStore

	result, err := processPayments(context.Background(), store, 5000)
	if err != nil {
		t.Fatalf("processPayments: %v", err)
	}
	if result.Failed != 1 || result.Matched != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestProcessPaymentsMarkFailed(t *testing.T) {
	store := baseStore()
	store.paidErr = errStore

	result, err := processPayments(context.Background(), store, 5000)
	if err != nil {
		t.Fatalf("processPayments: %v", err)
	}
	if result.Failed != 1 || result.Matched != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestProcessPaymentsExpireError(t *testing.T) {
	store := baseStore()
	store.expireErr = errStore

	if _, err := processPayments(context.Background(), store, 5000); err == nil {
		t.Fatal("expected expire error to abort the run")
	}
	if store.creditCalls != 0 {
		t.Fatalf("credit calls = %d, want 0", store.creditCalls)
	}
}

func TestProcessPaymentsListError(t *testing.T) {
	store := baseStore()
	store.pendingErr = errStore

	if _, err := processPayments(context.Background(), store, 5000); err == nil {
		t.Fatal("expected list error")
	}
}

func TestProcessPaymentsDefaultTimestamp(t *testing.T) {
	store := baseStore()

	if _, err := processPayments(context.Background(), store, 0); err != nil {
		t.Fatalf("processPayments: %v", err)
	}
	if len(store.paid) != 1 || store.paid[0].Now <= 0 {
		t.Fatalf("paid = %+v", store.paid)
	}
}
