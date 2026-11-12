package lib

import (
	"fmt"
	"strings"
)

type NetworkConfig struct {
	AlchemyNetwork string
	NativeSymbol   string
	ChainID        int
}

type TokenConfig struct {
	Symbol   string
	Contract string
	Decimals int
}

var SupportedNetworks = map[string]NetworkConfig{
	// Mainnets
	"eth-mainnet":    {AlchemyNetwork: "eth-mainnet", NativeSymbol: "ETH", ChainID: 1},
	"base-mainnet":   {AlchemyNetwork: "base-mainnet", NativeSymbol: "ETH", ChainID: 8453},
	"gnosis-mainnet": {AlchemyNetwork: "gnosis-mainnet", NativeSymbol: "XDAI", ChainID: 100},

	// Testnets
	"eth-sepolia":   {AlchemyNetwork: "eth-sepolia", NativeSymbol: "ETH", ChainID: 11155111},
	"base-sepolia":  {AlchemyNetwork: "base-sepolia", NativeSymbol: "ETH", ChainID: 84532},
	"gnosis-chiado": {AlchemyNetwork: "gnosis-chiado", NativeSymbol: "XDAI", ChainID: 10200},
}

var ERC20Tokens = map[string][]TokenConfig{
	"eth-mainnet": {
		{Symbol: "USDC", Contract: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Decimals: 6},
		{Symbol: "DAI", Contract: "0x6B175474E89094C44Da98b954EedeAC495271d0F", Decimals: 18},
		{Symbol: "USDT", Contract: "0xdAC17F958D2ee523a2206206994597C13D831ec7", Decimals: 6},
	},
	"base-mainnet": append([]TokenConfig{
		{Symbol: "USDC", Contract: "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", Decimals: 6},
		{Symbol: "EURC", Contract: "0x60a3E35Cc302bFA44Cb288Bc5a4F316Fdb1adb42", Decimals: 6},
		{Symbol: "DAI", Contract: "0x50c5725949A6F0c72E6C4a641F24049A917DB0Cb", Decimals: 18},
		{Symbol: "ZARP", Contract: "0xb755506531786C8aC63B756BaB1ac387bACB0C04", Decimals: 18},
	}, BaseStockTokens...),
	"gnosis-mainnet": {
		{Symbol: "USDC", Contract: "0xDDAfbb505ad214D7b80b1f830fcCc89B60fb7A83", Decimals: 6},
		{Symbol: "DAI", Contract: "0x44fA8E6f47987339850636F88629646b4B4c6890", Decimals: 18},
	},
	// Testnets — smaller token lists
	"eth-sepolia": {
		{Symbol: "USDC", Contract: "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238", Decimals: 6},
	},
	"base-sepolia": {
		{Symbol: "USDC", Contract: "0x036CbD53842c5426634e7929541eC2318f3dCF7e", Decimals: 6},
	},
	"gnosis-chiado": {},
}

func ResolveNetwork(network string, testnet bool) (NetworkConfig, string, error) {
	normalized := strings.ToLower(strings.TrimSpace(network))

	// Accept canonical network keys directly.
	if cfg, ok := SupportedNetworks[normalized]; ok {
		return cfg, normalized, nil
	}

	var key string
	switch normalized {
	case "eth", "ethereum":
		if testnet {
			key = "eth-sepolia"
		} else {
			key = "eth-mainnet"
		}
	case "base":
		if testnet {
			key = "base-sepolia"
		} else {
			key = "base-mainnet"
		}
	case "gnosis", "xdai":
		if testnet {
			key = "gnosis-chiado"
		} else {
			key = "gnosis-mainnet"
		}
	default:
		return NetworkConfig{}, "", fmt.Errorf("unsupported network: %q", network)
	}

	cfg, ok := SupportedNetworks[key]
	if !ok {
		return NetworkConfig{}, "", fmt.Errorf("network key %q not found in SupportedNetworks", key)
	}
	return cfg, key, nil
}
