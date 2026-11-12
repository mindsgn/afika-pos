//go:build js && wasm

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/mindsgn-studio/afika-backend/workers/balance/lib"
	"github.com/syumai/workers-go/cloudflare"
	"github.com/syumai/workers-go/cloudflare/cron"
	_ "github.com/syumai/workers-go/cloudflare/d1" // register the "d1" database/sql driver
	"github.com/syumai/workers-go/cloudflare/fetch"
)

// defaultDBBinding matches the shared `afika` D1 binding in wrangler.jsonc.
const defaultDBBinding = "afika"

func main() {
	cron.ScheduleTask(run)
}

// run executes one balance refresh for every wallet stored in D1.
func run(ctx context.Context) error {
	binding := cloudflare.Getenv("DB_BINDING")
	if binding == "" {
		binding = defaultDBBinding
	}

	db, err := sql.Open("d1", binding)
	if err != nil {
		return fmt.Errorf("open D1 binding %q: %w", binding, err)
	}
	defer db.Close()

	apiKey := cloudflare.Getenv("ALCHEMY_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("ALCHEMY_API_KEY is not configured")
	}

	store := lib.NewStore(db)

	// net/http's built-in js/wasm transport cannot call Workerd's fetch
	// directly, so route requests through workers-go's fetch client.
	httpClient := fetch.NewClient().HTTPClient(fetch.RedirectModeFollow)
	httpClient.Timeout = 10 * time.Second

	result, err := processBalances(ctx, store, &lib.AlchemyFetcher{
		APIKey: apiKey,
		Client: httpClient,
	})
	if err != nil {
		return fmt.Errorf("process balances: %w", err)
	}

	encoded, _ := json.Marshal(result)
	log.Printf("%s done %s", logPrefix, encoded)
	return nil
}
