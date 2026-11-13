package lib

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/mindsgn-studio/afika-backend/api/internal/posdomain"
)

// payAssetOrder fixes the order the payer sees on the checkout page. Labels
// mirror the POS app's payment options.
var payAssetOrder = []struct {
	ID    string
	Label string
	Sub   string
}{
	{"usdc-base", "USDC", "Base"},
	{"usdc-ethereum", "USDC", "Ethereum"},
	{"zarp-base", "ZARP", "Base"},
	{"zarp-ethereum", "ZARP", "Ethereum"},
	{"eurc-base", "EURC", "Base"},
	{"eurc-ethereum", "EURC", "Ethereum"},
	{"btc", "BTC", "Bitcoin"},
}

type payOptionView struct {
	ID       string
	Label    string
	Sub      string
	Selected bool
}

type payPageView struct {
	Payment       POSPayment
	Amount        string
	Options       []payOptionView
	StatusClass   string
	Headline      string
	Message       string
	Merchant      string
	MerchantMeta  string
	CryptoDisplay string
	HasAddress    bool
	Paid          bool
}

var payPageTemplate = template.Must(template.New("pay").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex,nofollow"><title>Pay {{.Amount}} — {{.Merchant}}</title>
<style>
:root{color-scheme:light;--ink:#1C1C1E;--soft:#646770;--line:#E7E8EA;--paper:#fff;--canvas:#F4F5F7;--lime:#D2E95F;--green:#3E9E78;font-family:Inter,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
*{box-sizing:border-box}body{margin:0;background:var(--canvas);color:var(--ink);display:flex;justify-content:center;padding:24px 16px}
.card{width:min(460px,100%);background:var(--paper);border-radius:26px;padding:28px;box-shadow:0 18px 50px #1c1c1e14}
.brand{display:flex;align-items:center;gap:10px;font-weight:750;letter-spacing:-.03em;text-decoration:none;color:var(--ink);margin-bottom:26px}
.mark{display:grid;place-items:center;width:32px;height:32px;border-radius:10px;background:var(--lime);font-size:15px}
.eyebrow{font-size:11px;letter-spacing:.14em;text-transform:uppercase;color:var(--soft);font-weight:700}
.amount{font-size:40px;letter-spacing:-.05em;margin:8px 0 2px;font-weight:720}
.for{color:var(--soft);font-size:14px;margin:0 0 22px}
.banner{border-radius:16px;padding:14px 16px;font-size:14px;margin-bottom:20px;background:#F1F6EC;color:#2C6B4F;border:1px solid #D9E8CE}
.banner.error{background:#FBEFEF;color:#8E2F33;border-color:#F1D5D6}
.banner.neutral{background:var(--canvas);color:var(--soft);border-color:var(--line)}
h2{font-size:12px;text-transform:uppercase;letter-spacing:.12em;color:var(--soft);margin:0 0 12px}
.grid{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-bottom:22px}
button.opt{display:flex;flex-direction:column;align-items:flex-start;gap:2px;padding:12px 14px;border-radius:14px;border:1px solid var(--line);background:var(--paper);cursor:pointer;font:inherit;text-align:left}
button.opt b{font-size:15px}button.opt span{font-size:12px;color:var(--soft)}
button.opt.selected{border-color:var(--ink);background:#FBFCEC;box-shadow:inset 0 0 0 1px var(--ink)}
.detail{display:flex;justify-content:space-between;gap:16px;padding:12px 0;border-top:1px solid var(--line);font-size:14px}
.detail span:first-child{color:var(--soft)}
.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px;word-break:break-all;text-align:right;max-width:62%}
.copy{margin-top:14px;display:flex;gap:10px}
.copy button{flex:1;min-height:48px;border-radius:14px;border:0;background:var(--ink);color:var(--lime);font-weight:650;font:inherit;cursor:pointer}
.foot{color:var(--soft);font-size:12px;margin-top:22px;text-align:center;line-height:1.6}
</style></head><body>
<main class="card">
<a class="brand" href="/"><span class="mark" aria-hidden="true">a</span>afika pay</a>
<div class="eyebrow">Amount due</div>
<div class="amount">{{.Amount}}</div>
<p class="for">{{.Merchant}}{{if .MerchantMeta}} · {{.MerchantMeta}}{{end}}</p>
<div class="banner {{.StatusClass}}">{{.Headline}}</div>
{{if .Paid}}
<div class="detail"><span>Reference</span><span class="mono">{{.Payment.ReceiptNo}}</span></div>
{{if .Payment.TxHash}}<div class="detail"><span>Transaction</span><span class="mono">{{.Payment.TxHash}}</span></div>{{end}}
{{else}}
<form method="post" action="">
<h2>Choose how to pay</h2>
<div class="grid">
{{range .Options}}<button class="opt{{if .Selected}} selected{{end}}" type="submit" name="asset" value="{{.ID}}"><b>{{.Label}}</b><span>{{.Sub}}</span></button>{{end}}
</div>
</form>
<div class="detail"><span>Send exactly</span><span class="mono">{{.CryptoDisplay}}</span></div>
<div class="detail"><span>Network</span><span>{{.Payment.Network}}</span></div>
<div class="detail"><span>Reference</span><span class="mono">{{.Payment.ReceiptNo}}</span></div>
{{if .HasAddress}}<h2 style="margin-top:20px">Payment address</h2><div class="detail" style="border-top:0;padding-top:0"><span>Address</span><span class="mono" id="address">{{.Payment.Address}}</span></div>
<div class="copy"><button type="button" onclick="copyAddress()">Copy address</button></div>
<script>
function copyAddress(){var t=document.getElementById('address').textContent.trim();if(navigator.clipboard){navigator.clipboard.writeText(t);}}

</script>
{{else}}<p class="banner neutral">The merchant has not configured a receiving address yet. Ask them to complete their wallet setup in the POS app.</p>{{end}}
{{end}}
{{if not .Paid}}<div class="payment-poll" hx-get="/pay/{{.Payment.ID}}/status" hx-trigger="every 5s" hx-swap="none" aria-hidden="true"></div>{{end}}
<p class="foot">Secure checkout by Afika. Crypto amounts are locked to the displayed rate for 30 minutes.</p>
</main></body></html>`))

// servePayPage renders the public checkout page at /pay/{id}. It owns every
// request under /pay/ so it must run before the static site handler.
func (h *Handler) servePayPage(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/pay/") {
		return false
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/pay/"), "/")
	statusPoll := strings.HasSuffix(id, "/status")
	if statusPoll {
		id = strings.TrimSuffix(id, "/status")
	}
	if id == "" || strings.Contains(id, "/") || len(id) > 64 {
		h.renderPayError(w, errors.New("This payment link is not valid."))
		return true
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex")

	if statusPoll {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		payment, apiErr := h.loadPOSPayment(r, id)
		if apiErr != nil {
			h.renderPayError(w, errors.New(apiErr.message))
			return true
		}
		if payment.Status != posStatusPending {
			w.Header().Set("HX-Redirect", "/pay/"+id)
		}
		w.WriteHeader(http.StatusNoContent)
		return true
	}

	var assetChoice string
	switch r.Method {
	case http.MethodGet:
	case http.MethodHead:
		return true
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form submission", http.StatusBadRequest)
			return true
		}
		assetChoice = r.PostFormValue("asset")
		if _, err := h.applyPayAsset(r, id, assetChoice); err != nil {
			h.renderPayError(w, err)
			return true
		}
		http.Redirect(w, r, "/pay/"+id, http.StatusSeeOther)
		return true
	default:
		w.Header().Set("Allow", "GET, HEAD, POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return true
	}

	payment, apiErr := h.loadPOSPayment(r, id)
	if apiErr != nil {
		h.renderPayError(w, errors.New(apiErr.message))
		return true
	}
	h.renderPayPage(w, payment)
	return true
}

func (h *Handler) applyPayAsset(r *http.Request, id, rawAsset string) (POSPayment, error) {
	assetID, asset, err := normalizePOSAsset(rawAsset)
	if err != nil {
		return POSPayment{}, err
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()
	payment, err := h.store.GetPOSPayment(ctx, id)
	if errors.Is(err, ErrPOSPaymentNotFound) {
		return POSPayment{}, errors.New("This payment link is not valid.")
	}
	if err != nil {
		return POSPayment{}, err
	}
	if payment.Status != posStatusPending {
		return payment, nil
	}
	payment.Asset = assetID
	payment.Network = asset.Network
	cryptoAmount, err := cryptoAmountFor(payment.AmountCents, asset)
	if err != nil {
		return POSPayment{}, err
	}
	payment.CryptoAmount = cryptoAmount
	payment.UpdatedAt = time.Now().UnixMilli()
	return h.store.UpdatePOSPayment(ctx, payment)
}

func (h *Handler) renderPayPage(w http.ResponseWriter, p POSPayment) {
	options := make([]payOptionView, 0, len(payAssetOrder))
	for _, o := range payAssetOrder {
		options = append(options, payOptionView{ID: o.ID, Label: o.Label, Sub: o.Sub, Selected: o.ID == p.Asset})
	}

	asset := posdomain.AssetByID(p.Asset)
	crypto := p.CryptoAmount
	if strings.TrimSpace(crypto) == "" {
		if amount, err := cryptoAmountFor(p.AmountCents, asset); err == nil {
			crypto = amount
		}
	}
	view := payPageView{
		Payment:       p,
		Amount:        formatZAR(p.AmountCents),
		Options:       options,
		Merchant:      defaultIfBlank(p.MerchantName, "Afika merchant"),
		CryptoDisplay: crypto + " " + asset.Asset,
		HasAddress:    strings.TrimSpace(p.Address) != "",
	}
	meta := []string{}
	if strings.TrimSpace(p.MerchantAddress) != "" {
		meta = append(meta, p.MerchantAddress)
	}
	if strings.TrimSpace(p.MerchantWebsite) != "" {
		meta = append(meta, p.MerchantWebsite)
	}
	view.MerchantMeta = strings.Join(meta, " · ")

	switch p.Status {
	case posStatusPaid:
		view.Paid = true
		view.StatusClass = ""
		view.Headline = "Payment received. Thank you!"
	case posStatusExpired:
		view.StatusClass = "neutral"
		view.Headline = "This payment link has expired. Ask the merchant for a new one."
	case posStatusFailed:
		view.StatusClass = "error"
		view.Headline = "This payment did not complete. Ask the merchant for a new link."
	default:
		view.Headline = fmt.Sprintf("Pay in %s. Select an asset, then send the exact amount.", asset.Asset)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = payPageTemplate.Execute(w, view)
}

func (h *Handler) renderPayError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex"><title>Payment link not found</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#F4F5F7;font-family:Inter,-apple-system,sans-serif;color:#1C1C1E;padding:24px}div{background:#fff;border-radius:24px;padding:32px;max-width:420px;box-shadow:0 18px 50px #1c1c1e14}h1{font-size:24px;letter-spacing:-.04em;margin:0 0 10px}p{color:#646770;line-height:1.6;margin:0}</style></head><body><div><h1>Payment link not found</h1><p>` + template.HTMLEscapeString(err.Error()) + `</p></div></body></html>`))
}

func formatZAR(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%sR%d.%02d", sign, cents/100, cents%100)
}

func defaultIfBlank(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
