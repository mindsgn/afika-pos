package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	"github.com/mindsgn-studio/afika-backend/api/lib"
	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "file:afika-e2e?mode=memory&cache=shared")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	migration, err := os.ReadFile("migrations/0002_pos_payments.sql")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		log.Fatal(err)
	}
	h := lib.NewHandler(lib.HandlerConfig{
		Store:            lib.NewD1Store(db),
		Verifier:         lib.NewStaticTokenVerifier("playwright-test-token"),
		AllowedOrigins:   []string{"*"},
		ReceivingAddress: "0x1111111111111111111111111111111111111111",
		CheckoutBaseURL:  "http://127.0.0.1:4173",
	})
	log.Fatal(http.ListenAndServe("127.0.0.1:4173", h))
}
