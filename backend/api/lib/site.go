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
<meta property="og:image" content="https://afika-api.seni-tembe.workers.dev/social-card.svg"><meta name="twitter:image" content="https://afika-api.seni-tembe.workers.dev/social-card.svg">
<meta property="og:image:alt" content="Afika — investing, made more accessible">
<meta name="robots" content="index,follow,max-image-preview:large">
<link rel="preconnect" href="https://unpkg.com"><script src="https://unpkg.com/htmx.org@2.0.4" defer></script>
<style>` + siteCSS + `</style>
`

func siteNav() string {
	return `<header class="wrap nav"><a class="brand" href="/" aria-label="Afika home"><span class="mark" aria-hidden="true">a</span>afika</a><nav class="links" aria-label="Main navigation"><a href="/#how">How it works</a><a class="optional" href="/support">Support</a><a class="button" href="/#get-started">Get the app</a></nav></header>`
}

func siteFooter() string {
	return `<footer class="wrap footer"><div class="footer-inner"><a class="brand" href="/"><span class="mark" aria-hidden="true">a</span>afika</a><span>© 2026 Afika. Investing involves risk.</span><nav class="footer-links" aria-label="Footer"><a href="/terms">Terms</a><a href="/privacy">Privacy</a><a href="/support">Support</a></nav></div></footer>`
}

func page(title, description, path, body string) string {
	canonical := "https://afika-api.seni-tembe.workers.dev" + path
	return `<!doctype html><html lang="en"><head><title>` + title + `</title>` + siteHead +
		`<meta name="description" content="` + description + `"><link rel="canonical" href="` + canonical + `">` +
		`<meta property="og:title" content="` + title + `"><meta property="og:description" content="` + description + `"><meta property="og:url" content="` + canonical + `"><meta name="twitter:title" content="` + title + `"><meta name="twitter:description" content="` + description + `"></head><body>` + body + `</body></html>`
}

func homePage() string {
	body := siteNav() + `<main><section class="wrap hero"><div><div class="eyebrow">A clearer way to invest</div><h1>Your money.<br>In motion.</h1><p>Explore investing with a simple, thoughtful experience designed to help you build confidence one step at a time.</p><div class="hero-actions"><a class="button" href="#get-started">Get the Afika app</a><a class="subtle" href="#how">Discover Afika ↓</a></div></div><div class="visual" aria-label="Illustration of an investment portfolio"><div class="visual-label">Portfolio overview</div><div class="balance">$2,480.60</div><div class="gain">↗ Your portfolio at a glance</div><svg class="chart" viewBox="0 0 600 240" preserveAspectRatio="none" role="img" aria-label="Illustrative upward portfolio chart"><defs><linearGradient id="fill" x1="0" x2="0" y1="0" y2="1"><stop offset="0" stop-color="#D2E95F" stop-opacity=".28"/><stop offset="1" stop-color="#D2E95F" stop-opacity="0"/></linearGradient></defs><path d="M0 190 C55 180 64 142 110 154 S174 172 210 129 S270 145 310 100 S385 130 420 77 S480 91 520 48 S570 64 600 20 V240 H0Z" fill="url(#fill)"/><path d="M0 190 C55 180 64 142 110 154 S174 172 210 129 S270 145 310 100 S385 130 420 77 S480 91 520 48 S570 64 600 20" fill="none" stroke="#D2E95F" stroke-width="4" stroke-linecap="round"/></svg><div class="ticker"><div><b>Explore</b><span>Discover opportunities</span></div><div><b>Track</b><span>See your progress</span></div></div></div></section><section class="section" id="how"><div class="wrap"><div class="section-head"><div class="eyebrow">Made for your next step</div><h2>Investing can feel simpler.</h2><p class="lead">Afika brings the essentials together in one welcoming place, so you can explore at your own pace.</p></div><div class="cards"><article class="card"><span class="num">01 / EXPLORE</span><h3>Find what interests you</h3><p>Browse investment opportunities and learn the basics with clear, approachable information.</p></article><article class="card"><span class="num">02 / BUILD</span><h3>Make it your portfolio</h3><p>Keep your investments and activity together in a view that is easy to understand.</p></article><article class="card"><span class="num">03 / LEARN</span><h3>Grow your confidence</h3><p>Take time to learn, review your choices, and make decisions that fit your goals.</p></article></div></div></section><section class="wrap" id="get-started"><div class="band"><div><h2>Take your next step with Afika.</h2><p>Explore the app and see if it’s right for you.</p></div><a class="button" href="/support">App availability</a></div></section><section class="wrap section"><p class="lead" style="font-size:14px">Investing involves risk, including the possible loss of your investment. Afika does not provide investment advice. Features and availability may vary by region. Afika is not available to US persons.</p></section></main>` + siteFooter()
	return page("Afika — Investing, made more accessible", "Explore investing with Afika, a thoughtful app designed to make your portfolio easier to understand.", "/", body)
}

func legalPage(kind string) string {
	title := "Terms of Service"
	desc := "Review the terms for using Afika."
	path := "/terms"
	content := `<p>These terms describe your use of the Afika app and related services. By using Afika, you agree to these terms. If you do not agree, do not use the service.</p><h2>Who can use Afika</h2><p>You must be legally able to enter into an agreement where you live and meet the eligibility requirements shown in the app. Afika is not available to US persons. We may restrict or suspend access where required by law or to protect the service.</p><h2>Investment risk</h2><p>Investments can rise or fall in value, and you may lose some or all of the money you invest. Past performance does not predict future results. Information in Afika is for general information and is not financial, legal, tax, or investment advice. You are responsible for your decisions.</p><h2>Your account and security</h2><p>Keep your device and account credentials secure. You are responsible for activity initiated through your account and should contact support promptly if you believe it has been compromised.</p><h2>Availability and changes</h2><p>We may update, suspend, or discontinue features. Network, market, and third-party services can affect availability, timing, and displayed information.</p><h2>Contact</h2><p>For questions about these terms, visit <a href="/support">Afika Support</a>.</p>`
	if kind == "privacy" {
		title, desc, path = "Privacy Policy", "Learn how Afika handles information when you use the app and website.", "/privacy"
		content = `<p>This policy explains the categories of information Afika may handle when you use our app and website, and the choices available to you. It is a general summary; we will publish the full entity and jurisdiction-specific details before collecting information through a public release.</p><h2>Information we handle</h2><p>Depending on the features you use, this may include account and eligibility details, wallet addresses, device and diagnostic information, and app activity needed to operate the service. Public blockchain data may be visible to anyone and is not controlled by Afika.</p><h2>How information is used</h2><p>We use information to provide and secure the service, display your portfolio and transaction activity, troubleshoot issues, meet legal obligations, and improve the app.</p><h2>Sharing and retention</h2><p>We may share information with service providers who help operate Afika, or when required by law. We retain information for as long as needed for these purposes and applicable legal requirements.</p><h2>Your choices</h2><p>You can stop using the service and contact support with privacy questions or requests. Some information may need to be retained to meet legal, security, or transaction record requirements.</p><h2>Contact</h2><p>For privacy questions, visit <a href="/support">Afika Support</a>.</p>`
	}
	body := siteNav() + `<main class="wrap legal"><div class="eyebrow">Afika / Legal</div><h1>` + title + `</h1><p class="updated">Last updated: October 31, 2026</p>` + content + `</main>` + siteFooter()
	return page(""+title+" — Afika", desc, path, body)
}

func supportPage() string {
	body := siteNav() + `<main class="wrap legal"><div class="eyebrow">We’re here to help</div><h1>Afika Support</h1><p class="lead">Find answers to common questions or get help with your app experience.</p><section class="support-box"><h2>Contact our team</h2><p>For account, app, or privacy questions, email our support team.</p><a href="mailto:support@afika.app">support@afika.app</a></section><section style="margin-top:44px"><div class="faq"><h3>Where can I download Afika?</h3><p>App availability depends on your region. Check the Apple App Store or Google Play for Afika.</p></div><div class="faq"><h3>Is investing risk-free?</h3><p>No. The value of investments can go down as well as up, and you may lose the amount invested.</p></div><div class="faq"><h3>Can US persons use Afika?</h3><p>Afika is not available to US persons. Eligibility requirements are shown in the app.</p></div><div class="faq"><h3>Where can I read the legal information?</h3><p>See our <a href="/terms">Terms of Service</a> and <a href="/privacy">Privacy Policy</a>.</p></div></section></main>` + siteFooter()
	return page("Support — Afika", "Get help with the Afika app, account access, and common questions.", "/support", body)
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
		contentType, content = "text/plain; charset=utf-8", "User-agent: *\nAllow: /\nDisallow: /v1/\n\nSitemap: https://afika-api.seni-tembe.workers.dev/sitemap.xml\n"
	case "/sitemap.xml":
		contentType, content = "application/xml; charset=utf-8", `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://afika-api.seni-tembe.workers.dev/</loc></url><url><loc>https://afika-api.seni-tembe.workers.dev/terms</loc></url><url><loc>https://afika-api.seni-tembe.workers.dev/privacy</loc></url><url><loc>https://afika-api.seni-tembe.workers.dev/support</loc></url></urlset>`
	case "/social-card.svg":
		contentType, content = "image/svg+xml; charset=utf-8", `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630" viewBox="0 0 1200 630"><rect width="1200" height="630" fill="#F4F5F7"/><circle cx="1050" cy="80" r="280" fill="#D2E95F"/><rect x="92" y="104" width="74" height="74" rx="22" fill="#D2E95F"/><text x="129" y="158" text-anchor="middle" font-family="Arial,sans-serif" font-size="48" font-weight="700" fill="#1C1C1E">a</text><text x="190" y="158" font-family="Arial,sans-serif" font-size="44" font-weight="700" fill="#1C1C1E">afika</text><text x="92" y="340" font-family="Arial,sans-serif" font-size="86" font-weight="700" letter-spacing="-4" fill="#1C1C1E">Your money.</text><text x="92" y="435" font-family="Arial,sans-serif" font-size="86" font-weight="700" letter-spacing="-4" fill="#1C1C1E">In motion.</text><text x="96" y="510" font-family="Arial,sans-serif" font-size="28" fill="#747780">Investing, made more accessible.</text></svg>`
	case "/llm.txt", "/llms.txt":
		contentType, content = "text/plain; charset=utf-8", "# Afika\n\nAfika is an investing app designed to make portfolio exploration and tracking easier to understand. Investing involves risk, including possible loss. Afika does not provide investment advice and is not available to US persons.\n\n## Pages\n- [Home](https://afika-api.seni-tembe.workers.dev/): Product overview\n- [Terms](https://afika-api.seni-tembe.workers.dev/terms): Terms of Service\n- [Privacy](https://afika-api.seni-tembe.workers.dev/privacy): Privacy Policy\n- [Support](https://afika-api.seni-tembe.workers.dev/support): Help and frequently asked questions\n"
	}
	w.Header().Set("Content-Type", contentType)
	if r.Method == http.MethodHead {
		return true
	}
	_, _ = w.Write([]byte(strings.TrimSpace(content)))
	return true
}
