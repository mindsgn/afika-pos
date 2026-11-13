package posservice

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/mindsgn-studio/afika-backend/api/internal/posstore"
)

// Service is the POS use-case layer. It owns business validation and the order
// of reads/writes; it never talks to HTTP.
type Service struct {
	store    *posstore.DB
	location *time.Location
}

// New wraps a POS repository.
func New(store *posstore.DB) *Service {
	return &Service{store: store, location: time.UTC}
}

// SetLocation sets the time zone used when formatting receipt timestamps.
func (s *Service) SetLocation(location *time.Location) {
	if location != nil {
		s.location = location
	}
}

// Store exposes the repository for the rare caller that needs a raw read (the
// payment confirmation path looks sales up without going through a use case).
func (s *Service) Store() *posstore.DB { return s.store }

func (s *Service) nowMS() int64 { return s.store.Now().UnixMilli() }

// BusinessInput is the onboarding form. Website may be entered without a
// scheme; it is normalised before saving.
type BusinessInput struct {
	Name          string `json:"name"`
	Website       string `json:"website"`
	Address       string `json:"address"`
	WalletAddress string `json:"walletAddress"`
	LogoURI       string `json:"logoUri"`
}

var websitePattern = regexp.MustCompile(`(?i)^https?://[^\s.]+\.[^\s]{2,}$`)

// normalizeWebsite adds https:// when the merchant types a bare domain.
func normalizeWebsite(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(value), "http://") && !strings.HasPrefix(strings.ToLower(value), "https://") {
		return "https://" + value
	}
	return value
}

// Business returns the merchant profile owned by subject, or nil when
// onboarding has not run yet.
func (s *Service) Business(ctx context.Context, ownerID string) (*posstore.Business, error) {
	if strings.TrimSpace(ownerID) == "" {
		return nil, invalidf(CodeInvalidBusiness, "a signed-in merchant is required")
	}
	business, err := s.store.GetBusinessByOwner(ctx, ownerID)
	if errors.Is(err, posstore.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, internalf(err)
	}
	return &business, nil
}

// RequireBusiness resolves the merchant profile or fails with
// BUSINESS_NOT_SET_UP, which the clients treat as "go to onboarding".
func (s *Service) RequireBusiness(ctx context.Context, ownerID string) (posstore.Business, error) {
	business, err := s.Business(ctx, ownerID)
	if err != nil {
		return posstore.Business{}, err
	}
	if business == nil {
		return posstore.Business{}, conflictf(CodeBusinessNotSetUp, "set up your business profile first")
	}
	return *business, nil
}

// SaveBusiness creates or updates the merchant profile.
func (s *Service) SaveBusiness(ctx context.Context, ownerID string, input BusinessInput) (posstore.Business, error) {
	if strings.TrimSpace(ownerID) == "" {
		return posstore.Business{}, invalidf(CodeInvalidBusiness, "a signed-in merchant is required")
	}
	name := strings.TrimSpace(input.Name)
	if len([]rune(name)) < 2 {
		return posstore.Business{}, invalidf(CodeInvalidBusiness, "Enter your business name")
	}
	website := normalizeWebsite(input.Website)
	if website != "" && !websitePattern.MatchString(website) {
		return posstore.Business{}, invalidf(CodeInvalidBusiness, "Enter a valid website, e.g. afika.app")
	}
	address := strings.TrimSpace(input.Address)
	if len([]rune(address)) < 4 {
		return posstore.Business{}, invalidf(CodeInvalidBusiness, "Enter your business address")
	}
	wallet := strings.ToLower(strings.TrimSpace(input.WalletAddress))

	existing, err := s.Business(ctx, ownerID)
	if err != nil {
		return posstore.Business{}, err
	}
	now := s.nowMS()
	business := posstore.Business{
		ID:            posstore.NewID(),
		Name:          name,
		Website:       website,
		Address:       address,
		WalletAddress: wallet,
		LogoURI:       strings.TrimSpace(input.LogoURI),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if existing != nil {
		business.ID = existing.ID
		business.CreatedAt = existing.CreatedAt
		if business.LogoURI == "" {
			business.LogoURI = existing.LogoURI
		}
	}

	writer := posstore.NewWriter()
	writer.UpsertBusiness(ownerID, business)
	if err := s.store.Write(ctx, writer); err != nil {
		return posstore.Business{}, internalf(err)
	}
	saved, err := s.store.GetBusinessByOwner(ctx, ownerID)
	if err != nil {
		return posstore.Business{}, internalf(err)
	}
	return saved, nil
}

// ---------------------------------------------------------------------------
// Categories
// ---------------------------------------------------------------------------

// ListCategories returns the business catalogue groups.
func (s *Service) ListCategories(ctx context.Context, businessID string) ([]posstore.Category, error) {
	items, err := s.store.ListCategories(ctx, businessID)
	if err != nil {
		return nil, internalf(err)
	}
	return items, nil
}

// CreateCategory adds a uniquely named category.
func (s *Service) CreateCategory(ctx context.Context, businessID, name string) (posstore.Category, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return posstore.Category{}, invalidf(CodeInvalidName, "Category name is required")
	}
	taken, err := s.store.CategoryNameTaken(ctx, businessID, trimmed, "")
	if err != nil {
		return posstore.Category{}, internalf(err)
	}
	if taken {
		return posstore.Category{}, conflictf(CodeCategoryExists, "That category already exists")
	}
	category := posstore.Category{ID: posstore.NewID(), Name: trimmed, CreatedAt: s.nowMS()}
	writer := posstore.NewWriter()
	writer.InsertCategory(businessID, category)
	if err := s.store.Write(ctx, writer); err != nil {
		return posstore.Category{}, internalf(err)
	}
	return category, nil
}

// RenameCategory changes a category's name.
func (s *Service) RenameCategory(ctx context.Context, businessID, id, name string) (posstore.Category, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return posstore.Category{}, invalidf(CodeInvalidName, "Category name is required")
	}
	if _, err := s.store.GetCategory(ctx, businessID, id); err != nil {
		return posstore.Category{}, mapNotFound(err, CodeCategoryNotFound, "Category not found")
	}
	taken, err := s.store.CategoryNameTaken(ctx, businessID, trimmed, id)
	if err != nil {
		return posstore.Category{}, internalf(err)
	}
	if taken {
		return posstore.Category{}, conflictf(CodeCategoryExists, "That category already exists")
	}
	writer := posstore.NewWriter()
	writer.RenameCategory(businessID, id, trimmed)
	if err := s.store.Write(ctx, writer); err != nil {
		return posstore.Category{}, internalf(err)
	}
	category, err := s.store.GetCategory(ctx, businessID, id)
	if err != nil {
		return posstore.Category{}, mapNotFound(err, CodeCategoryNotFound, "Category not found")
	}
	return category, nil
}

// DeleteCategory removes an empty category.
func (s *Service) DeleteCategory(ctx context.Context, businessID, id string) error {
	if _, err := s.store.GetCategory(ctx, businessID, id); err != nil {
		return mapNotFound(err, CodeCategoryNotFound, "Category not found")
	}
	used, err := s.store.CountProductsInCategory(ctx, businessID, id)
	if err != nil {
		return internalf(err)
	}
	if used > 0 {
		return conflictf(CodeCategoryInUse, "Move or delete the %d product(s) in this category first", used)
	}
	writer := posstore.NewWriter()
	writer.DeleteCategory(businessID, id)
	if err := s.store.Write(ctx, writer); err != nil {
		return internalf(err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Products
// ---------------------------------------------------------------------------

// ProductInput is the create/update payload. Stock is only accepted on create,
// where it becomes the opening-stock movement.
type ProductInput struct {
	SKU          string `json:"sku"`
	Name         string `json:"name"`
	ImageURI     string `json:"imageUri"`
	CategoryID   string `json:"categoryId"`
	Price        int64  `json:"price"`
	CostPrice    int64  `json:"costPrice"`
	Stock        int64  `json:"stock"`
	ReorderLevel int64  `json:"reorderLevel"`
}

// ProductFilter mirrors the app's catalogue filters.
type ProductFilter struct {
	Query           string
	CategoryID      string
	IncludeInactive bool
	LowStockOnly    bool
	Limit           int
}

func validateProduct(input ProductInput) (ProductInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.SKU = strings.ToUpper(strings.TrimSpace(input.SKU))
	input.ImageURI = strings.TrimSpace(input.ImageURI)
	if input.Name == "" {
		return input, invalidf(CodeInvalidProduct, "Product name is required")
	}
	if input.SKU == "" {
		return input, invalidf(CodeInvalidProduct, "SKU is required")
	}
	if input.Price < 0 {
		return input, invalidf(CodeInvalidProduct, "Price must be zero or more")
	}
	if input.CostPrice < 0 {
		return input, invalidf(CodeInvalidProduct, "Cost price must be zero or more")
	}
	if input.Stock < 0 {
		return input, invalidf(CodeInvalidProduct, "Stock must be zero or more")
	}
	if input.ReorderLevel < 0 {
		return input, invalidf(CodeInvalidProduct, "Reorder level must be zero or more")
	}
	return input, nil
}

// ListProducts returns the business catalogue.
func (s *Service) ListProducts(ctx context.Context, businessID string, filter ProductFilter) ([]posstore.Product, error) {
	products, err := s.store.ListProducts(ctx, businessID, posstore.ProductFilter{
		Query:           filter.Query,
		CategoryID:      filter.CategoryID,
		IncludeInactive: filter.IncludeInactive,
		LowStockOnly:    filter.LowStockOnly,
		Limit:           filter.Limit,
	})
	if err != nil {
		return nil, internalf(err)
	}
	return products, nil
}

// GetProduct returns one product.
func (s *Service) GetProduct(ctx context.Context, businessID, id string) (posstore.Product, error) {
	product, err := s.store.GetProduct(ctx, businessID, id)
	if err != nil {
		return posstore.Product{}, mapNotFound(err, CodeProductNotFound, "Product not found")
	}
	return product, nil
}

// CreateProduct adds a product and records its opening stock.
func (s *Service) CreateProduct(ctx context.Context, businessID string, input ProductInput) (posstore.Product, error) {
	input, err := validateProduct(input)
	if err != nil {
		return posstore.Product{}, err
	}
	if err := s.requireCategory(ctx, businessID, input.CategoryID); err != nil {
		return posstore.Product{}, err
	}
	taken, err := s.store.ProductSKUTaken(ctx, businessID, input.SKU, "")
	if err != nil {
		return posstore.Product{}, internalf(err)
	}
	if taken {
		return posstore.Product{}, conflictf(CodeSKUExists, "SKU %s is already in use", input.SKU)
	}

	now := s.nowMS()
	product := posstore.Product{
		ID:           posstore.NewID(),
		SKU:          input.SKU,
		Name:         input.Name,
		ImageURI:     input.ImageURI,
		CategoryID:   input.CategoryID,
		Price:        input.Price,
		CostPrice:    input.CostPrice,
		ReorderLevel: input.ReorderLevel,
		Active:       true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	writer := posstore.NewWriter()
	writer.InsertProduct(businessID, product)
	if input.Stock > 0 {
		cost := input.CostPrice
		writer.MoveStock(businessID, product.ID, "initial", input.Stock, &cost, "", "Opening stock", now)
	}
	if err := s.store.Write(ctx, writer); err != nil {
		return posstore.Product{}, internalf(err)
	}
	return s.GetProduct(ctx, businessID, product.ID)
}

// UpdateProduct edits a product's details. Stock is not editable here.
func (s *Service) UpdateProduct(ctx context.Context, businessID, id string, input ProductInput) (posstore.Product, error) {
	input, err := validateProduct(input)
	if err != nil {
		return posstore.Product{}, err
	}
	if _, err := s.store.GetProduct(ctx, businessID, id); err != nil {
		return posstore.Product{}, mapNotFound(err, CodeProductNotFound, "Product not found")
	}
	if err := s.requireCategory(ctx, businessID, input.CategoryID); err != nil {
		return posstore.Product{}, err
	}
	taken, err := s.store.ProductSKUTaken(ctx, businessID, input.SKU, id)
	if err != nil {
		return posstore.Product{}, internalf(err)
	}
	if taken {
		return posstore.Product{}, conflictf(CodeSKUExists, "SKU %s is already in use", input.SKU)
	}

	product := posstore.Product{
		ID:           id,
		SKU:          input.SKU,
		Name:         input.Name,
		ImageURI:     input.ImageURI,
		CategoryID:   input.CategoryID,
		Price:        input.Price,
		CostPrice:    input.CostPrice,
		ReorderLevel: input.ReorderLevel,
		UpdatedAt:    s.nowMS(),
	}
	writer := posstore.NewWriter()
	writer.UpdateProduct(businessID, product)
	if err := s.store.Write(ctx, writer); err != nil {
		return posstore.Product{}, internalf(err)
	}
	return s.GetProduct(ctx, businessID, id)
}

// SetProductActive archives or restores a product.
func (s *Service) SetProductActive(ctx context.Context, businessID, id string, active bool) (posstore.Product, error) {
	if _, err := s.store.GetProduct(ctx, businessID, id); err != nil {
		return posstore.Product{}, mapNotFound(err, CodeProductNotFound, "Product not found")
	}
	writer := posstore.NewWriter()
	writer.SetProductActive(businessID, id, active, s.nowMS())
	if err := s.store.Write(ctx, writer); err != nil {
		return posstore.Product{}, internalf(err)
	}
	return s.GetProduct(ctx, businessID, id)
}

func (s *Service) requireCategory(ctx context.Context, businessID, categoryID string) error {
	if strings.TrimSpace(categoryID) == "" {
		return nil
	}
	if _, err := s.store.GetCategory(ctx, businessID, categoryID); err != nil {
		return mapNotFound(err, CodeCategoryNotFound, "Category not found")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Customers
// ---------------------------------------------------------------------------

// ListCustomers returns loyalty members, optionally filtered by name or phone.
func (s *Service) ListCustomers(ctx context.Context, businessID, query string) ([]posstore.Customer, error) {
	customers, err := s.store.ListCustomers(ctx, businessID, query)
	if err != nil {
		return nil, internalf(err)
	}
	return customers, nil
}

// GetCustomer returns one loyalty member.
func (s *Service) GetCustomer(ctx context.Context, businessID, id string) (posstore.Customer, error) {
	customer, err := s.store.GetCustomer(ctx, businessID, id)
	if err != nil {
		return posstore.Customer{}, mapNotFound(err, CodeCustomerNotFound, "Customer not found")
	}
	return customer, nil
}

// CreateCustomer adds a loyalty member. Phone numbers are unique per business.
func (s *Service) CreateCustomer(ctx context.Context, businessID, name, phone string) (posstore.Customer, error) {
	name = strings.TrimSpace(name)
	phone = strings.TrimSpace(phone)
	if name == "" {
		return posstore.Customer{}, invalidf(CodeInvalidCustomer, "Customer name is required")
	}
	if phone != "" {
		taken, err := s.store.CustomerPhoneTaken(ctx, businessID, phone)
		if err != nil {
			return posstore.Customer{}, internalf(err)
		}
		if taken {
			return posstore.Customer{}, conflictf(CodeCustomerPhoneUsed, "A customer with that phone number already exists")
		}
	}
	customer := posstore.Customer{
		ID:        posstore.NewID(),
		Name:      name,
		Phone:     phone,
		CreatedAt: s.nowMS(),
	}
	writer := posstore.NewWriter()
	writer.InsertCustomer(businessID, customer)
	if err := s.store.Write(ctx, writer); err != nil {
		return posstore.Customer{}, internalf(err)
	}
	return customer, nil
}

// UpdateCustomer changes a customer's name and phone.
func (s *Service) UpdateCustomer(ctx context.Context, businessID, id, name, phone string) (posstore.Customer, error) {
	name = strings.TrimSpace(name)
	phone = strings.TrimSpace(phone)
	if name == "" {
		return posstore.Customer{}, invalidf(CodeInvalidCustomer, "Customer name is required")
	}
	if _, err := s.store.GetCustomer(ctx, businessID, id); err != nil {
		return posstore.Customer{}, mapNotFound(err, CodeCustomerNotFound, "Customer not found")
	}
	if phone != "" {
		taken, err := s.store.CustomerPhoneTakenByOther(ctx, businessID, phone, id)
		if err != nil {
			return posstore.Customer{}, internalf(err)
		}
		if taken {
			return posstore.Customer{}, conflictf(CodeCustomerPhoneUsed, "A customer with that phone number already exists")
		}
	}
	writer := posstore.NewWriter()
	writer.UpdateCustomer(businessID, id, name, phone)
	if err := s.store.Write(ctx, writer); err != nil {
		return posstore.Customer{}, internalf(err)
	}
	return s.GetCustomer(ctx, businessID, id)
}

// ---------------------------------------------------------------------------
// Inventory
// ---------------------------------------------------------------------------

// AdjustStockInput is a manual stock correction or a restock. A restock can
// carry the new unit cost, which becomes the product's cost price.
type AdjustStockInput struct {
	Delta    int64  `json:"delta"`
	Kind     string `json:"kind"`
	Note     string `json:"note"`
	UnitCost *int64 `json:"unitCost"`
}

// AdjustStock applies a manual stock movement.
func (s *Service) AdjustStock(ctx context.Context, businessID, productID string, input AdjustStockInput) (posstore.Product, error) {
	if input.Kind != "restock" && input.Kind != "adjustment" {
		return posstore.Product{}, invalidf(CodeInvalidStockDelta, "Type must be restock or adjustment")
	}
	if input.Delta == 0 {
		return posstore.Product{}, invalidf(CodeInvalidStockDelta, "Enter a whole number other than zero")
	}
	if input.Kind == "restock" && input.Delta < 0 {
		return posstore.Product{}, invalidf(CodeInvalidStockDelta, "A restock must be a positive quantity")
	}
	if input.UnitCost != nil && *input.UnitCost < 0 {
		return posstore.Product{}, invalidf(CodeInvalidStockDelta, "Cost cannot be negative")
	}
	product, err := s.store.GetProduct(ctx, businessID, productID)
	if err != nil {
		return posstore.Product{}, mapNotFound(err, CodeProductNotFound, "Product not found")
	}
	if input.Kind == "adjustment" && product.Stock+input.Delta < 0 {
		return posstore.Product{}, invalidf(CodeInsufficientStock, "Not enough stock for %s", product.Name)
	}
	note := strings.TrimSpace(input.Note)
	now := s.nowMS()

	writer := posstore.NewWriter()
	var unitCost *int64
	if input.Kind == "restock" {
		unitCost = input.UnitCost
	}
	writer.MoveStock(businessID, productID, input.Kind, input.Delta, unitCost, "", note, now)
	if input.Kind == "restock" && input.UnitCost != nil {
		writer.SetProductCost(businessID, productID, *input.UnitCost, now)
	}
	if err := s.store.Write(ctx, writer); err != nil {
		return posstore.Product{}, internalf(err)
	}
	return s.GetProduct(ctx, businessID, productID)
}

// ListMovements returns the stock ledger newest first.
func (s *Service) ListMovements(ctx context.Context, businessID, productID string, limit int) ([]posstore.StockMovement, error) {
	movements, err := s.store.ListMovements(ctx, businessID, posstore.MovementFilter{ProductID: productID, Limit: limit})
	if err != nil {
		return nil, internalf(err)
	}
	return movements, nil
}

// LowStock returns active products at or below their reorder level.
func (s *Service) LowStock(ctx context.Context, businessID string) ([]posstore.Product, error) {
	return s.ListProducts(ctx, businessID, ProductFilter{LowStockOnly: true})
}

// Valuation returns the current inventory value at cost and at retail.
func (s *Service) Valuation(ctx context.Context, businessID string) (posstore.InventoryValue, error) {
	value, err := s.store.Valuation(ctx, businessID)
	if err != nil {
		return posstore.InventoryValue{}, internalf(err)
	}
	return value, nil
}

// CostHistory returns the movements that recorded a unit cost, newest first.
func (s *Service) CostHistory(ctx context.Context, businessID, productID string) ([]posstore.StockMovement, error) {
	movements, err := s.store.ListMovements(ctx, businessID, posstore.MovementFilter{ProductID: productID, Limit: 500})
	if err != nil {
		return nil, internalf(err)
	}
	withCost := make([]posstore.StockMovement, 0, len(movements))
	for _, movement := range movements {
		if movement.HasUnitCost {
			withCost = append(withCost, movement)
		}
	}
	return withCost, nil
}

// mapNotFound converts a repository miss into a POS not-found error.
func mapNotFound(err error, code, message string) error {
	if errors.Is(err, posstore.ErrNotFound) {
		return notFoundf(code, "%s", message)
	}
	return internalf(err)
}
