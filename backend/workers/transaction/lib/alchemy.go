package lib

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"
)

const defaultAlchemyTimeout = 15 * time.Second

// AlchemyFetcher calls Alchemy's alchemy_getAssetTransfers JSON-RPC method.
type AlchemyFetcher struct {
	// APIKey is the Alchemy key appended to the endpoint path.
	APIKey string
	// Client defaults to an http.Client with a 15s timeout.
	Client *http.Client
	// BaseURL overrides the network host, for example https://host/v2. Tests
	// point this at an httptest server.
	BaseURL string
}

func NewAlchemyFetcher(apiKey string, client *http.Client) *AlchemyFetcher {
	return &AlchemyFetcher{APIKey: apiKey, Client: client}
}

func (f *AlchemyFetcher) httpClient() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: defaultAlchemyTimeout}
}

func (f *AlchemyFetcher) endpoint(network string) (string, error) {
	cfg, ok := supportedNetworks[network]
	if !ok {
		return "", fmt.Errorf("unsupported network: %s", network)
	}
	apiKey := strings.TrimSpace(f.APIKey)
	if apiKey == "" {
		return "", fmt.Errorf("ALCHEMY_API_KEY is required")
	}

	base := strings.TrimSpace(f.BaseURL)
	if base == "" {
		base = fmt.Sprintf("https://%s.g.alchemy.com/v2", cfg.alchemyNetwork)
	}
	return strings.TrimRight(base, "/") + "/" + apiKey, nil
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
	ID      int    `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type transfer struct {
	Hash     string `json:"hash"`
	From     string `json:"from"`
	To       string `json:"to"`
	Asset    string `json:"asset"`
	Value    any    `json:"value"`
	BlockNum string `json:"blockNum"`
	Metadata struct {
		BlockTimestamp string `json:"blockTimestamp"`
	} `json:"metadata"`
	RawContract struct {
		Address string `json:"address"`
		Value   string `json:"value"`
		Decimal string `json:"decimal"`
	} `json:"rawContract"`
}

type transferResult struct {
	Transfers []transfer `json:"transfers"`
}

type transferResponse struct {
	Result transferResult `json:"result"`
	Error  *rpcError      `json:"error"`
}

// batchResponse is one entry of a JSON-RPC batch response.
type batchResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

// FetchAssetTransfers fetches external and ERC-20 transfers for a wallet.
func (f *AlchemyFetcher) FetchAssetTransfers(ctx context.Context, network, address, direction string, fromBlock uint64) ([]TransactionItem, error) {
	return f.fetchAssetTransfers(ctx, network, address, direction, fromBlock, nil)
}

// FetchStockTransfers asks Alchemy for the tracked Base stock contracts.
// Those tokens are often omitted from an unfiltered erc20 transfer query.
func (f *AlchemyFetcher) FetchStockTransfers(ctx context.Context, network, address, direction string, fromBlock uint64) ([]TransactionItem, error) {
	return f.fetchAssetTransfers(ctx, network, address, direction, fromBlock, baseStockContracts())
}

func assetTransferFilter(address, direction string, fromBlock uint64, contracts []string) map[string]any {
	filter := map[string]any{
		"fromBlock":        "0x0",
		"category":         []string{"external", "erc20"},
		"withMetadata":     true,
		"excludeZeroValue": true,
		"maxCount":         "0x3e8",
		"order":            "asc",
	}
	if fromBlock > 0 {
		filter["fromBlock"] = fmt.Sprintf("0x%x", fromBlock)
	}
	if direction == "credit" {
		filter["toAddress"] = address
	} else {
		filter["fromAddress"] = address
	}
	if len(contracts) > 0 {
		filter["contractAddresses"] = contracts
		filter["category"] = []string{"erc20"}
	}
	return filter
}

func (f *AlchemyFetcher) fetchAssetTransfers(ctx context.Context, network, address, direction string, fromBlock uint64, contracts []string) ([]TransactionItem, error) {
	endpoint, err := f.endpoint(network)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		Method:  "alchemy_getAssetTransfers",
		Params:  []any{assetTransferFilter(address, direction, fromBlock, contracts)},
		ID:      1,
	})
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("alchemy HTTP %d", resp.StatusCode)
	}

	var parsed transferResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("alchemy error %d: %s", parsed.Error.Code, parsed.Error.Message)
	}

	return transferItems(network, address, direction, parsed.Result.Transfers), nil
}

func transferItems(network, address, direction string, transfers []transfer) []TransactionItem {
	items := make([]TransactionItem, 0, len(transfers))
	for _, tr := range transfers {
		decimalHex := decimalsHexForToken(tr.RawContract.Address, tr.RawContract.Decimal)
		items = append(items, TransactionItem{
			WalletAddress: strings.ToLower(address),
			TxHash:        strings.ToLower(strings.TrimSpace(tr.Hash)),
			FromAddress:   strings.ToLower(strings.TrimSpace(tr.From)),
			ToAddress:     strings.ToLower(strings.TrimSpace(tr.To)),
			TokenAddress:  strings.ToLower(strings.TrimSpace(tr.RawContract.Address)),
			TokenType:     TokenTypeFromAddress(tr.RawContract.Address),
			TokenSymbol:   tokenSymbol(tr.RawContract.Address, tr.Asset),
			Amount:        ParseTransferAmount(tr.Value, tr.RawContract.Value, decimalHex),
			Network:       network,
			Direction:     direction,
			State:         "completed",
			BlockNumber:   ParseBlockNumber(tr.BlockNum),
			TimestampMs:   ParseBlockTimestamp(tr.Metadata.BlockTimestamp),
		})
	}
	return items
}

// FetchWalletTransfers fetches credits and debits (including tracked stock
// tokens) with a single batched JSON-RPC request. One subrequest per wallet
// keeps the worker well below the Workers Free limit of 50 subrequests per
// invocation; four separate calls per wallet only scaled to about twelve
// wallets before the run was cut off.
func (f *AlchemyFetcher) FetchWalletTransfers(ctx context.Context, network, address string, fromBlock uint64) ([]TransactionItem, error) {
	endpoint, err := f.endpoint(network)
	if err != nil {
		return nil, err
	}

	type transferQuery struct {
		id        int
		direction string
		contracts []string
	}
	queries := []transferQuery{
		{id: 1, direction: "credit"},
		{id: 2, direction: "debit"},
		{id: 3, direction: "credit", contracts: baseStockContracts()},
		{id: 4, direction: "debit", contracts: baseStockContracts()},
	}

	requests := make([]rpcRequest, 0, len(queries))
	for _, query := range queries {
		requests = append(requests, rpcRequest{
			JSONRPC: "2.0",
			Method:  "alchemy_getAssetTransfers",
			Params:  []any{assetTransferFilter(address, query.direction, fromBlock, query.contracts)},
			ID:      query.id,
		})
	}

	responses, err := f.callRPCBatch(ctx, endpoint, requests)
	if err != nil {
		return nil, err
	}

	groups := make([][]TransactionItem, 0, len(queries))
	assetFailures := 0
	for _, query := range queries {
		response, ok := responses[query.id]
		if !ok {
			return nil, fmt.Errorf("missing batch response for %s/%s", query.direction, transferQueryLabel(query.contracts))
		}
		if response.Error != nil {
			if len(query.contracts) > 0 {
				// Stock transfers are supplementary. Their failure must not
				// discard the regular asset transfers in the same batch.
				log.Printf("[alchemy] stock %s transfers for %s: %d %s",
					query.direction, address, response.Error.Code, response.Error.Message)
				continue
			}
			assetFailures++
			log.Printf("[alchemy] %s transfers for %s: %d %s",
				query.direction, address, response.Error.Code, response.Error.Message)
			continue
		}

		var result transferResult
		if err := json.Unmarshal(response.Result, &result); err != nil {
			return nil, fmt.Errorf("decode %s transfers: %w", query.direction, err)
		}
		groups = append(groups, transferItems(network, address, query.direction, result.Transfers))
	}

	if assetFailures == 2 {
		return nil, fmt.Errorf("alchemy asset transfers failed for both directions")
	}
	return MergeTransfers(groups...), nil
}

func transferQueryLabel(contracts []string) string {
	if len(contracts) > 0 {
		return "stocks"
	}
	return "assets"
}

// callRPCBatch sends several JSON-RPC calls in one HTTP request and returns
// the per-call responses so callers can tolerate individual failures.
func (f *AlchemyFetcher) callRPCBatch(ctx context.Context, endpoint string, reqs []rpcRequest) (map[int]batchResponse, error) {
	body, err := json.Marshal(reqs)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("alchemy HTTP %d", resp.StatusCode)
	}

	var parsed []batchResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode batch response: %w", err)
	}

	out := make(map[int]batchResponse, len(parsed))
	for _, item := range parsed {
		out[item.ID] = item
	}
	return out, nil
}

func TokenTypeFromAddress(tokenAddress string) string {
	if strings.TrimSpace(tokenAddress) == "" {
		return "native"
	}
	return "erc20"
}

func ParseBlockTimestamp(ts string) int64 {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return time.Now().UnixMilli()
	}
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Now().UnixMilli()
	}
	return parsed.UnixMilli()
}

func ParseBlockNumber(hexValue string) uint64 {
	hexValue = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(hexValue), "0x"))
	if hexValue == "" {
		return 0
	}
	value := new(big.Int)
	if _, ok := value.SetString(hexValue, 16); !ok {
		return 0
	}
	return value.Uint64()
}

func ParseTransferAmount(value any, rawHex, decimalHex string) string {
	switch v := value.(type) {
	case float64:
		return strings.TrimSpace(big.NewFloat(v).Text('f', 8))
	case string:
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}

	rawHex = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rawHex), "0x"))
	if rawHex == "" {
		return "0"
	}
	raw := new(big.Int)
	if _, ok := raw.SetString(rawHex, 16); !ok {
		return "0"
	}
	decimals := ParseBlockNumber(decimalHex)
	if decimals == 0 {
		return raw.String()
	}

	amount := new(big.Float).SetInt(raw)
	divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	amount.Quo(amount, divisor)
	return amount.Text('f', int(decimals))
}
