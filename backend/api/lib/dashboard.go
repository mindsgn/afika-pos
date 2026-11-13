package lib

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mindsgn-studio/afika-backend/api/internal/posdomain"
	"github.com/mindsgn-studio/afika-backend/api/internal/posservice"
	"github.com/mindsgn-studio/afika-backend/api/internal/posstore"
)

const dashboardCSS = `<style>
body{background:#f5f6f3;color:#20221d}.dash{max-width:1200px;margin:24px auto;padding:0 24px}.dash-head{display:flex;justify-content:space-between;align-items:center;background:#20221d;color:white;border-radius:18px;padding:18px 22px}.dash-head a{color:#d9ed65;text-decoration:none}.dash-grid{display:grid;grid-template-columns:220px 1fr;gap:20px;margin-top:20px}.dash-nav,.dash-content{background:white;border:1px solid #e5e7e0;border-radius:18px;padding:20px}.dash-nav{display:flex;flex-direction:column;gap:5px;align-self:start}.dash-nav a{padding:10px 12px;border-radius:10px;text-decoration:none;color:#45483f}.dash-nav a:hover,.dash-nav a.active{background:#f0f3df;color:#20221d}.dash-content h1{margin:0 0 8px;font-size:30px;letter-spacing:-.04em}.dash-content h2{font-size:18px;margin:28px 0 12px}.dash-content p{color:#6a6e63}.dash-content form{background:#f8f9f6;border:1px solid #eceee8;border-radius:14px;padding:16px;margin:14px 0}.fields{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.dash label{font-size:13px;font-weight:650;display:block;margin-bottom:5px}.dash input,.dash select{width:100%;height:42px;border:1px solid #dfe2d9;border-radius:9px;padding:0 10px;font:inherit;background:white}.dash button,.dash .action{display:inline-flex;align-items:center;border:0;border-radius:10px;background:#20221d;color:#d9ed65;padding:11px 15px;font:inherit;font-weight:700;text-decoration:none;cursor:pointer;margin-top:12px}.dash table{width:100%;border-collapse:collapse;font-size:14px}.dash th,.dash td{text-align:left;padding:12px 8px;border-bottom:1px solid #eceee8}.dash th{color:#777b71;font-size:12px;text-transform:uppercase;letter-spacing:.04em}.stats{display:grid;grid-template-columns:repeat(3,1fr);gap:12px;margin:18px 0}.stat{background:#f8f9f6;padding:16px;border-radius:12px}.stat b{display:block;font-size:22px;margin-top:8px}.notice{padding:12px 14px;background:#f0f3df;border-radius:10px;margin:14px 0}.empty{padding:24px;text-align:center;background:#f8f9f6;border-radius:12px}@media(max-width:760px){.dash-grid{grid-template-columns:1fr}.dash-nav{display:grid;grid-template-columns:repeat(3,1fr)}.fields,.stats{grid-template-columns:1fr}.dash{padding:0 12px}.dash table{font-size:12px}}
</style>`

func (h *Handler) serveDashboard(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/dashboard" && !strings.HasPrefix(r.URL.Path, "/dashboard/") {
		return false
	}
	userID, ok := h.sessionUser(r)
	if !ok {
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return true
	}
	if h.pos == nil {
		http.Error(w, "Dashboard is unavailable", http.StatusServiceUnavailable)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		if err := r.ParseForm(); err != nil {
			h.dashboardError(w, r, "Check the form and try again.")
			return true
		}
		if target, err := h.dashboardAction(ctx, userID, r); err != nil {
			h.dashboardError(w, r, err.Error())
			return true
		} else {
			http.Redirect(w, r, target, http.StatusSeeOther)
			return true
		}
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return true
	}
	business, err := h.pos.Business(ctx, userID)
	if err != nil {
		h.dashboardError(w, r, "We could not load your business.")
		return true
	}
	if business == nil && r.URL.Path != "/dashboard/onboarding" {
		http.Redirect(w, r, "/dashboard/onboarding", http.StatusSeeOther)
		return true
	}
	if business != nil && r.URL.Path == "/dashboard/onboarding" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return true
	}
	body := h.dashboardPage(ctx, userID, business, r.URL.Path)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.WriteString(w, body)
	return true
}

func (h *Handler) dashboardAction(ctx context.Context, owner string, r *http.Request) (string, error) {
	if r.URL.Path == "/dashboard/onboard" {
		_, err := h.pos.SaveBusiness(ctx, owner, posservice.BusinessInput{Name: r.FormValue("name"), Website: r.FormValue("website"), Address: r.FormValue("address"), WalletAddress: r.FormValue("walletAddress")})
		return "/dashboard", err
	}
	business, err := h.pos.RequireBusiness(ctx, owner)
	if err != nil {
		return "", err
	}
	switch r.URL.Path {
	case "/dashboard/category":
		_, err = h.pos.CreateCategory(ctx, business.ID, r.FormValue("name"))
		return "/dashboard/categories", err
	case "/dashboard/customer":
		_, err = h.pos.CreateCustomer(ctx, business.ID, r.FormValue("name"), r.FormValue("phone"))
		return "/dashboard/loyalty", err
	case "/dashboard/product":
		price, e := formInt(r, "price")
		if e != nil {
			return "", e
		}
		cost, e := formInt(r, "costPrice")
		if e != nil {
			return "", e
		}
		stock, e := formInt(r, "stock")
		if e != nil {
			return "", e
		}
		reorder, e := formInt(r, "reorderLevel")
		if e != nil {
			return "", e
		}
		_, err = h.pos.CreateProduct(ctx, business.ID, posservice.ProductInput{Name: r.FormValue("name"), SKU: r.FormValue("sku"), CategoryID: r.FormValue("categoryId"), Price: price, CostPrice: cost, Stock: stock, ReorderLevel: reorder})
		return "/dashboard/products", err
	case "/dashboard/stock":
		delta, e := formInt(r, "delta")
		if e != nil {
			return "", e
		}
		input := posservice.AdjustStockInput{Delta: delta, Kind: r.FormValue("kind"), Note: r.FormValue("note")}
		cost := strings.TrimSpace(r.FormValue("unitCost"))
		if cost != "" {
			v, e := strconv.ParseInt(cost, 10, 64)
			if e != nil {
				return "", fmt.Errorf("Unit cost must be cents")
			}
			input.UnitCost = &v
		}
		_, err = h.pos.AdjustStock(ctx, business.ID, r.FormValue("productId"), input)
		return "/dashboard/inventory", err
	case "/dashboard/checkout":
		productIDs, quantities := r.Form["productId"], r.Form["qty"]
		items := make([]posservice.SaleItemInput, 0, len(productIDs))
		for i, productID := range productIDs {
			if strings.TrimSpace(productID) == "" {
				continue
			}
			qty := int64(1)
			if i < len(quantities) && strings.TrimSpace(quantities[i]) != "" {
				value, e := strconv.ParseInt(quantities[i], 10, 64)
				if e != nil {
					return "", fmt.Errorf("Enter a whole number for quantity")
				}
				qty = value
			}
			items = append(items, posservice.SaleItemInput{ProductID: productID, Qty: qty})
		}
		if len(items) == 0 {
			return "", fmt.Errorf("Add at least one product to the cart")
		}
		discountValue := int64(0)
		if raw := strings.TrimSpace(r.FormValue("discountValue")); raw != "" {
			value, e := strconv.ParseInt(raw, 10, 64)
			if e != nil {
				return "", fmt.Errorf("Enter a whole number for discount")
			}
			discountValue = value
		}
		redeemPoints := int64(0)
		if raw := strings.TrimSpace(r.FormValue("redeemPoints")); raw != "" {
			value, e := strconv.ParseInt(raw, 10, 64)
			if e != nil {
				return "", fmt.Errorf("Enter a whole number for points")
			}
			redeemPoints = value
		}
		method := r.FormValue("paymentMethod")
		if method == "" {
			method = "cash"
		}
		detail, e := h.pos.CreateSale(ctx, business.ID, posservice.CreateSaleInput{Items: items, Discount: posservice.SaleDiscountInput{Type: r.FormValue("discountType"), Value: discountValue}, CustomerID: r.FormValue("customerId"), RedeemPoints: redeemPoints, PaymentMethod: method, Complete: true})
		if e != nil {
			return "", e
		}
		return "/dashboard/sales/" + detail.Sale.ID, e
	}
	if strings.HasPrefix(r.URL.Path, "/dashboard/sales/") {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 3 {
			return "", fmt.Errorf("Sale not found")
		}
		id := parts[2]
		switch parts[len(parts)-1] {
		case "void":
			_, err = h.pos.VoidSale(ctx, business.ID, id, r.FormValue("reason"))
			return "/dashboard/sales/" + id, err
		case "refund":
			qty, e := formInt(r, "qty")
			if e != nil {
				return "", e
			}
			restock := r.FormValue("restock") == "on"
			_, err = h.pos.CreateRefund(ctx, business.ID, id, posservice.CreateRefundInput{Items: []posservice.RefundItemInput{{SaleItemID: r.FormValue("saleItemId"), Qty: qty}}, Reason: r.FormValue("reason"), Restock: &restock})
			return "/dashboard/sales/" + id, err
		}
	}
	return "", fmt.Errorf("That dashboard action is not available")
}

func formInt(r *http.Request, name string) (int64, error) {
	v, e := strconv.ParseInt(strings.TrimSpace(r.FormValue(name)), 10, 64)
	if e != nil {
		return 0, fmt.Errorf("Enter a whole number for %s", name)
	}
	return v, nil
}
func money(cents int64) string { return fmt.Sprintf("R%.2f", float64(cents)/100) }
func esc(s string) string      { return template.HTMLEscapeString(s) }

func (h *Handler) dashboardPage(ctx context.Context, owner string, business *posstore.Business, path string) string {
	if business == nil {
		return dashboardHTML("Set up your business", `<h1>Set up your business</h1><p>Add the details that appear on your receipts.</p><form method="post" action="/dashboard/onboard"><div class="fields"><div><label for="name">Business name</label><input id="name" name="name" required></div><div><label for="website">Website</label><input id="website" name="website" placeholder="shop.example"></div><div><label for="address">Business address</label><input id="address" name="address" required></div><div><label for="walletAddress">Receiving wallet (optional)</label><input id="walletAddress" name="walletAddress"></div></div><button>Continue to dashboard</button></form>`, path)
	}
	section := strings.TrimPrefix(path, "/dashboard")
	if section == "" {
		section = "/overview"
	}
	var content strings.Builder
	switch {
	case section == "/overview" || section == "/insights" || section == "/reports":
		products, _ := h.pos.ListProducts(ctx, business.ID, posservice.ProductFilter{})
		sales, _ := h.pos.ListSales(ctx, business.ID, posstore.SaleFilter{Limit: 100})
		now := time.Now()
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).UnixMilli()
		report, _ := h.pos.Profit(ctx, business.ID, from, now.Add(24*time.Hour).UnixMilli())
		heading := business.Name
		if section == "/insights" {
			heading = "Insights"
		} else if section == "/reports" {
			heading = "Reports"
		}
		fmt.Fprintf(&content, `<h1>%s</h1><p>Welcome back. Here is your shop at a glance.</p><div class="stats"><div class="stat">Products<b>%d</b></div><div class="stat">Sales<b>%d</b></div><div class="stat">Net profit today<b>%s</b></div></div><h2>Quick checkout</h2>`, esc(heading), len(products), len(sales), money(report.Profit))
		customers, _ := h.pos.ListCustomers(ctx, business.ID, "")
		content.WriteString(h.checkoutForm(products, customers))
		content.WriteString(`<h2>Recent sales</h2>`)
		content.WriteString(h.salesTable(ctx, business.ID, sales[:minInt(len(sales), 8)]))
	case section == "/products":
		products, _ := h.pos.ListProducts(ctx, business.ID, posservice.ProductFilter{})
		cats, _ := h.pos.ListCategories(ctx, business.ID)
		content.WriteString(`<h1>Products</h1><p>Manage your catalogue and reorder levels. Prices and costs use cents.</p><form method="post" action="/dashboard/product"><div class="fields"><div><label>Product name<input name="name" required></label></div><div><label>SKU<input name="sku" required></label></div><div><label>Category<select name="categoryId"><option value="">Uncategorised</option>`)
		for _, c := range cats {
			fmt.Fprintf(&content, `<option value="%s">%s</option>`, esc(c.ID), esc(c.Name))
		}
		content.WriteString(`</select></label></div><div><label>Price (cents)<input name="price" type="number" min="0" required></label></div><div><label>Cost price (cents)<input name="costPrice" type="number" min="0" required></label></div><div><label>Opening stock<input name="stock" type="number" min="0" value="0"></label></div><div><label>Reorder level<input name="reorderLevel" type="number" min="0" value="0"></label></div></div><button>Add product</button></form>`)
		content.WriteString(productTable(products))
	case section == "/categories":
		cats, _ := h.pos.ListCategories(ctx, business.ID)
		content.WriteString(`<h1>Categories</h1><form method="post" action="/dashboard/category"><label>Category name<input name="name" required></label><button>Add category</button></form><ul>`)
		for _, c := range cats {
			fmt.Fprintf(&content, `<li>%s</li>`, esc(c.Name))
		}
		content.WriteString(`</ul>`)
	case section == "/inventory" || section == "/stock-movements":
		products, _ := h.pos.ListProducts(ctx, business.ID, posservice.ProductFilter{})
		movements, _ := h.pos.ListMovements(ctx, business.ID, "", 100)
		valuation, _ := h.pos.Valuation(ctx, business.ID)
		fmt.Fprintf(&content, `<h1>Inventory</h1><div class="stats"><div class="stat">Units at cost<b>%s</b></div><div class="stat">Retail value<b>%s</b></div><div class="stat">Stock movements<b>%d</b></div></div>`, money(valuation.CostValue), money(valuation.RetailValue), len(movements))
		content.WriteString(`<form method="post" action="/dashboard/stock"><h2>Record stock movement</h2><div class="fields"><div><label>Product<select name="productId">`)
		for _, p := range products {
			fmt.Fprintf(&content, `<option value="%s">%s · on hand %d</option>`, esc(p.ID), esc(p.Name), p.Stock)
		}
		content.WriteString(`</select></label></div><div><label>Type<select name="kind"><option value="restock">Restock</option><option value="adjustment">Adjustment</option></select></label></div><div><label>Quantity change<input name="delta" type="number" required></label></div><div><label>Unit cost (cents, optional)<input name="unitCost" type="number" min="0"></label></div><div><label>Note<input name="note"></label></div></div><button>Save movement</button></form><h2>Stock movement history</h2><table><thead><tr><th>Product</th><th>Type</th><th>Change</th><th>Note</th></tr></thead><tbody>`)
		for _, m := range movements {
			fmt.Fprintf(&content, `<tr><td>%s</td><td>%s</td><td>%+d</td><td>%s</td></tr>`, esc(m.ProductName), esc(m.Type), m.Quantity, esc(m.Note))
		}
		content.WriteString(`</tbody></table>`)
	case section == "/checkout" || section == "/cart":
		products, _ := h.pos.ListProducts(ctx, business.ID, posservice.ProductFilter{})
		customers, _ := h.pos.ListCustomers(ctx, business.ID, "")
		heading := "Checkout"
		if section == "/cart" {
			heading = "Cart"
		}
		fmt.Fprintf(&content, `<h1>%s</h1><p>Add an item and quantity, then complete payment. Prices come from your catalogue.</p>`, heading)
		content.WriteString(h.checkoutForm(products, customers))
	case section == "/sales":
		sales, _ := h.pos.ListSales(ctx, business.ID, posstore.SaleFilter{Limit: 100})
		content.WriteString(`<h1>Sales</h1><p>Open a sale to view its receipt, refund items, or void it.</p>`)
		content.WriteString(h.salesTable(ctx, business.ID, sales))
	case section == "/refunds":
		sales, _ := h.pos.ListSales(ctx, business.ID, posstore.SaleFilter{Limit: 100})
		content.WriteString(`<h1>Refunds</h1><table><thead><tr><th>Receipt</th><th>Amount</th><th>Reason</th><th>Date</th></tr></thead><tbody>`)
		for _, sale := range sales {
			detail, e := h.pos.GetSale(ctx, business.ID, sale.ID)
			if e == nil {
				for _, rf := range detail.Refunds {
					fmt.Fprintf(&content, `<tr><td><a href="/dashboard/sales/%s">%s</a></td><td>%s</td><td>%s</td><td>%s</td></tr>`, esc(sale.ID), esc(sale.ReceiptNo), money(rf.Amount), esc(rf.Reason), time.UnixMilli(rf.CreatedAt).Format("2 Jan 2006 15:04"))
				}
			}
		}
		content.WriteString(`</tbody></table>`)
	case section == "/voids":
		sales, _ := h.pos.ListSales(ctx, business.ID, posstore.SaleFilter{Status: posdomain.StatusVoided, Limit: 100})
		content.WriteString(`<h1>Voids</h1><p>Voided sales are recorded with restored inventory and reversed loyalty points.</p>`)
		content.WriteString(h.salesTable(ctx, business.ID, sales))
	case section == "/stock-movements":
		movements, _ := h.pos.ListMovements(ctx, business.ID, "", 200)
		content.WriteString(`<h1>Stock movements</h1><table><thead><tr><th>Product</th><th>Type</th><th>Change</th><th>Balance</th><th>Reference / note</th></tr></thead><tbody>`)
		for _, m := range movements {
			fmt.Fprintf(&content, `<tr><td>%s</td><td>%s</td><td>%+d</td><td>%d</td><td>%s %s</td></tr>`, esc(m.ProductName), esc(m.Type), m.Quantity, m.BalanceAfter, esc(m.Reference), esc(m.Note))
		}
		content.WriteString(`</tbody></table>`)
	case section == "/loyalty":
		customers, _ := h.pos.ListCustomers(ctx, business.ID, "")
		content.WriteString(`<h1>Loyalty</h1><p>Customers earn points on completed sales. Add a customer here and select them during checkout.</p><form method="post" action="/dashboard/customer"><div class="fields"><label>Customer name<input name="name" required></label><label>Phone<input name="phone" type="tel"></label></div><button>Add customer</button></form><table><thead><tr><th>Customer</th><th>Phone</th><th>Points balance</th></tr></thead><tbody>`)
		for _, c := range customers {
			fmt.Fprintf(&content, `<tr><td>%s</td><td>%s</td><td>%d</td></tr>`, esc(c.Name), esc(c.Phone), c.LoyaltyPoints)
		}
		content.WriteString(`</tbody></table>`)
	case section == "/payments":
		sales, _ := h.pos.ListSales(ctx, business.ID, posstore.SaleFilter{Limit: 100})
		content.WriteString(`<h1>Payments</h1><p>Payment method and settlement status for each recorded sale.</p><table><thead><tr><th>Receipt</th><th>Method</th><th>Status</th><th>Amount</th><th>Date</th></tr></thead><tbody>`)
		for _, s := range sales {
			fmt.Fprintf(&content, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`, esc(s.ReceiptNo), esc(s.PaymentMethod), esc(s.PaymentStatus), money(s.Total), time.UnixMilli(s.CreatedAt).Format("2 Jan 2006 15:04"))
		}
		content.WriteString(`</tbody></table><h2>Online checkout</h2><p>Shareable crypto checkout requests are created from the Afika POS app and their status is reflected in sales when settled.</p>`)
	case section == "/receipts":
		sales, _ := h.pos.ListSales(ctx, business.ID, posstore.SaleFilter{Limit: 100})
		content.WriteString(`<h1>Receipts</h1><p>Open a sale to view its receipt.</p>`)
		content.WriteString(h.salesTable(ctx, business.ID, sales))
	case strings.HasPrefix(section, "/sales/") && !strings.HasSuffix(section, "/receipt") && !strings.Contains(section, "/refunds/"):
		id := strings.TrimPrefix(section, "/sales/")
		detail, e := h.pos.GetSale(ctx, business.ID, id)
		if e != nil {
			content.WriteString(`<h1>Sale not found</h1>`)
			break
		}
		fmt.Fprintf(&content, `<h1>Sale %s</h1><p>Status: %s · Total: %s · Payment: %s</p><p><a class="action" href="/dashboard/sales/%s/receipt">View receipt</a></p><table><thead><tr><th>Item</th><th>Qty</th><th>Unit price</th><th>Refunded</th></tr></thead><tbody>`, esc(detail.Sale.ReceiptNo), esc(detail.Sale.Status), money(detail.Sale.Total), esc(detail.Sale.PaymentMethod), esc(id))
		for _, it := range detail.Items {
			fmt.Fprintf(&content, `<tr><td>%s</td><td>%d</td><td>%s</td><td>%d</td></tr>`, esc(it.Name), it.Qty, money(it.UnitPrice), it.RefundedQty)
		}
		content.WriteString(`</tbody></table>`)
		if detail.Sale.Status == posdomain.StatusCompleted || detail.Sale.Status == posdomain.StatusPartiallyRefunded {
			if len(detail.Items) > 0 {
				it := detail.Items[0]
				fmt.Fprintf(&content, `<form method="post" action="/dashboard/sales/%s/refund"><h2>Refund an item</h2><input type="hidden" name="saleItemId" value="%s"><label>Quantity to refund (%s)<input name="qty" type="number" min="1" max="%d" value="1" required></label><label>Reason<input name="reason"></label><label><input style="width:auto;height:auto" type="checkbox" name="restock" checked> Return item to stock</label><button>Record refund</button></form>`, esc(id), esc(it.ID), esc(it.Name), it.Qty-it.RefundedQty)
			}
			if detail.Sale.Status == posdomain.StatusCompleted {
				fmt.Fprintf(&content, `<form method="post" action="/dashboard/sales/%s/void"><label>Void reason<input name="reason"></label><button>Void sale</button></form>`, esc(id))
			}
		}
		if len(detail.Refunds) > 0 {
			content.WriteString(`<h2>Refunds</h2><ul>`)
			for _, rf := range detail.Refunds {
				fmt.Fprintf(&content, `<li>%s · %s · <a href="/dashboard/sales/%s/refunds/%s/receipt">refund receipt</a></li>`, money(rf.Amount), time.UnixMilli(rf.CreatedAt).Format("2 Jan 2006 15:04"), esc(id), esc(rf.ID))
			}
			content.WriteString(`</ul>`)
		}
	case strings.HasSuffix(section, "/receipt") || strings.Contains(section, "/refunds/"):
		parts := strings.Split(strings.Trim(section, "/"), "/")
		if len(parts) >= 3 {
			var receipt string
			var e error
			if len(parts) == 3 {
				receipt, e = h.pos.SaleReceipt(ctx, business.ID, parts[1])
			} else {
				receipt, e = h.pos.RefundReceipt(ctx, business.ID, parts[1], parts[3])
			}
			if e == nil {
				fmt.Fprintf(&content, `<h1>Receipt</h1><pre>%s</pre>`, esc(receipt))
			} else {
				content.WriteString(`<h1>Receipt not found</h1>`)
			}
		}
	default:
		content.WriteString(`<h1>Page not found</h1>`)
	}
	return dashboardHTML(esc(business.Name)+" · Afika", content.String(), section)
}

func (h *Handler) checkoutForm(products []posstore.Product, customers []posstore.Customer) string {
	var b strings.Builder
	b.WriteString(`<form method="post" action="/dashboard/checkout"><h2>Cart items</h2>`)
	for row := 1; row <= 3; row++ {
		fmt.Fprintf(&b, `<div class="fields"><div><label>Product %d<select name="productId"><option value="">Choose a product</option>`, row)
		for _, p := range products {
			fmt.Fprintf(&b, `<option value="%s">%s · %s · stock %d</option>`, esc(p.ID), esc(p.Name), money(p.Price), p.Stock)
		}
		fmt.Fprintf(&b, `</select></label></div><div><label>Quantity %d<input name="qty" type="number" min="1" value="1"></label></div></div>`, row)
	}
	b.WriteString(`<div class="fields"><div><label>Discount type<select name="discountType"><option value="none">No discount</option><option value="percent">Percent</option><option value="fixed">Fixed cents</option></select></label></div><div><label>Discount value<input name="discountValue" type="number" min="0" value="0"></label></div><div><label>Payment method<select name="paymentMethod"><option value="cash">Cash</option></select></label></div><div><label>Loyalty customer<select name="customerId"><option value="">No customer</option>`)
	for _, c := range customers {
		fmt.Fprintf(&b, `<option value="%s">%s · %d points</option>`, esc(c.ID), esc(c.Name), c.LoyaltyPoints)
	}
	b.WriteString(`</select></label></div><div><label>Redeem loyalty points<input name="redeemPoints" type="number" min="0" value="0"></label></div></div><button>Complete sale</button></form>`)
	return b.String()
}
func productTable(products []posstore.Product) string {
	if len(products) == 0 {
		return `<div class="empty">No products yet. Add your first product above.</div>`
	}
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Product</th><th>SKU</th><th>Price</th><th>In stock</th><th>Reorder at</th></tr></thead><tbody>`)
	for _, p := range products {
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%d</td></tr>`, esc(p.Name), esc(p.SKU), money(p.Price), p.Stock, p.ReorderLevel)
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}
func (h *Handler) salesTable(ctx context.Context, businessID string, sales []posstore.Sale) string {
	if len(sales) == 0 {
		return `<div class="empty">No sales recorded yet.</div>`
	}
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Receipt</th><th>Status</th><th>Total</th><th>Date</th><th></th></tr></thead><tbody>`)
	for _, s := range sales {
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td><a href="/dashboard/sales/%s">Open</a></td></tr>`, esc(s.ReceiptNo), esc(s.Status), money(s.Total), time.UnixMilli(s.CreatedAt).Format("2 Jan 15:04"), esc(s.ID))
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func dashboardHTML(title, content, path string) string {
	nav := []struct{ path, label string }{{"/overview", "Overview"}, {"/products", "Products"}, {"/categories", "Categories"}, {"/inventory", "Inventory"}, {"/stock-movements", "Stock movements"}, {"/cart", "Cart"}, {"/checkout", "Checkout"}, {"/sales", "Sales"}, {"/refunds", "Refunds"}, {"/voids", "Voids"}, {"/payments", "Payments"}, {"/loyalty", "Loyalty"}, {"/receipts", "Receipts"}, {"/reports", "Reports"}, {"/insights", "Insights"}}
	var links strings.Builder
	for _, n := range nav {
		active := ""
		if path == n.path || (n.path == "/sales" && strings.HasPrefix(path, "/sales/")) || (n.path == "/overview" && path == "/overview") {
			active = " active"
		}
		fmt.Fprintf(&links, `<a class="%s" href="/dashboard%s">%s</a>`, active, n.path, n.label)
	}
	body := `<main class="dash"><header class="dash-head"><a href="/dashboard"><strong>afika</strong> · Merchant dashboard</a><form method="post" action="/signout"><button>Sign out</button></form></header><div class="dash-grid"><nav class="dash-nav" aria-label="Dashboard" hx-boost="true">` + links.String() + `</nav><section class="dash-content">` + content + `</section></div></main>`
	return page(title, "Afika POS merchant dashboard", path, dashboardCSS+body)
}

func (h *Handler) dashboardError(w http.ResponseWriter, r *http.Request, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = io.WriteString(w, dashboardHTML("Dashboard error", `<h1>Could not save that change</h1><p class="notice">`+esc(message)+`</p><a class="action" href="/dashboard">Return to dashboard</a>`, r.URL.Path))
}
