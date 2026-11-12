package lib

import (
	"fmt"
	"regexp"
	"strings"
)

// evmAddressPattern matches a 0x-prefixed 20-byte address. Wallet addresses are
// stored lowercased so they match the worker keys and Firestore document ids.
var evmAddressPattern = regexp.MustCompile(`^0x[0-9a-f]{40}$`)

type networkInfo struct {
	canonical string
	testnet   bool
}

// networks maps accepted network names to the canonical keys understood by
// workers/balance and workers/transaction.
var networks = map[string]networkInfo{
	"base":           {canonical: "base-mainnet"},
	"base-mainnet":   {canonical: "base-mainnet"},
	"base-sepolia":   {canonical: "base-sepolia", testnet: true},
	"eth":            {canonical: "eth-mainnet"},
	"ethereum":       {canonical: "eth-mainnet"},
	"eth-mainnet":    {canonical: "eth-mainnet"},
	"eth-sepolia":    {canonical: "eth-sepolia", testnet: true},
	"gnosis":         {canonical: "gnosis-mainnet"},
	"xdai":           {canonical: "gnosis-mainnet"},
	"gnosis-mainnet": {canonical: "gnosis-mainnet"},
	"gnosis-chiado":  {canonical: "gnosis-chiado", testnet: true},
}

func normalizeAddress(raw string) (string, error) {
	address := strings.ToLower(strings.TrimSpace(raw))
	if !evmAddressPattern.MatchString(address) {
		return "", fmt.Errorf("address must be a 0x-prefixed 40-character hex string")
	}
	return address, nil
}

// normalizeNetwork returns the canonical network key and whether it is a
// testnet. An empty network defaults to Base mainnet.
func normalizeNetwork(raw string) (string, bool, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		key = "base-mainnet"
	}
	info, ok := networks[key]
	if !ok {
		return "", false, fmt.Errorf("unsupported network %q", raw)
	}
	return info.canonical, info.testnet, nil
}

// supportedCurrencies are the display currencies the app exposes.
var supportedCurrencies = map[string]struct{}{
	"USD": {},
	"ZAR": {},
	"EUR": {},
}

// defaultCurrency is used when a wallet has no saved preference.
const defaultCurrency = "USD"

// normalizeCurrency uppercases and validates a display currency.
func normalizeCurrency(raw string) (string, error) {
	currency := strings.ToUpper(strings.TrimSpace(raw))
	if _, ok := supportedCurrencies[currency]; !ok {
		return "", fmt.Errorf("currency must be one of USD, ZAR, or EUR")
	}
	return currency, nil
}
