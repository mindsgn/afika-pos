package posdomain

import "testing"

func TestCryptoAmountFor(t *testing.T) {
	tests := []struct {
		name   string
		asset  string
		cents  int64
		expect string
	}{
		{"usdc base", "usdc-base", 12345, "6.67"},
		{"btc", "btc", 12345, "0.00010288"},
		{"zarp base", "zarp-base", 12345, "123.45"},
		{"eurc ethereum", "eurc-ethereum", 1000, "0.497512"},
		{"zero", "usdc-base", 0, "0.00"},
		{"usdc ethereum", "usdc-ethereum", 1850, "1.000000"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			asset := AssetByID(tc.asset)
			got, err := CryptoAmountFor(tc.cents, asset)
			if err != nil {
				t.Fatalf("CryptoAmountFor: %v", err)
			}
			if got != tc.expect {
				t.Fatalf("CryptoAmountFor = %q, want %q", got, tc.expect)
			}
		})
	}
}

func TestCryptoAmountForNeverUsesFloat(t *testing.T) {
	// 1 cent of BTC at R1,200,000/BTC is 0.0000000083..., which rounds to
	// 0.00000001 at 8 decimal places.
	got, err := CryptoAmountFor(1, AssetByID("btc"))
	if err != nil {
		t.Fatalf("CryptoAmountFor: %v", err)
	}
	if got != "0.00000001" {
		t.Fatalf("one cent of BTC = %q", got)
	}
}

func TestValidateAssetID(t *testing.T) {
	if asset, err := ValidateAssetID(""); err != nil || asset.ID != DefaultAssetID {
		t.Fatalf("empty id = %q, %v", asset.ID, err)
	}
	if asset, err := ValidateAssetID("BTC"); err != nil || asset.ID != "btc" {
		t.Fatalf("BTC = %q, %v", asset.ID, err)
	}
	if _, err := ValidateAssetID("doge"); err == nil {
		t.Fatal("expected an error for an unsupported asset")
	}
}

func TestAssetTableIsComplete(t *testing.T) {
	if len(Assets) != len(AssetOrder) {
		t.Fatalf("Assets has %d entries but AssetOrder has %d", len(Assets), len(AssetOrder))
	}
	for _, id := range AssetOrder {
		asset, ok := Assets[id]
		if !ok {
			t.Fatalf("AssetOrder lists %q but Assets does not", id)
		}
		if asset.ID != id {
			t.Fatalf("asset %q has ID %q", id, asset.ID)
		}
	}
}
