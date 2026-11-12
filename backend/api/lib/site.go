package lib

import (
	"net/http"
	"strings"
)

const siteCSS = `
:root{color-scheme:light;--ink:#1C1C1E;--ink-soft:#2A2A2D;--paper:#FFFFFF;--canvas:#F4F5F7;--line:#ECEDEF;--muted:#747780;--lime:#D2E95F;--lime-line:#B7CF4A;--green:#3E9E78;--red:#CF4448;font-family:Inter,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:var(--ink);background:var(--canvas)}
*{box-sizing:border-box}body{margin:0}a{color:inherit} .wrap{width:min(1120px,calc(100% - 40px));margin:auto}.nav{height:76px;display:flex;align-items:center;justify-content:space-between}.brand{display:flex;align-items:center;gap:10px;text-decoration:none;font-weight:750;font-size:20px;letter-spacing:-.04em}.mark{display:grid;place-items:center;width:34px;height:34px;border-radius:11px;background:var(--lime);font-size:16px}.links{display:flex;align-items:center;gap:28px;font-size:14px}.links a,.footer a{color:var(--muted);text-decoration:none}.links a:hover,.footer a:hover{color:var(--ink)}.button{display:inline-flex;align-items:center;justify-content:center;min-height:48px;padding:0 20px;background:var(--ink);color:var(--lime);border-radius:14px;text-decoration:none;font-weight:650}.hero{padding:74px 0 86px;display:grid;grid-template-columns:1.05fr .95fr;gap:56px;align-items:center}.eyebrow{font-size:12px;letter-spacing:.14em;text-transform:uppercase;font-weight:750;color:#686b73}.hero h1{font-size:clamp(46px,7vw,78px);line-height:.98;letter-spacing:-.065em;margin:20px 0}.hero p,.lead{font-size:18px;line-height:1.65;color:#646770;max-width:560px}.hero-actions{display:flex;gap:12px;align-items:center;margin-top:28px}.subtle{padding:12px 4px;text-decoration:none;font-weight:650}.visual{min-height:420px;background:var(--ink);border-radius:32px;padding:30px;color:var(--paper);position:relative;overflow:hidden;box-shadow:0 28px 70px #1c1c1e20}.visual:after{content:"";position:absolute;width:360px;height:360px;right:-110px;top:-110px;background:var(--lime);border-radius:50%;opacity:.98}.visual-label{position:relative;z-index:1;color:#b9bbc0;font-size:13px}.balance{position:relative;z-index:1;font-size:42px;letter-spacing:-.05em;margin-top:8px;font-weight:700}.gain{position:relative;z-index:1;color:var(--lime);font-size:13px;margin-top:6px}.chart{position:absolute;inset:150px 0 30px;width:100%;height:220px}.ticker{position:absolute;z-index:1;bottom:28px;left:28px;right:28px;display:flex;justify-content:space-between;align-items:center;border-top:1px solid #ffffff28;padding-top:18px;font-size:13px}.ticker b{display:block;font-size:14px}.ticker span{color:#b9bbc0}.section{padding:76px 0}.section h2{font-size:clamp(32px,4vw,48px);letter-spacing:-.055em;margin:12px 0}.section-head{max-width:650px;margin-bottom:34px}.cards{display:grid;grid-template-columns:repeat(3,1fr);gap:16px}.card{background:var(--paper);border:1px solid var(--line);border-radius:22px;padding:25px}.card .num{font-size:12px;color:var(--muted);font-weight:700}.card h3{font-size:20px;margin:28px 0 8px;letter-spacing:-.03em}.card p{color:#686b73;line-height:1.6;margin:0}.band{background:var(--lime);border-radius:28px;padding:38px 42px;display:flex;align-items:center;justify-content:space-between;gap:24px}.band h2{font-size:clamp(28px,4vw,42px);margin:0;letter-spacing:-.05em}.band p{margin:8px 0 0;color:#41452d}.footer{padding:32px 0 42px;border-top:1px solid var(--line);margin-top:60px}.footer-inner{display:flex;justify-content:space-between;gap:24px;align-items:center;font-size:13px}.footer-links{display:flex;gap:20px}.legal{max-width:780px;padding:58px 0 30px}.legal h1{font-size:clamp(42px,6vw,64px);letter-spacing:-.06em;margin:14px 0}.legal h2{margin:36px 0 10px;letter-spacing:-.03em}.legal p,.legal li{color:#5e616a;line-height:1.75}.legal .updated{font-size:14px;color:var(--muted)}.support-box{background:var(--paper);border:1px solid var(--line);border-radius:24px;padding:28px;margin-top:32px}.support-box a{font-weight:700}.faq{border-top:1px solid var(--line);padding:20px 0}.faq h3{margin:0 0 8px}.faq p{margin:0;color:#646770;line-height:1.65} @media(max-width:760px){.links{gap:14px}.links .optional{display:none}.hero{grid-template-columns:1fr;padding:48px 0 58px;gap:34px}.visual{min-height:350px}.chart{top:125px;height:200px}.cards{grid-template-columns:1fr}.card h3{margin-top:18px}.section{padding:55px 0}.band{align-items:flex-start;flex-direction:column;padding:30px}.footer-inner{align-items:flex-start;flex-direction:column}.legal{padding-top:40px}} @media(prefers-reduced-motion:no-preference){html{scroll-behavior:smooth}}
`

const siteHead = `
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="theme-color" content="#F4F5F7"><meta name="application-name" content="Afika">
<meta property="og:site_name" content="Afika"><meta property="og:type" content="website">
<meta name="twitter:card" content="summary">
<meta property="og:image" content="https://pos.phumzile.studio/social-card.svg"><meta name="twitter:image" content="https://pos.phumzile.studio/social-card.svg">
<meta property="og:image:alt" content="Afika POS — sales, stock, and profit in one place">
<meta name="robots" content="index,follow,max-image-preview:large">
<link rel="preconnect" href="https://unpkg.com"><script src="https://unpkg.com/htmx.org@2.0.4" defer></script>
<style>` + siteCSS + `</style>
`

func siteNav() string {
	return `<header class="wrap nav"><a class="brand" href="/" aria-label="Afika home"><span class="mark" aria-hidden="true">a</span>afika</a><nav class="links" aria-label="Main navigation"><a href="/#how">How it works</a><a class="optional" href="/support">Support</a><a class="button" href="/#get-started">Get the app</a></nav></header>`
}

func siteFooter() string {
	return `<footer class="wrap footer"><div class="footer-inner"><a class="brand" href="/"><span class="mark" aria-hidden="true">a</span>afika</a><span>© 2026 Afika POS. Built for independent shops.</span><nav class="footer-links" aria-label="Footer"><a href="/terms">Terms</a><a href="/privacy">Privacy</a><a href="/support">Support</a></nav></div></footer>`
}

func page(title, description, path, body string) string {
	canonical := "https://pos.phumzile.studio" + path
	return `<!doctype html><html lang="en"><head><title>` + title + `</title>` + siteHead +
		`<meta name="description" content="` + description + `"><link rel="canonical" href="` + canonical + `">` +
		`<meta property="og:title" content="` + title + `"><meta property="og:description" content="` + description + `"><meta property="og:url" content="` + canonical + `"><meta name="twitter:title" content="` + title + `"><meta name="twitter:description" content="` + description + `"></head><body>` + body + `</body></html>`
}

func homePage() string {
	body := siteNav() + `<main><section class="wrap hero"><div><div class="eyebrow">A simpler way to run your shop</div><h1>Every sale.<br>In control.</h1><p>Afika POS helps independent South African shops sell faster, keep stock accurate, and understand what they earn—even when the connection drops.</p><div class="hero-actions"><a class="button" href="#get-started">Explore Afika POS</a><a class="subtle" href="#how">See how it works ↓</a></div></div><div class="visual" aria-label="Afika point of sale dashboard preview"><div class="visual-label">Today’s takings · ZAR</div><div class="balance">R 4,820.50</div><div class="gain">12 sales · 3 items running low</div><div class="ticker"><div><b>Sell</b><span>Fast checkout</span></div><div><b>Track</b><span>Stock and profit</span></div></div></div></section><section class="section" id="how"><div class="wrap"><div class="section-head"><div class="eyebrow">Built around the counter</div><h2>Know what sold and what’s left.</h2><p class="lead">A practical till and back office for cafés, market stalls, and neighbourhood retailers.</p></div><div class="cards"><article class="card"><span class="num">01 / SELL</span><h3>Checkout that keeps moving</h3><p>Search products, build a cart, apply discounts, and share a clear receipt with your customer.</p></article><article class="card"><span class="num">02 / MANAGE</span><h3>Stock you can trust</h3><p>Record restocks and adjustments, spot low items, and keep a useful movement history.</p></article><article class="card"><span class="num">03 / UNDERSTAND</span><h3>See your real margin</h3><p>Review sales, returns, cost of goods, and profit without stitching together paper records.</p></article></div></div></section><section class="wrap" id="get-started"><div class="band"><div><h2>Keep your shop moving.</h2><p>Start on your phone and use Afika’s online checkout for shareable payments.</p></div><a class="button" href="/support">Get the app</a></div></section><section class="wrap section"><p class="lead" style="font-size:14px">Afika POS stores till data on your device so you can keep serving customers offline. Online crypto checkout requires an internet connection and a configured receiving wallet.</p></section></main>` + siteFooter()
	return page("Afika POS — Sales, stock, and profit in one place", "A point-of-sale app for independent South African shops. Sell, track stock, manage returns, and understand your profit.", "/", body)
}

func legalPage(kind string) string {
	title := "Terms of Service"
	desc := "Review the terms for using Afika POS."
	path := "/terms"
	content := `<p>These terms apply when you use the Afika POS app, website, and online checkout. By using the service, you agree to these terms.</p><h2>Using Afika POS</h2><p>Afika POS provides tools for recording sales, products, stock, customers, and receipts. Merchants are responsible for the accuracy of their records, their tax and consumer obligations, and securing access to their devices and accounts.</p><h2>Offline data and online services</h2><p>The till can store data on the device while offline. Keep your own backups and protect the device: locally stored data may be lost if the device is damaged, reset, or unavailable. Online checkout, payment status updates, and account services need an internet connection and may be unavailable temporarily.</p><h2>Payments</h2><p>Online crypto checkout displays the amount and network selected for a payment request. Customers must verify the network, destination address, and amount before sending. Blockchain transfers may be irreversible. Afika does not hold customer funds or guarantee blockchain confirmation times.</p><h2>Availability and changes</h2><p>We may change or suspend parts of the service to maintain or protect it. We will make reasonable efforts to keep the service available, but do not promise uninterrupted operation.</p><h2>Contact</h2><p>For questions about these terms, visit <a href="/support">Afika Support</a>.</p>`
	if kind == "privacy" {
		title, desc, path = "Privacy Policy", "Learn how Afika POS handles account, business, and checkout information.", "/privacy"
		content = `<p>This policy describes information Afika handles to operate the POS app, website, and online checkout.</p><h2>Information we handle</h2><p>This can include business details entered for receipts, products and sales stored on your device, Privy account identifiers when you sign in, and payment request details needed to create checkout links and report payment status. Public blockchain transactions are visible on their networks.</p><h2>How we use it</h2><p>We use this information to provide checkout and account features, keep payment status current, protect the service, troubleshoot errors, and meet legal obligations.</p><h2>Service providers</h2><p>Privy provides account services. Cloudflare hosts the API and payment request data. Blockchain networks provide public transaction records. These providers process information under their own service terms and privacy practices.</p><h2>Retention and choices</h2><p>Device records remain on your device until you remove them or reset the app. Server payment records are retained as needed to operate checkout and support payment reconciliation. Contact support for privacy questions or requests.</p><h2>Contact</h2><p>For privacy questions, visit <a href="/support">Afika Support</a>.</p>`
	}
	body := siteNav() + `<main class="wrap legal"><div class="eyebrow">Afika / Legal</div><h1>` + title + `</h1><p class="updated">Last updated: November 12, 2026</p>` + content + `</main>` + siteFooter()
	return page(title+" — Afika POS", desc, path, body)
}

func supportPage() string {
	body := siteNav() + `<main class="wrap legal"><div class="eyebrow">We’re here to help</div><h1>Afika POS Support</h1><p class="lead">Get help with your till, stock records, Privy sign-in, or an online checkout.</p><section class="support-box"><h2>Contact our team</h2><p>For app, account, payment, or privacy questions, email our support team.</p><a href="mailto:support@afika.app">support@afika.app</a></section><section style="margin-top:44px"><div class="faq"><h3>Does Afika POS work offline?</h3><p>Your till and stock records are stored on the device, so core counter tasks continue without a connection. Online checkout and payment status need internet access.</p></div><div class="faq"><h3>How do online payments work?</h3><p>Create a checkout link from the app, then share it with your customer. They choose an asset and network on the hosted checkout page.</p></div><div class="faq"><h3>What if a customer has sent a payment?</h3><p>Keep the checkout open while the receipt service checks the transaction. If the payment does not appear, contact support with the receipt reference.</p></div><div class="faq"><h3>Where can I read the legal information?</h3><p>See our <a href="/terms">Terms of Service</a> and <a href="/privacy">Privacy Policy</a>.</p></div></section></main>` + siteFooter()
	return page("Support — Afika POS", "Get help with Afika POS, business records, and online checkout.", "/support", body)
}

func serveSite(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/" && r.URL.Path != "/terms" && r.URL.Path != "/privacy" && r.URL.Path != "/support" && r.URL.Path != "/robots.txt" && r.URL.Path != "/sitemap.xml" && r.URL.Path != "/social-card.svg" && r.URL.Path != "/llm.txt" && r.URL.Path != "/llms.txt" {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return true
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=300, s-maxage=3600")
	var contentType, content string
	switch r.URL.Path {
	case "/":
		contentType, content = "text/html; charset=utf-8", homePage()
	case "/terms":
		contentType, content = "text/html; charset=utf-8", legalPage("terms")
	case "/privacy":
		contentType, content = "text/html; charset=utf-8", legalPage("privacy")
	case "/support":
		contentType, content = "text/html; charset=utf-8", supportPage()
	case "/robots.txt":
		contentType, content = "text/plain; charset=utf-8", "User-agent: *\nAllow: /\nDisallow: /v1/\n\nSitemap: https://pos.phumzile.studio/sitemap.xml\n"
	case "/sitemap.xml":
		contentType, content = "application/xml; charset=utf-8", `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://pos.phumzile.studio/</loc></url><url><loc>https://pos.phumzile.studio/terms</loc></url><url><loc>https://pos.phumzile.studio/privacy</loc></url><url><loc>https://pos.phumzile.studio/support</loc></url></urlset>`
	case "/social-card.svg":
		contentType, content = "image/svg+xml; charset=utf-8", `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630" viewBox="0 0 1200 630"><rect width="1200" height="630" fill="#F4F5F7"/><circle cx="1050" cy="80" r="280" fill="#D2E95F"/><rect x="92" y="104" width="74" height="74" rx="22" fill="#D2E95F"/><text x="129" y="158" text-anchor="middle" font-family="Arial,sans-serif" font-size="48" font-weight="700" fill="#1C1C1E">a</text><text x="190" y="158" font-family="Arial,sans-serif" font-size="44" font-weight="700" fill="#1C1C1E">afika</text><text x="92" y="340" font-family="Arial,sans-serif" font-size="86" font-weight="700" letter-spacing="-4" fill="#1C1C1E">Your money.</text><text x="92" y="435" font-family="Arial,sans-serif" font-size="86" font-weight="700" letter-spacing="-4" fill="#1C1C1E">In motion.</text><text x="96" y="510" font-family="Arial,sans-serif" font-size="28" fill="#747780">Sales, stock, and profit in one place.</text></svg>`
	case "/llm.txt", "/llms.txt":
		contentType, content = "text/plain; charset=utf-8", "# Afika\n\nAfika POS is a point-of-sale app for independent South African shops. It helps merchants manage checkout, products, stock, sales, returns, and profit.\n\n## Pages\n- [Home](https://pos.phumzile.studio/): Product overview\n- [Terms](https://pos.phumzile.studio/terms): Terms of Service\n- [Privacy](https://pos.phumzile.studio/privacy): Privacy Policy\n- [Support](https://pos.phumzile.studio/support): Help and frequently asked questions\n"
	}
	w.Header().Set("Content-Type", contentType)
	if r.Method == http.MethodHead {
		return true
	}
	_, _ = w.Write([]byte(strings.TrimSpace(content)))
	return true
}
