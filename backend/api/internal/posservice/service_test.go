package posservice

import (
	"context"
	"testing"
)

func TestBusinessLifecycle(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()

	business, err := service.Business(ctx, testOwner)
	if err != nil || business != nil {
		t.Fatalf("empty business = %+v / %v", business, err)
	}
	if _, err := service.RequireBusiness(ctx, testOwner); errCode(t, err) != CodeBusinessNotSetUp {
		t.Fatalf("require business error = %v", err)
	}

	// Validation mirrors the onboarding form.
	cases := []BusinessInput{
		{Name: "A", Website: "kofi.example", Address: "12 Long St"},
		{Name: "Kofi", Website: "not a website", Address: "12 Long St"},
		{Name: "Kofi", Website: "kofi.example", Address: "12"},
	}
	for _, input := range cases {
		if _, err := service.SaveBusiness(ctx, testOwner, input); errCode(t, err) != CodeInvalidBusiness {
			t.Fatalf("invalid business %+v error = %v", input, err)
		}
	}

	saved := seedBusiness(t, service)
	if saved.ID == "" || saved.Website != "https://kofi.example" {
		t.Fatalf("saved business = %+v", saved)
	}

	updated, err := service.SaveBusiness(ctx, testOwner, BusinessInput{
		Name: "Kofi Coffee Co", Website: "https://kofi.example", Address: "13 Long St",
	})
	if err != nil {
		t.Fatalf("update business: %v", err)
	}
	if updated.ID != saved.ID || updated.CreatedAt != saved.CreatedAt {
		t.Fatalf("update changed identity: %+v", updated)
	}
	if updated.Name != "Kofi Coffee Co" || updated.Address != "13 Long St" {
		t.Fatalf("update not applied: %+v", updated)
	}
}

func TestCategories(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)

	coffee, err := service.CreateCategory(ctx, business.ID, " Coffee ")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	if coffee.Name != "Coffee" {
		t.Fatalf("category name = %q", coffee.Name)
	}
	if _, err := service.CreateCategory(ctx, business.ID, "Coffee"); errCode(t, err) != CodeCategoryExists {
		t.Fatalf("duplicate category error = %v", err)
	}
	if _, err := service.CreateCategory(ctx, business.ID, "  "); errCode(t, err) != CodeInvalidName {
		t.Fatalf("empty category error = %v", err)
	}

	bakery, err := service.CreateCategory(ctx, business.ID, "Bakery")
	if err != nil {
		t.Fatalf("create bakery: %v", err)
	}
	if _, err := service.RenameCategory(ctx, business.ID, bakery.ID, "Coffee"); errCode(t, err) != CodeCategoryExists {
		t.Fatalf("rename clash error = %v", err)
	}
	renamed, err := service.RenameCategory(ctx, business.ID, bakery.ID, "Pastry")
	if err != nil || renamed.Name != "Pastry" {
		t.Fatalf("rename = %+v / %v", renamed, err)
	}
	if _, err := service.RenameCategory(ctx, business.ID, "missing", "Pastry"); errCode(t, err) != CodeCategoryNotFound {
		t.Fatalf("rename missing error = %v", err)
	}

	catalogue, err := service.ListCategories(ctx, business.ID)
	if err != nil || len(catalogue) != 2 {
		t.Fatalf("categories = %+v / %v", catalogue, err)
	}
	if catalogue[0].Name != "Coffee" || catalogue[1].Name != "Pastry" {
		t.Fatalf("categories not sorted: %+v", catalogue)
	}

	// A category with products cannot be deleted.
	flat, err := service.CreateProduct(ctx, business.ID, ProductInput{
		SKU: "cof", Name: "Flat White", CategoryID: coffee.ID, Price: 3500, CostPrice: 1200, Stock: 4,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	if flat.SKU != "COF" {
		t.Fatalf("SKU not normalised: %q", flat.SKU)
	}
	if err := service.DeleteCategory(ctx, business.ID, coffee.ID); errCode(t, err) != CodeCategoryInUse {
		t.Fatalf("delete used category error = %v", err)
	}
	if err := service.DeleteCategory(ctx, business.ID, bakery.ID); err != nil {
		t.Fatalf("delete empty category: %v", err)
	}
	if err := service.DeleteCategory(ctx, business.ID, bakery.ID); errCode(t, err) != CodeCategoryNotFound {
		t.Fatalf("delete missing category error = %v", err)
	}
}

func TestProductsAndInventory(t *testing.T) {
	service, store := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)

	flat := seedProduct(t, service, business.ID, "cof", "Flat White", 3500, 1200, 10, 5)
	if flat.Stock != 10 || flat.Active != true {
		t.Fatalf("product = %+v", flat)
	}
	// Opening stock is a movement, not a silent write.
	movements, err := service.ListMovements(ctx, business.ID, flat.ID, 10)
	if err != nil || len(movements) != 1 {
		t.Fatalf("movements = %+v / %v", movements, err)
	}
	if movements[0].Type != "initial" || movements[0].Quantity != 10 || movements[0].BalanceAfter != 10 {
		t.Fatalf("initial movement = %+v", movements[0])
	}

	if _, err := service.CreateProduct(ctx, business.ID, ProductInput{
		SKU: "COF", Name: "Other", Price: 100,
	}); errCode(t, err) != CodeSKUExists {
		t.Fatalf("duplicate SKU error = %v", err)
	}
	for _, input := range []ProductInput{
		{SKU: "", Name: "No SKU", Price: 100},
		{SKU: "X", Name: " ", Price: 100},
		{SKU: "X", Name: "Bad price", Price: -1},
		{SKU: "X", Name: "Bad cost", Price: 100, CostPrice: -1},
		{SKU: "X", Name: "Bad stock", Price: 100, Stock: -1},
	} {
		if _, err := service.CreateProduct(ctx, business.ID, input); errCode(t, err) != CodeInvalidProduct {
			t.Fatalf("invalid product %+v error = %v", input, err)
		}
	}
	if _, err := service.CreateProduct(ctx, business.ID, ProductInput{
		SKU: "MUF", Name: "Muffin", CategoryID: "missing", Price: 2500,
	}); errCode(t, err) != CodeCategoryNotFound {
		t.Fatalf("missing category error = %v", err)
	}

	// Edits never touch stock.
	updated, err := service.UpdateProduct(ctx, business.ID, flat.ID, ProductInput{
		SKU: "cof", Name: "Flat White Large", Price: 3900, CostPrice: 1250, Stock: 999, ReorderLevel: 4,
	})
	if err != nil {
		t.Fatalf("update product: %v", err)
	}
	if updated.Name != "Flat White Large" || updated.Price != 3900 || updated.Stock != 10 || updated.ReorderLevel != 4 {
		t.Fatalf("updated product = %+v", updated)
	}
	if _, err := service.UpdateProduct(ctx, business.ID, "missing", ProductInput{SKU: "X", Name: "X"}); errCode(t, err) != CodeProductNotFound {
		t.Fatalf("update missing product error = %v", err)
	}

	// Restock with a new unit cost updates the cost price and logs history.
	restocked, err := service.AdjustStock(ctx, business.ID, flat.ID, AdjustStockInput{Delta: 6, Kind: "restock", UnitCost: ptr(1300), Note: "Delivery"})
	if err != nil {
		t.Fatalf("restock: %v", err)
	}
	if restocked.Stock != 16 || restocked.CostPrice != 1300 {
		t.Fatalf("restocked product = %+v", restocked)
	}
	history, err := service.CostHistory(ctx, business.ID, flat.ID)
	if err != nil || len(history) != 2 || history[0].UnitCost != 1300 {
		t.Fatalf("cost history = %+v / %v", history, err)
	}

	// Adjustments may go down but never below zero.
	adjusted, err := service.AdjustStock(ctx, business.ID, flat.ID, AdjustStockInput{Delta: -2, Kind: "adjustment"})
	if err != nil || adjusted.Stock != 14 {
		t.Fatalf("adjust = %+v / %v", adjusted, err)
	}
	if _, err := service.AdjustStock(ctx, business.ID, flat.ID, AdjustStockInput{Delta: -99, Kind: "adjustment"}); errCode(t, err) != CodeInsufficientStock {
		t.Fatalf("over-adjust error = %v", err)
	}
	if _, err := service.AdjustStock(ctx, business.ID, flat.ID, AdjustStockInput{Delta: -1, Kind: "restock"}); errCode(t, err) != CodeInvalidStockDelta {
		t.Fatalf("negative restock error = %v", err)
	}
	if _, err := service.AdjustStock(ctx, business.ID, flat.ID, AdjustStockInput{Delta: 0, Kind: "adjustment"}); errCode(t, err) != CodeInvalidStockDelta {
		t.Fatalf("zero adjustment error = %v", err)
	}
	if _, err := service.AdjustStock(ctx, business.ID, "missing", AdjustStockInput{Delta: 1, Kind: "restock"}); errCode(t, err) != CodeProductNotFound {
		t.Fatalf("missing product stock error = %v", err)
	}

	// Low stock only includes active products with a reorder level.
	muffin := seedProduct(t, service, business.ID, "MUF", "Muffin", 2500, 900, 2, 5)
	low, err := service.LowStock(ctx, business.ID)
	if err != nil || len(low) != 1 || low[0].ID != muffin.ID {
		t.Fatalf("low stock = %+v / %v", low, err)
	}

	archived, err := service.SetProductActive(ctx, business.ID, muffin.ID, false)
	if err != nil || archived.Active {
		t.Fatalf("archive = %+v / %v", archived, err)
	}
	value, err := service.Valuation(ctx, business.ID)
	if err != nil {
		t.Fatalf("valuation: %v", err)
	}
	if value.Units != 14 || value.CostValue != 14*1300 || value.ProductCount != 1 {
		t.Fatalf("valuation = %+v", value)
	}

	if n := countRows(t, store, `SELECT COUNT(*) FROM pos_stock_movements WHERE business_id = ?`, business.ID); n != 4 {
		t.Fatalf("movement count = %d, want 4 (initial, restock, adjustment, muffin initial)", n)
	}
}

func TestCustomers(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)

	if _, err := service.CreateCustomer(ctx, business.ID, " ", ""); errCode(t, err) != CodeInvalidCustomer {
		t.Fatalf("empty customer error = %v", err)
	}
	first := seedCustomer(t, service, business.ID, "Thandi", "0820000000")
	if first.LoyaltyPoints != 0 {
		t.Fatalf("new customer points = %d", first.LoyaltyPoints)
	}
	if _, err := service.CreateCustomer(ctx, business.ID, "Other", "0820000000"); errCode(t, err) != CodeCustomerPhoneUsed {
		t.Fatalf("duplicate phone error = %v", err)
	}
	second := seedCustomer(t, service, business.ID, "Sipho", "")

	renamed, err := service.UpdateCustomer(ctx, business.ID, second.ID, "Sipho N", "0830000000")
	if err != nil || renamed.Name != "Sipho N" || renamed.Phone != "0830000000" {
		t.Fatalf("update customer = %+v / %v", renamed, err)
	}
	if _, err := service.UpdateCustomer(ctx, business.ID, second.ID, "Sipho N", "0820000000"); errCode(t, err) != CodeCustomerPhoneUsed {
		t.Fatalf("update to used phone error = %v", err)
	}
	if _, err := service.UpdateCustomer(ctx, business.ID, "missing", "X", ""); errCode(t, err) != CodeCustomerNotFound {
		t.Fatalf("update missing customer error = %v", err)
	}

	found, err := service.ListCustomers(ctx, business.ID, "thandi")
	if err != nil || len(found) != 1 || found[0].ID != first.ID {
		t.Fatalf("search customers = %+v / %v", found, err)
	}
}

func ptr(value int64) *int64 { return &value }
