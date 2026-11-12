package lib

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"
)

const defaultAlchemyBaseURL = "https://%s.g.alchemy.com"

// BalanceFetcher reads on-chain balances for a wallet.
type BalanceFetcher interface {
	FetchAllBalances(ctx context.Context, wallet Wallet, networks map[string]NetworkConfig, tokens map[string][]TokenConfig) ([]Balance, error)
}

// AlchemyFetcher fetches native and ERC-20 balances through Alchemy's
// JSON-RPC endpoint.
type AlchemyFetcher struct {
	APIKey string
	Client *http.Client
	// BaseURL is the Alchemy host template. It defaults to
	// "https://%s.g.alchemy.com" and may omit %s to point at a test server.
	BaseURL string
}

func NewAlchemyFetcher(apiKey string) *AlchemyFetcher {
	return &AlchemyFetcher{APIKey: apiKey}
}

// rpcRequest is a standard JSON-RPC 2.0 request body.
type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

// rpcResponse is the envelope returned by Alchemy for all RPC calls. Result is
// kept raw because batch calls return different shapes per method.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// tokenBalanceResult is the result shape of alchemy_getTokenBalances.
type tokenBalanceResult struct {
	Address       string `json:"address"`
	TokenBalances []struct {
		ContractAddress string `json:"contractAddress"`
		TokenBalance    string `json:"tokenBalance"`
	} `json:"tokenBalances"`
}

func (f *AlchemyFetcher) endpoint(alchemyNetwork string) (string, error) {
	if strings.TrimSpace(f.APIKey) == "" {
		return "", fmt.Errorf("ALCHEMY_API_KEY is not configured")
	}
	base := strings.TrimSpace(f.BaseURL)
	if base == "" {
		base = defaultAlchemyBaseURL
	}
	if strings.Contains(base, "%s") {
		base = fmt.Sprintf(base, alchemyNetwork)
	}
	return strings.TrimRight(base, "/") + "/v2/" + f.APIKey, nil
}

func (f *AlchemyFetcher) httpClient() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// callRPC sends a JSON-RPC request to Alchemy and returns the result string.
func (f *AlchemyFetcher) callRPC(ctx context.Context, endpoint string, req rpcRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal rpc request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient().Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("alchemy returned status %d", resp.StatusCode)
	}

	var rpcResp rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return "", fmt.Errorf("decode rpc response: %w", err)
	}

	if rpcResp.Error != nil {
		return "", fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	if len(rpcResp.Result) == 0 {
		return "", fmt.Errorf("rpc response missing result")
	}

	var result string
	if err := json.Unmarshal(rpcResp.Result, &result); err != nil {
		return "", fmt.Errorf("decode rpc result: %w", err)
	}
	return result, nil
}

// callRPCBatch sends several JSON-RPC calls in a single HTTP request. Alchemy
// counts a batch as one subrequest, which keeps the worker below the 50
// subrequests per invocation limit on the Workers Free plan.
func (f *AlchemyFetcher) callRPCBatch(ctx context.Context, endpoint string, reqs []rpcRequest) (map[int]json.RawMessage, error) {
	body, err := json.Marshal(reqs)
	if err != nil {
		return nil, fmt.Errorf("marshal rpc batch: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alchemy returned status %d", resp.StatusCode)
	}

	var rpcResps []rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResps); err != nil {
		return nil, fmt.Errorf("decode rpc batch response: %w", err)
	}

	results := make(map[int]json.RawMessage, len(rpcResps))
	for _, rpcResp := range rpcResps {
		if rpcResp.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
		}
		results[rpcResp.ID] = rpcResp.Result
	}
	return results, nil
}

// hexToDecimal converts a 0x-prefixed hex string to a decimal string,
// dividing by 10^decimals to produce a human-readable token amount.
func hexToDecimal(hexStr string, decimals int) (string, error) {
	hexStr = strings.TrimPrefix(strings.TrimSpace(hexStr), "0x")
	if hexStr == "" {
		hexStr = "0"
	}

	wei := new(big.Int)
	if _, ok := wei.SetString(hexStr, 16); !ok {
		return "", fmt.Errorf("invalid hex value: %q", hexStr)
	}

	if decimals <= 0 {
		return wei.String(), nil
	}

	divisor := new(big.Float).SetInt(new(big.Int).Exp(
		big.NewInt(10),
		big.NewInt(int64(decimals)),
		nil,
	))
	amount := new(big.Float).SetInt(wei)
	amount.Quo(amount, divisor)

	return amount.Text('f', decimals), nil
}

// FetchNativeBalance fetches the native token balance (ETH, XDAI, etc.)
// for the given wallet.
func (f *AlchemyFetcher) FetchNativeBalance(ctx context.Context, wallet Wallet, networks map[string]NetworkConfig) (Balance, error) {
	cfg, networkKey, err := resolveNetworkWithRegistry(wallet.Network, wallet.Testnet, networks)
	if err != nil {
		return Balance{}, fmt.Errorf("resolve network: %w", err)
	}

	endpoint, err := f.endpoint(cfg.AlchemyNetwork)
	if err != nil {
		return Balance{}, err
	}

	result, err := f.callRPC(ctx, endpoint, rpcRequest{
		JSONRPC: "2.0",
		Method:  "eth_getBalance",
		Params:  []interface{}{wallet.Address, "latest"},
		ID:      1,
	})
	if err != nil {
		return Balance{}, fmt.Errorf("eth_getBalance: %w", err)
	}

	amount, err := hexToDecimal(result, 18)
	if err != nil {
		return Balance{}, fmt.Errorf("convert native balance: %w", err)
	}

	return Balance{
		WalletID:     wallet.ID,
		Address:      wallet.Address,
		Network:      networkKey,
		TokenType:    "native",
		TokenSymbol:  cfg.NativeSymbol,
		TokenAddress: "",
		Amount:       amount,
		FetchedAt:    time.Now().UnixMilli(),
	}, nil
}

// balanceOfData builds the eth_call data payload for ERC-20 balanceOf(address).
// 0x70a08231 is the fixed 4-byte selector for balanceOf(address).
func balanceOfData(walletAddress string) string {
	addr := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(walletAddress)), "0x")
	padded := addr
	if len(padded) < 64 {
		padded = strings.Repeat("0", 64-len(padded)) + padded
	}
	return "0x70a08231" + padded
}

// FetchERC20Balance fetches the balance of a specific ERC-20 token.
func (f *AlchemyFetcher) FetchERC20Balance(ctx context.Context, wallet Wallet, token TokenConfig, networks map[string]NetworkConfig) (Balance, error) {
	cfg, networkKey, err := resolveNetworkWithRegistry(wallet.Network, wallet.Testnet, networks)
	if err != nil {
		return Balance{}, fmt.Errorf("resolve network: %w", err)
	}

	endpoint, err := f.endpoint(cfg.AlchemyNetwork)
	if err != nil {
		return Balance{}, err
	}

	result, err := f.callRPC(ctx, endpoint, rpcRequest{
		JSONRPC: "2.0",
		Method:  "eth_call",
		Params: []interface{}{
			map[string]string{
				"to":   token.Contract,
				"data": balanceOfData(wallet.Address),
			},
			"latest",
		},
		ID: 1,
	})
	if err != nil {
		return Balance{}, fmt.Errorf("eth_call balanceOf: %w", err)
	}

	amount, err := hexToDecimal(result, token.Decimals)
	if err != nil {
		return Balance{}, fmt.Errorf("convert erc20 balance: %w", err)
	}

	return Balance{
		WalletID:     wallet.ID,
		Address:      wallet.Address,
		Network:      networkKey,
		TokenType:    "erc20",
		TokenSymbol:  token.Symbol,
		TokenAddress: token.Contract,
		Amount:       amount,
		FetchedAt:    time.Now().UnixMilli(),
	}, nil
}

// FetchAllBalances fetches the native balance plus every registered ERC-20
// balance for the wallet's network in one batched JSON-RPC request.
//
// One subrequest per wallet matters: Workers Free allows 50 subrequests per
// invocation. Fetching each token separately burned that budget after two
// wallets and silently stopped the run, leaving later wallets with no rows.
func (f *AlchemyFetcher) FetchAllBalances(
	ctx context.Context,
	wallet Wallet,
	networks map[string]NetworkConfig,
	tokenMap map[string][]TokenConfig,
) ([]Balance, error) {
	cfg, networkKey, err := resolveNetworkWithRegistry(wallet.Network, wallet.Testnet, networks)
	if err != nil {
		return nil, fmt.Errorf("resolve network: %w", err)
	}

	endpoint, err := f.endpoint(cfg.AlchemyNetwork)
	if err != nil {
		return nil, err
	}

	tokens := uniqueTokens(tokenMap[networkKey])
	contracts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		contracts = append(contracts, token.Contract)
	}

	requests := []rpcRequest{{
		JSONRPC: "2.0",
		Method:  "eth_getBalance",
		Params:  []interface{}{wallet.Address, "latest"},
		ID:      nativeBalanceRequestID,
	}}
	if len(contracts) > 0 {
		requests = append(requests, rpcRequest{
			JSONRPC: "2.0",
			Method:  "alchemy_getTokenBalances",
			Params:  []interface{}{wallet.Address, contracts},
			ID:      tokenBalancesRequestID,
		})
	}

	results, err := f.callRPCBatch(ctx, endpoint, requests)
	if err != nil {
		return nil, fmt.Errorf("alchemy balance batch: %w", err)
	}

	rawNative, ok := results[nativeBalanceRequestID]
	if !ok {
		return nil, fmt.Errorf("native balance missing from batch response")
	}
	var nativeHex string
	if err := json.Unmarshal(rawNative, &nativeHex); err != nil {
		return nil, fmt.Errorf("decode native balance: %w", err)
	}
	nativeAmount, err := hexToDecimal(nativeHex, 18)
	if err != nil {
		return nil, fmt.Errorf("convert native balance: %w", err)
	}

	balances := make([]Balance, 0, 1+len(tokens))
	native := Balance{
		WalletID:     wallet.ID,
		Address:      wallet.Address,
		Network:      networkKey,
		TokenType:    "native",
		TokenSymbol:  cfg.NativeSymbol,
		TokenAddress: "",
		Amount:       nativeAmount,
		FetchedAt:    time.Now().UnixMilli(),
	}
	// Guard: if the native balance has empty fields, the Alchemy call silently
	// returned zero-value data - treat this as a hard error rather than writing
	// empty rows.
	if native.Amount == "" || native.TokenSymbol == "" || native.Network == "" {
		return nil, fmt.Errorf("native balance returned empty fields for wallet %s (amount=%q symbol=%q network=%q)",
			wallet.ID, native.Amount, native.TokenSymbol, native.Network)
	}
	balances = append(balances, native)

	amounts := map[string]string{}
	if len(contracts) > 0 {
		rawTokens, ok := results[tokenBalancesRequestID]
		if !ok {
			return nil, fmt.Errorf("token balances missing from batch response")
		}
		var tokenResult tokenBalanceResult
		if err := json.Unmarshal(rawTokens, &tokenResult); err != nil {
			return nil, fmt.Errorf("decode token balances: %w", err)
		}
		for _, tokenBalance := range tokenResult.TokenBalances {
			key := strings.ToLower(strings.TrimSpace(tokenBalance.ContractAddress))
			if key == "" {
				continue
			}
			amounts[key] = tokenBalance.TokenBalance
		}
	}

	for _, token := range tokens {
		rawAmount := amounts[strings.ToLower(token.Contract)]
		if strings.TrimSpace(rawAmount) == "" {
			rawAmount = "0x0"
		}
		amount, err := hexToDecimal(rawAmount, token.Decimals)
		if err != nil {
			log.Printf("[alchemy] skipping %s on %s for wallet %s: %v",
				token.Symbol, networkKey, wallet.ID, err)
			continue
		}
		balances = append(balances, Balance{
			WalletID:     wallet.ID,
			Address:      wallet.Address,
			Network:      networkKey,
			TokenType:    "erc20",
			TokenSymbol:  token.Symbol,
			TokenAddress: token.Contract,
			Amount:       amount,
			FetchedAt:    time.Now().UnixMilli(),
		})
	}

	return balances, nil
}

const (
	nativeBalanceRequestID = 1
	tokenBalancesRequestID = 2
)

// uniqueTokens drops duplicate and malformed contract entries so Alchemy
// cannot reject the whole batch because of one bad registry row.
func uniqueTokens(tokens []TokenConfig) []TokenConfig {
	seen := map[string]bool{}
	out := make([]TokenConfig, 0, len(tokens))
	for _, token := range tokens {
		contract := strings.TrimSpace(token.Contract)
		key := strings.ToLower(contract)
		if seen[key] || !validContractAddress(contract) {
			continue
		}
		seen[key] = true
		token.Contract = contract
		out = append(out, token)
	}
	return out
}

func validContractAddress(address string) bool {
	trimmed := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(address)), "0x")
	if len(trimmed) != 40 {
		return false
	}
	_, err := hex.DecodeString(trimmed)
	return err == nil
}

func resolveNetworkWithRegistry(network string, testnet bool, networks map[string]NetworkConfig) (NetworkConfig, string, error) {
	normalized := strings.ToLower(strings.TrimSpace(network))

	// Accept canonical network keys directly (e.g. "eth-sepolia").
	if cfg, ok := networks[normalized]; ok {
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

	cfg, ok := networks[key]
	if !ok {
		return NetworkConfig{}, "", fmt.Errorf("network key %q not found in registry", key)
	}
	return cfg, key, nil
}
