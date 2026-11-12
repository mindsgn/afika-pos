//go:build js && wasm

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/mindsgn-studio/afika-backend/workers/forex/lib"
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

// run executes one hourly FX refresh.
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

	// net/http's built-in js/wasm transport cannot call Workerd's fetch
	// directly, so route requests through workers-go's fetch client.
	httpClient := fetch.NewClient().HTTPClient(fetch.RedirectModeFollow)
	httpClient.Timeout = 15 * time.Second

	store := lib.NewStore(db)
	result, err := processRates(ctx, store, &lib.FrankfurterFetcher{Client: httpClient}, 0)
	if err != nil {
		return fmt.Errorf("process rates: %w", err)
	}

	encoded, _ := json.Marshal(result)
	log.Printf("%s done %s", logPrefix, encoded)
	return nil
}
