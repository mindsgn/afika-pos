package lib

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type capturedRPC struct {
	path string
	req  rpcRequest
}

func newAlchemyServer(t *testing.T, response string, status int) (*httptest.Server, *capturedRPC) {
	t.Helper()

	captured := &capturedRPC{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.path = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured.req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)
	return server, captured
}

func TestFetchAssetTransfers(t *testing.T) {
	response := `{
		"result": {
			"transfers": [
				{
					"hash": "0xABC",
					"from": "0xDEF",
					"to": "0x111",
					"asset": "usdc",
					"value": "10.5",
					"blockNum": "0x64",
					"metadata": {"blockTimestamp": "2024-01-02T03:04:05Z"},
					"rawContract": {"address": "0x8335", "value": "0x0", "decimal": "0x6"}
				},
				{
					"hash": "0xDEF",
					"from": "0x222",
					"to": "0xABC",
					"asset": "eth",
					"value": 1.25,
					"blockNum": "0x65",
					"metadata": {"blockTimestamp": "2024-01-02T04:00:00Z"},
					"rawContract": {"address": "", "value": "0x0", "decimal": "0x12"}
				}
			]
		}
	}`
	server, captured := newAlchemyServer(t, response, http.StatusOK)

	fetcher := &AlchemyFetcher{APIKey: "test-key", BaseURL: server.URL}
	items, err := fetcher.FetchAssetTransfers(context.Background(), "base-mainnet", "0xABC", "credit", 10)
	if err != nil {
		t.Fatalf("FetchAssetTransfers: %v", err)
	}
	if captured.path != "/test-key" {
		t.Fatalf("path = %q, want /test-key", captured.path)
	}
	if captured.req.Method != "alchemy_getAssetTransfers" {
		t.Fatalf("method = %q", captured.req.Method)
	}

	filter, ok := captured.req.Params[0].(map[string]any)
	if !ok {
		t.Fatalf("params = %#v", captured.req.Params)
	}
	if filter["toAddress"] != "0xABC" {
		t.Fatalf("toAddress = %#v", filter["toAddress"])
	}
	if filter["fromBlock"] != "0xa" {
		t.Fatalf("fromBlock = %#v", filter["fromBlock"])
	}
	if _, ok := filter["contractAddresses"]; ok {
		t.Fatal("unfiltered query should not set contractAddresses")
	}

	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}

	first := items[0]
	if first.WalletAddress != "0xabc" || first.TxHash != "0xabc" {
		t.Fatalf("wallet/hash = %q/%q", first.WalletAddress, first.TxHash)
	}
	if first.FromAddress != "0xdef" || first.ToAddress != "0x111" {
		t.Fatalf("addresses = %q/%q", first.FromAddress, first.ToAddress)
	}
	if first.TokenAddress != "0x8335" || first.TokenType != "erc20" || first.TokenSymbol != "USDC" {
		t.Fatalf("token = %+v", first)
	}
	if first.Amount != "10.5" {
		t.Fatalf("amount = %q", first.Amount)
	}
	if first.BlockNumber != 100 || first.Direction != "credit" || first.State != "completed" {
		t.Fatalf("item = %+v", first)
	}
	if first.Network != "base-mainnet" {
		t.Fatalf("network = %q", first.Network)
	}

	second := items[1]
	if second.TokenType != "native" || second.TokenSymbol != "ETH" {
		t.Fatalf("native token = %+v", second)
	}
	if second.Amount != "1.25000000" {
		t.Fatalf("native amount = %q", second.Amount)
	}
	if second.BlockNumber != 101 {
		t.Fatalf("block = %d", second.BlockNumber)
	}
}

func newBatchTransferServer(t *testing.T, results map[int]string, failures map[int]bool) (*httptest.Server, *int) {
	t.Helper()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var reqs []rpcRequest
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &reqs); err != nil {
			t.Errorf("decode batch: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if len(reqs) != 4 {
			t.Errorf("batch size = %d, want 4", len(reqs))
		}

		responses := make([]map[string]any, 0, len(reqs))
		for _, req := range reqs {
			if failures[req.ID] {
				responses = append(responses, map[string]any{
					"jsonrpc": "2.0", "id": req.ID,
					"error": map[string]any{"code": -32000, "message": "boom"},
				})
				continue
			}
			result, ok := results[req.ID]
			if !ok {
				t.Errorf("unexpected batch id %d", req.ID)
				http.Error(w, "unexpected", http.StatusInternalServerError)
				return
			}
			responses = append(responses, map[string]any{
				"jsonrpc": "2.0", "id": req.ID, "result": json.RawMessage(result),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(responses)
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func TestFetchWalletTransfersBatchesRequests(t *testing.T) {
	credit := `{"transfers":[{"hash":"0xC1","from":"0x111","to":"0xABC","asset":"usdc","value":"2.5","blockNum":"0x10","metadata":{"blockTimestamp":"2024-01-02T03:04:05Z"},"rawContract":{"address":"0x8335","value":"0x0","decimal":"0x6"}}]}`
	debit := `{"transfers":[{"hash":"0xD1","from":"0xABC","to":"0x222","asset":"eth","value":0.5,"blockNum":"0x11","metadata":{"blockTimestamp":"2024-01-02T04:00:00Z"},"rawContract":{"address":"","value":"0x0","decimal":"0x12"}}]}`
	empty := `{"transfers":[]}`

	server, requests := newBatchTransferServer(t, map[int]string{1: credit, 2: debit, 3: empty, 4: empty}, nil)

	fetcher := &AlchemyFetcher{APIKey: "test-key", BaseURL: server.URL}
	items, err := fetcher.FetchWalletTransfers(context.Background(), "base-mainnet", "0xABC", 5)
	if err != nil {
		t.Fatalf("FetchWalletTransfers: %v", err)
	}
	if *requests != 1 {
		t.Fatalf("http requests = %d, want 1 batched request", *requests)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2 (%+v)", len(items), items)
	}

	creditItem, ok := findItem(items, "0xc1", "credit")
	if !ok || creditItem.Amount != "2.5" || creditItem.TokenSymbol != "USDC" {
		t.Fatalf("credit item = %+v", creditItem)
	}
	debitItem, ok := findItem(items, "0xd1", "debit")
	if !ok || debitItem.TokenSymbol != "ETH" || debitItem.Amount != "0.50000000" {
		t.Fatalf("debit item = %+v", debitItem)
	}
}

func TestFetchWalletTransfersToleratesStockFailure(t *testing.T) {
	credit := `{"transfers":[{"hash":"0xC1","from":"0x111","to":"0xABC","asset":"usdc","value":"2.5","blockNum":"0x10","metadata":{"blockTimestamp":"2024-01-02T03:04:05Z"},"rawContract":{"address":"0x8335","value":"0x0","decimal":"0x6"}}]}`
	empty := `{"transfers":[]}`
	server, _ := newBatchTransferServer(t, map[int]string{1: credit, 2: empty}, map[int]bool{3: true, 4: true})

	fetcher := &AlchemyFetcher{APIKey: "test-key", BaseURL: server.URL}
	items, err := fetcher.FetchWalletTransfers(context.Background(), "base-mainnet", "0xABC", 0)
	if err != nil {
		t.Fatalf("stock failure should not abort: %v", err)
	}
	if len(items) != 1 || items[0].TxHash != "0xc1" {
		t.Fatalf("items = %+v, want the credit transfer", items)
	}
}

func TestFetchWalletTransfersBothAssetDirectionsFail(t *testing.T) {
	empty := `{"transfers":[]}`
	server, _ := newBatchTransferServer(t, map[int]string{3: empty, 4: empty}, map[int]bool{1: true, 2: true})

	fetcher := &AlchemyFetcher{APIKey: "test-key", BaseURL: server.URL}
	if _, err := fetcher.FetchWalletTransfers(context.Background(), "base-mainnet", "0xABC", 0); err == nil {
		t.Fatal("expected error when both asset directions fail")
	}
}

func findItem(items []TransactionItem, hash, direction string) (TransactionItem, bool) {
	for _, item := range items {
		if item.TxHash == hash && item.Direction == direction {
			return item, true
		}
	}
	return TransactionItem{}, false
}

func TestFetchStockTransfers(t *testing.T) {
	server, captured := newAlchemyServer(t, `{"result":{"transfers":[]}}`, http.StatusOK)

	fetcher := &AlchemyFetcher{APIKey: "test-key", BaseURL: server.URL}
	if _, err := fetcher.FetchStockTransfers(context.Background(), "base-mainnet", "0xabc", "debit", 0); err != nil {
		t.Fatalf("FetchStockTransfers: %v", err)
	}

	filter, ok := captured.req.Params[0].(map[string]any)
	if !ok {
		t.Fatalf("params = %#v", captured.req.Params)
	}
	category, ok := filter["category"].([]any)
	if !ok || len(category) != 1 || category[0] != "erc20" {
		t.Fatalf("category = %#v", filter["category"])
	}
	contracts, ok := filter["contractAddresses"].([]any)
	if !ok || len(contracts) != len(baseStocks) {
		t.Fatalf("contracts = %#v", filter["contractAddresses"])
	}
	if filter["fromAddress"] != "0xabc" {
		t.Fatalf("fromAddress = %#v", filter["fromAddress"])
	}
	if filter["fromBlock"] != "0x0" {
		t.Fatalf("fromBlock = %#v", filter["fromBlock"])
	}
}

func TestAlchemyRPCError(t *testing.T) {
	server, _ := newAlchemyServer(t, `{"error":{"code":-32000,"message":"boom"}}`, http.StatusOK)

	fetcher := &AlchemyFetcher{APIKey: "test-key", BaseURL: server.URL}
	_, err := fetcher.FetchAssetTransfers(context.Background(), "base-mainnet", "0xabc", "credit", 0)
	if err == nil || err.Error() != "alchemy error -32000: boom" {
		t.Fatalf("err = %v", err)
	}
}

func TestAlchemyHTTPError(t *testing.T) {
	server, _ := newAlchemyServer(t, `upstream`, http.StatusInternalServerError)

	fetcher := &AlchemyFetcher{APIKey: "test-key", BaseURL: server.URL}
	if _, err := fetcher.FetchAssetTransfers(context.Background(), "base-mainnet", "0xabc", "credit", 0); err == nil {
		t.Fatal("expected HTTP error")
	}
}

func TestAlchemyInvalidJSON(t *testing.T) {
	server, _ := newAlchemyServer(t, `not-json`, http.StatusOK)

	fetcher := &AlchemyFetcher{APIKey: "test-key", BaseURL: server.URL}
	if _, err := fetcher.FetchAssetTransfers(context.Background(), "base-mainnet", "0xabc", "credit", 0); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestAlchemyValidation(t *testing.T) {
	tests := []struct {
		name    string
		fetcher *AlchemyFetcher
		network string
	}{
		{name: "missing api key", fetcher: &AlchemyFetcher{}, network: "base-mainnet"},
		{name: "unsupported network", fetcher: &AlchemyFetcher{APIKey: "test-key"}, network: "solana-mainnet"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.fetcher.FetchAssetTransfers(context.Background(), tc.network, "0xabc", "credit", 0); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestResolveNetwork(t *testing.T) {
	tests := []struct {
		name    string
		network string
		testnet bool
		want    string
		wantErr bool
	}{
		{name: "canonical", network: "base-mainnet", want: "base-mainnet"},
		{name: "base mainnet", network: "Base", want: "base-mainnet"},
		{name: "base testnet", network: "base", testnet: true, want: "base-sepolia"},
		{name: "eth", network: "ethereum", want: "eth-mainnet"},
		{name: "eth testnet", network: "eth", testnet: true, want: "eth-sepolia"},
		{name: "gnosis", network: "gnosis", want: "gnosis-mainnet"},
		{name: "xdai testnet", network: "xdai", testnet: true, want: "gnosis-chiado"},
		{name: "unsupported", network: "solana", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveNetwork(tc.network, tc.testnet)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveNetwork: %v", err)
			}
			if got != tc.want {
				t.Fatalf("network = %q, want %q", got, tc.want)
			}
		})
	}
}
