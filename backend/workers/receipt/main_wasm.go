//go:build js && wasm

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"github.com/mindsgn-studio/afika-backend/workers/receipt/lib"
	"github.com/syumai/workers-go/cloudflare"
	"github.com/syumai/workers-go/cloudflare/cron"
	_ "github.com/syumai/workers-go/cloudflare/d1" // register the "d1" database/sql driver
)

// defaultDBBinding matches the shared `afika` D1 binding in wrangler.jsonc.
const defaultDBBinding = "afika"

func main() {
	cron.ScheduleTask(run)
}

// run executes one receipt reconciliation pass.
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

	result, err := processPayments(ctx, lib.NewStore(db), 0)
	if err != nil {
		return fmt.Errorf("process payments: %w", err)
	}

	encoded, _ := json.Marshal(result)
	log.Printf("%s done %s", logPrefix, encoded)
	return nil
}
