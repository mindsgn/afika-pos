package lib

import (
	"fmt"
	"strings"
)

type networkConfig struct {
	alchemyNetwork string
}

// supportedNetworks lists the networks this worker can query. It intentionally
// matches the old PM2 worker: only Base mainnet is enabled, even though
// ResolveNetwork knows the canonical names for the other chains.
var supportedNetworks = map[string]networkConfig{
	"base-mainnet": {alchemyNetwork: "base-mainnet"},
}

// ResolveNetwork maps a wallet's network value to an Alchemy network key.
func ResolveNetwork(network string, testnet bool) (string, error) {
	n := strings.ToLower(strings.TrimSpace(network))
	if _, ok := supportedNetworks[n]; ok {
		return n, nil
	}
	switch n {
	case "eth", "ethereum":
		if testnet {
			return "eth-sepolia", nil
		}
		return "eth-mainnet", nil
	case "base":
		if testnet {
			return "base-sepolia", nil
		}
		return "base-mainnet", nil
	case "gnosis", "xdai":
		if testnet {
			return "gnosis-chiado", nil
		}
		return "gnosis-mainnet", nil
	default:
		return "", fmt.Errorf("unsupported network: %q", network)
	}
}
