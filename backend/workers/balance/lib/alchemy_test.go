package lib

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newAlchemyServer(t *testing.T, results map[string]string, failures map[string]bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type = %q", got)
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		key := req.Method
		if req.Method == "eth_call" {
			params, ok := req.Params[0].(map[string]any)
			if !ok {
				t.Errorf("eth_call params = %#v", req.Params)
			}
			to, _ := params["to"].(string)
			key = strings.ToLower(to)
		}

		w.Header().Set("Content-Type", "application/json")
		if failures[key] {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"error":   map[string]any{"code": -32000, "message": "boom"},
			})
			return
		}

		result, ok := results[key]
		if !ok {
			t.Errorf("unexpected rpc call %q", key)
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Errorf("marshal result: %v", err)
			http.Error(w, "bad result", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: 1, Result: encoded})
	}))
}

const (
	testUSDC = "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
	testDAI  = "0x50c5725949A6F0c72E6C4a641F24049A917DB0Cb"
)

type batchCall struct {
	requests []rpcRequest
}

// newBatchAlchemyServer responds to one JSON-RPC batch containing the native
// balance and token balances calls, and records how many HTTP requests it saw.
func newBatchAlchemyServer(t *testing.T, native any, tokens tokenBalanceResult, nativeErr bool) (*httptest.Server, *int, *batchCall) {
	t.Helper()

	requests := 0
	captured := &batchCall{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if err := json.NewDecoder(r.Body).Decode(&captured.requests); err != nil {
			t.Errorf("decode batch: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		responses := make([]map[string]any, 0, len(captured.requests))
		for _, req := range captured.requests {
			switch req.Method {
			case "eth_getBalance":
				if nativeErr {
					responses = append(responses, map[string]any{
						"jsonrpc": "2.0", "id": req.ID,
						"error": map[string]any{"code": -32000, "message": "boom"},
					})
					continue
				}
				responses = append(responses, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": native})
			case "alchemy_getTokenBalances":
				responses = append(responses, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": tokens})
			default:
				t.Errorf("unexpected rpc method %q", req.Method)
				http.Error(w, "unexpected", http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(responses)
	}))
	t.Cleanup(server.Close)
	return server, &requests, captured
}

func tokenBalances(entries map[string]string) tokenBalanceResult {
	result := tokenBalanceResult{}
	for contract, amount := range entries {
		result.TokenBalances = append(result.TokenBalances, struct {
			ContractAddress string `json:"contractAddress"`
			TokenBalance    string `json:"tokenBalance"`
		}{ContractAddress: contract, TokenBalance: amount})
	}
	return result
}

func TestAlchemyFetcherFetchAllBalances(t *testing.T) {
	server, requests, captured := newBatchAlchemyServer(t,
		"0xde0b6b3a7640000", // 1 ETH
		tokenBalances(map[string]string{
			testUSDC: "0x989680", // 10 USDC with 6 decimals
			testDAI:  "0x0",
		}),
		false,
	)

	fetcher := &AlchemyFetcher{APIKey: "test-key", BaseURL: server.URL}
	wallet := Wallet{ID: "w1", Address: "0xabc", Network: "base"}
	tokens := map[string][]TokenConfig{
		"base-mainnet": {
			{Symbol: "USDC", Contract: testUSDC, Decimals: 6},
			{Symbol: "DAI", Contract: testDAI, Decimals: 18},
		},
	}

	got, err := fetcher.FetchAllBalances(context.Background(), wallet, CloneNetworkConfigMap(SupportedNetworks), tokens)
	if err != nil {
		t.Fatalf("FetchAllBalances: %v", err)
	}
	if *requests != 1 {
		t.Fatalf("http requests = %d, want 1 batched request", *requests)
	}
	if len(captured.requests) != 2 {
		t.Fatalf("rpc calls = %d, want 2 (native + token balances)", len(captured.requests))
	}
	if len(got) != 3 {
		t.Fatalf("balances = %d, want 3 (%+v)", len(got), got)
	}

	native := got[0]
	if native.TokenType != "native" || native.TokenSymbol != "ETH" || native.Network != "base-mainnet" {
		t.Fatalf("native = %+v", native)
	}
	if native.Amount != "1.000000000000000000" {
		t.Fatalf("native amount = %q", native.Amount)
	}
	if native.WalletID != "w1" || native.Address != "0xabc" {
		t.Fatalf("native identity = %+v", native)
	}

	usdc := got[1]
	if usdc.TokenType != "erc20" || usdc.TokenSymbol != "USDC" || usdc.TokenAddress != testUSDC {
		t.Fatalf("usdc = %+v", usdc)
	}
	if usdc.Amount != "10.000000" {
		t.Fatalf("usdc amount = %q", usdc.Amount)
	}

	dai := got[2]
	if dai.TokenSymbol != "DAI" || dai.Amount != "0.000000000000000000" {
		t.Fatalf("dai = %+v", dai)
	}
}

func TestAlchemyFetcherNativeFailureAbortsWallet(t *testing.T) {
	server, _, _ := newBatchAlchemyServer(t, nil, tokenBalances(map[string]string{testUSDC: "0x1"}), true)

	fetcher := &AlchemyFetcher{APIKey: "test-key", BaseURL: server.URL}
	wallet := Wallet{ID: "w1", Address: "0xabc", Network: "base"}

	if _, err := fetcher.FetchAllBalances(context.Background(), wallet, SupportedNetworks, nil); err == nil {
		t.Fatal("expected native balance error")
	}
}

func TestUniqueTokensSkipsInvalidContracts(t *testing.T) {
	got := uniqueTokens([]TokenConfig{
		{Symbol: "USDC", Contract: testUSDC, Decimals: 6},
		{Symbol: "USDC2", Contract: strings.ToLower(testUSDC), Decimals: 6},
		{Symbol: "BAD", Contract: "0xnothex", Decimals: 6},
		{Symbol: "EMPTY", Contract: "  ", Decimals: 6},
	})
	if len(got) != 1 || got[0].Symbol != "USDC" {
		t.Fatalf("uniqueTokens = %+v, want only USDC", got)
	}
}

func TestAlchemyFetcherRequiresAPIKey(t *testing.T) {
	fetcher := &AlchemyFetcher{}
	wallet := Wallet{ID: "w1", Address: "0xabc", Network: "base"}

	if _, err := fetcher.FetchNativeBalance(context.Background(), wallet, SupportedNetworks); err == nil {
		t.Fatal("expected missing API key error")
	}
}

func TestAlchemyFetcherHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	fetcher := &AlchemyFetcher{APIKey: "test-key", BaseURL: server.URL}
	wallet := Wallet{ID: "w1", Address: "0xabc", Network: "base"}

	_, err := fetcher.FetchNativeBalance(context.Background(), wallet, SupportedNetworks)
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("error = %v, want status 502", err)
	}
}

func TestHexToDecimal(t *testing.T) {
	tests := []struct {
		name     string
		hex      string
		decimals int
		want     string
		wantErr  bool
	}{
		{name: "zero", hex: "0x0", decimals: 18, want: "0.000000000000000000"},
		{name: "one ether", hex: "0xde0b6b3a7640000", decimals: 18, want: "1.000000000000000000"},
		{name: "ten usdc", hex: "0x989680", decimals: 6, want: "10.000000"},
		{name: "no decimals", hex: "0x10", decimals: 0, want: "16"},
		{name: "empty", hex: "", decimals: 0, want: "0"},
		{name: "without prefix", hex: "ff", decimals: 0, want: "255"},
		{name: "invalid", hex: "0xzz", decimals: 6, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := hexToDecimal(tc.hex, tc.decimals)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("hexToDecimal: %v", err)
			}
			if got != tc.want {
				t.Fatalf("hexToDecimal(%q, %d) = %q, want %q", tc.hex, tc.decimals, got, tc.want)
			}
		})
	}
}

func TestBalanceOfData(t *testing.T) {
	got := balanceOfData("0xAbC123")
	want := "0x70a08231" + strings.Repeat("0", 64-len("abc123")) + "abc123"
	if got != want {
		t.Fatalf("balanceOfData = %q, want %q", got, want)
	}
}

func TestResolveNetworkWithRegistry(t *testing.T) {
	tests := []struct {
		name       string
		network    string
		testnet    bool
		wantKey    string
		wantSymbol string
		wantErr    bool
	}{
		{name: "canonical", network: "base-mainnet", wantKey: "base-mainnet", wantSymbol: "ETH"},
		{name: "base mainnet", network: "base", wantKey: "base-mainnet", wantSymbol: "ETH"},
		{name: "base testnet", network: "base", testnet: true, wantKey: "base-sepolia", wantSymbol: "ETH"},
		{name: "gnosis", network: "xdai", wantKey: "gnosis-mainnet", wantSymbol: "XDAI"},
		{name: "unsupported", network: "solana", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, key, err := resolveNetworkWithRegistry(tc.network, tc.testnet, SupportedNetworks)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if key != tc.wantKey || cfg.NativeSymbol != tc.wantSymbol {
				t.Fatalf("got key %q symbol %q, want %q %q", key, cfg.NativeSymbol, tc.wantKey, tc.wantSymbol)
			}
		})
	}
}

func TestAlchemyFetcherRPCError(t *testing.T) {
	server := newAlchemyServer(t, map[string]string{}, map[string]bool{"eth_getBalance": true})
	defer server.Close()

	fetcher := &AlchemyFetcher{APIKey: "test-key", BaseURL: server.URL}
	_, err := fetcher.FetchNativeBalance(context.Background(), Wallet{ID: "w1", Address: "0xabc", Network: "base"}, SupportedNetworks)
	if err == nil {
		t.Fatal("expected rpc error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v, want rpc message", err)
	}
}

func TestAlchemyFetcherUnsupportedNetwork(t *testing.T) {
	fetcher := &AlchemyFetcher{APIKey: "test-key"}
	_, err := fetcher.FetchNativeBalance(context.Background(), Wallet{ID: "w1", Address: "0xabc", Network: "solana"}, SupportedNetworks)
	if err == nil || !strings.Contains(err.Error(), "unsupported network") {
		t.Fatalf("error = %v, want unsupported network", err)
	}
}
