package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"checker-parser-service/internal/api"
)

const DefaultURL = "https://afisha.yandex.ru/samara/selections/hot?source=selection-events&city=samara"

type YandexParser struct {
	Client         *http.Client
	UseBrowser     bool
	BrowserTimeout time.Duration
	BrowserBin     string
}

type ParseMeta struct {
	Source          string   `json:"source"`
	URL             string   `json:"url"`
	HTTPEvents      int      `json:"httpEvents"`
	BrowserEvents   int      `json:"browserEvents"`
	Warnings        []string `json:"warnings,omitempty"`
	RenderedHTMLLen int      `json:"renderedHtmlLen,omitempty"`
}

type ParseResult struct {
	Count  int         `json:"count"`
	Events []api.Event `json:"events"`
	Meta   ParseMeta   `json:"meta"`
}

type jsonLD struct {
	Context     any    `json:"@context"`
	Type        any    `json:"@type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	StartDate   string `json:"startDate"`
	EndDate     string `json:"endDate"`
	Image       any    `json:"image"`
	URL         string `json:"url"`
	Location    any    `json:"location"`
	Offers      any    `json:"offers"`
	Organizer   any    `json:"organizer"`
}

func (p YandexParser) Parse(ctx context.Context, rawURL string) ([]api.Event, error) {
	result, err := p.ParseDetailed(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return result.Events, nil
}

func (p YandexParser) ParseDetailed(ctx context.Context, rawURL string) (ParseResult, error) {
	if rawURL == "" {
		rawURL = DefaultURL
	}
	meta := ParseMeta{Source: "yandex-afisha", URL: rawURL}

	htmlSrc, err := p.fetchHTML(ctx, rawURL)
	if err != nil {
		meta.Warnings = append(meta.Warnings, "HTTP fetch failed: "+err.Error())
	} else {
		events := parseHTML(htmlSrc, rawURL)
		meta.HTTPEvents = len(events)
		if len(events) > 0 || !p.UseBrowser {
			return ParseResult{Count: len(events), Events: events, Meta: meta}, nil
		}
	}

	if !p.UseBrowser {
		return ParseResult{Count: 0, Events: nil, Meta: meta}, nil
	}

	rendered, err := p.renderHTML(ctx, rawURL)
	if err != nil {
		return ParseResult{Meta: meta}, fmt.Errorf("browser render failed: %w", err)
	}
	meta.RenderedHTMLLen = len(rendered)
	events := parseHTML(rendered, rawURL)
	meta.BrowserEvents = len(events)
	if len(events) == 0 {
		meta.Warnings = append(meta.Warnings, "No events found after browser render. Yandex may have changed markup or blocked headless browser.")
	}
	return ParseResult{Count: len(events), Events: events, Meta: meta}, nil
}

func (p YandexParser) fetchHTML(ctx context.Context, rawURL string) (string, error) {
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	setBrowserHeaders(req.Header)
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("status %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 30<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (p YandexParser) renderHTML(ctx context.Context, rawURL string) (string, error) {
	timeout := p.BrowserTimeout
	if timeout == 0 {
		timeout = 45 * time.Second
	}
	bin, err := findBrowserBin(p.BrowserBin)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	profileDir, err := os.MkdirTemp("", "checker-chrome-profile-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(profileDir)
	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--disable-dev-shm-usage",
		"--disable-blink-features=AutomationControlled",
		"--window-size=1365,1600",
		"--lang=ru-RU",
		"--virtual-time-budget=8000",
		"--user-agent=" + browserUserAgent(),
		"--user-data-dir=" + profileDir,
		"--dump-dom",
		rawURL,
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("browser timeout after %s", timeout)
	}
	if err != nil {
		return "", fmt.Errorf("%s failed: %w: %s", filepath.Base(bin), err, truncate(string(out), 500))
	}
	return string(out), nil
}

func findBrowserBin(explicit string) (string, error) {
	candidates := []string{}
	if explicit != "" {
		candidates = append(candidates, explicit)
	}
	if env := os.Getenv("BROWSER_BIN"); env != "" {
		candidates = append(candidates, env)
	}
	candidates = append(candidates,
		"chromium-browser",
		"chromium",
		"google-chrome",
		"google-chrome-stable",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		`C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe`,
		`C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe`,
	)
	for _, c := range candidates {
		if strings.ContainsAny(c, `/\\`) {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				return c, nil
			}
			continue
		}
		if path, err := exec.LookPath(c); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("Chrome/Chromium not found. Install Chrome or set BROWSER_BIN")
}

func setBrowserHeaders(h http.Header) {
	h.Set("User-Agent", browserUserAgent())
	h.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	h.Set("Accept-Language", "ru-RU,ru;q=0.9,en;q=0.8")
	h.Set("Cache-Control", "no-cache")
}

func browserUserAgent() string {
	return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
}

func parseHTML(src, pageURL string) []api.Event {
	if events := parseJSONLDEvents(src, pageURL); len(events) > 0 {
		return events
	}
	return parseCardEvents(src, pageURL)
}

func parseJSONLDEvents(src, pageURL string) []api.Event {
	blocks := extractJSONLDBlocks(src)
	now := time.Now().UTC().Format(time.RFC3339)
	events := make([]api.Event, 0, len(blocks))
	seen := map[string]bool{}
	for _, raw := range blocks {
		for _, item := range normalizeJSONLD(raw) {
			if !isEventType(item.Type) || strings.TrimSpace(item.Name) == "" {
				continue
			}
			key := item.Name + "|" + item.StartDate
			if seen[key] {
				continue
			}
			seen[key] = true
			id := len(events) + 1
			desc := cleanText(item.Description)
			if desc == "" {
				desc = cleanText(item.Name)
			}
			short := truncate(desc, 180)
			img := firstString(item.Image)
			ticketURL := absolutize(item.URL, pageURL)
			if ticketURL == "" {
				ticketURL = pageURL
			}
			currency := "RUB"
			service := "Яндекс Афиша"
			minPrice, maxPrice := prices(item.Offers)
			events = append(events, api.Event{
				ID:                   id,
				Title:                cleanText(item.Name),
				ShortDescription:     short,
				FullDescription:      desc,
				Category:             "other",
				CoverImageURL:        absolutize(img, pageURL),
				OrganizerName:        organizerName(item.Organizer),
				OrganizerDescription: "Импортировано из Яндекс Афиши",
				Status:               "published",
				CreatedAt:            now,
				UpdatedAt:            now,
				EventSessions: []api.EventSession{{
					ID:                id,
					StartAt:           normalizeDate(item.StartDate, now),
					EndAt:             optionalDate(item.EndDate),
					IsOnline:          false,
					TicketMinPrice:    minPrice,
					TicketMaxPrice:    maxPrice,
					Currency:          &currency,
					TicketURL:         &ticketURL,
					TicketServiceName: &service,
					Status:            "published",
					CreatedAt:         now,
					UpdatedAt:         now,
					Venue:             venue(item.Location),
				}},
			})
		}
	}
	return events
}

type cardCandidate struct {
	Title string
	URL   string
	Text  string
	Image string
}

func parseCardEvents(src, pageURL string) []api.Event {
	links := extractLinks(src, pageURL)
	if len(links) == 0 {
		return nil
	}
	seen := map[string]bool{}
	cands := make([]cardCandidate, 0, len(links))
	for _, l := range links {
		if !looksLikeEventURL(l.URL) {
			continue
		}
		fragment := boundedEventFragment(src, l.Start, l.End)
		text := cleanText(stripTags(fragment))
		title := bestTitle(l.Text, fragment, text)
		if title == "" || len([]rune(title)) < 3 || len([]rune(title)) > 120 {
			continue
		}
		key := strings.ToLower(title) + "|" + l.URL
		if seen[key] {
			continue
		}
		seen[key] = true
		cands = append(cands, cardCandidate{Title: title, URL: l.URL, Text: text, Image: firstImage(fragment, pageURL)})
	}
	sort.SliceStable(cands, func(i, j int) bool { return scoreCandidate(cands[i]) > scoreCandidate(cands[j]) })
	return candidatesToEvents(cands)
}

type linkHit struct {
	URL        string
	Text       string
	Start, End int
}

func extractLinks(src, pageURL string) []linkHit {
	re := regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	matches := re.FindAllStringSubmatchIndex(src, -1)
	out := make([]linkHit, 0, len(matches))
	for _, m := range matches {
		if len(m) < 6 {
			continue
		}
		href := src[m[2]:m[3]]
		body := src[m[4]:m[5]]
		out = append(out, linkHit{URL: absolutize(html.UnescapeString(href), pageURL), Text: cleanText(stripTags(body)), Start: m[0], End: m[1]})
	}
	return out
}

func bestTitle(anchorText, fragment, fullText string) string {
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?is)<h[1-4][^>]*>(.*?)</h[1-4]>`),
		regexp.MustCompile(`(?is)<[^>]+(?:class|data-testid)=["'][^"']*(?:title|Title|name|Name)[^"']*["'][^>]*>(.*?)</[^>]+>`),
	} {
		for _, m := range re.FindAllStringSubmatch(fragment, -1) {
			t := cleanText(stripTags(m[1]))
			if len([]rune(t)) >= 3 && len([]rune(t)) <= 120 && !isNoiseTitle(t) {
				return t
			}
		}
	}
	if anchorText != "" && !isNoiseTitle(anchorText) {
		return anchorText
	}
	words := strings.Fields(fullText)
	for n := 3; n <= 9 && n <= len(words); n++ {
		t := strings.Join(words[:n], " ")
		if !isNoiseTitle(t) {
			return t
		}
	}
	return ""
}

func isNoiseTitle(s string) bool {
	s = strings.ToLower(cleanText(s))
	noise := []string{"купить", "билеты", "подробнее", "яндекс", "афиша", "войти", "регистрация", "самара", "выбрать"}
	for _, n := range noise {
		if s == n {
			return true
		}
	}
	return false
}

func boundedEventFragment(src string, linkStart, linkEnd int) string {
	// Prefer a complete surrounding card. The old implementation sliced a wide
	// arbitrary window around the link; if the slice started inside an SVG <path>,
	// SVG attributes became plain text and leaked into descriptions.
	leftLimit := max(0, linkStart-9000)
	rightLimit := min(len(src), linkEnd+9000)
	left := src[leftLimit:linkStart]
	right := src[linkEnd:rightLimit]

	for _, tag := range []string{"article", "li"} {
		open := strings.LastIndex(strings.ToLower(left), "<"+tag)
		close := strings.Index(strings.ToLower(right), "</"+tag+">")
		if open >= 0 && close >= 0 {
			start := leftLimit + open
			end := linkEnd + close + len(tag) + 3
			return src[start:end]
		}
	}

	// Fallback: keep the slice smaller and make sure it starts at a tag boundary,
	// not in the middle of an SVG attribute list.
	start := max(0, linkStart-1600)
	if idx := strings.Index(src[start:linkStart], "<"); idx >= 0 {
		start += idx
	}
	end := min(len(src), linkEnd+1600)
	return src[start:end]
}

func firstImage(fragment, pageURL string) string {
	re := regexp.MustCompile(`(?is)<img\b[^>]*(?:src|data-src|srcset)=["']([^"']+)["']`)
	m := re.FindStringSubmatch(fragment)
	if len(m) < 2 {
		return ""
	}
	img := strings.Fields(strings.Split(m[1], ",")[0])[0]
	return absolutize(html.UnescapeString(img), pageURL)
}

func scoreCandidate(c cardCandidate) int {
	score := len([]rune(c.Text))
	if c.Image != "" {
		score += 150
	}
	if strings.Contains(c.Text, "₽") || strings.Contains(strings.ToLower(c.Text), "руб") {
		score += 100
	}
	return score
}

func candidatesToEvents(cands []cardCandidate) []api.Event {
	now := time.Now().UTC().Format(time.RFC3339)
	out := []api.Event{}
	seenTitles := map[string]bool{}
	for _, c := range cands {
		if len(out) >= 50 {
			break
		}
		k := strings.ToLower(c.Title)
		if seenTitles[k] {
			continue
		}
		seenTitles[k] = true
		id := len(out) + 1
		desc := safeDescription(c)
		age := extractAge(desc)
		minPrice, maxPrice := extractPrices(desc)
		currency := "RUB"
		service := "Яндекс Афиша"
		venueName := extractVenue(desc)
		out = append(out, api.Event{
			ID:                   id,
			Title:                c.Title,
			ShortDescription:     truncate(desc, 180),
			FullDescription:      desc,
			Category:             "other",
			AgeRestriction:       age,
			CoverImageURL:        c.Image,
			OrganizerName:        "Яндекс Афиша",
			OrganizerDescription: "Импортировано из Яндекс Афиши",
			Status:               "published",
			CreatedAt:            now,
			UpdatedAt:            now,
			EventSessions: []api.EventSession{{
				ID:                id,
				StartAt:           now,
				IsOnline:          false,
				TicketMinPrice:    minPrice,
				TicketMaxPrice:    maxPrice,
				Currency:          &currency,
				TicketURL:         &c.URL,
				TicketServiceName: &service,
				Status:            "published",
				CreatedAt:         now,
				UpdatedAt:         now,
				Venue:             &api.Venue{Name: venueName, Address: "Самара"},
			}},
		})
	}
	return out
}

func looksLikeEventURL(u string) bool {
	if u == "" || !strings.Contains(u, "afisha.yandex") {
		return false
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	path := strings.ToLower(parsed.EscapedPath())
	bad := []string{
		"/selections/", "/places/", "/cinema/", "/concert/venues", "/maps/",
		"/profile", "/favorites", "/hall/", "/venue/", "/my-tickets", "/login",
	}
	for _, b := range bad {
		if strings.Contains(path, b) {
			return false
		}
	}
	if strings.Contains(path, "/event/") || strings.Contains(path, "/events/") {
		return true
	}
	// Yandex Afisha does not use only /events/ URLs. Real event pages are often
	// category based, for example /samara/concert/some-artist or
	// /samara/theatre/some-play. The previous prototype looked only for
	// /event(s)/ and therefore returned count:0 even when the rendered HTML
	// contained cards.
	eventPrefixes := []string{
		"/samara/event/", "/samara/events/",
		"/samara/concert/", "/samara/theatre/", "/samara/kids/",
		"/samara/standup/", "/samara/exhibitions/", "/samara/quest/",
		"/samara/show/", "/samara/sport/", "/samara/circus/",
		"/samara/lecture/", "/samara/other/",
	}
	for _, prefix := range eventPrefixes {
		if strings.HasPrefix(path, prefix) && len(strings.TrimPrefix(path, prefix)) > 0 {
			return true
		}
	}
	return false
}

func extractVenue(s string) string {
	for _, marker := range []string{"Театр", "Кинотеатр", "Филармония", "ДК", "Клуб", "Музей", "Стадион", "Цирк", "Дом культуры"} {
		idx := strings.Index(s, marker)
		if idx >= 0 {
			part := []rune(s[idx:])
			if len(part) > 80 {
				part = part[:80]
			}
			return cleanText(string(part))
		}
	}
	return "Самара"
}

func extractAge(s string) *int {
	re := regexp.MustCompile(`(\d{1,2})\s*\+`)
	m := re.FindStringSubmatch(s)
	if len(m) != 2 {
		return nil
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return nil
	}
	return &n
}

func extractPrices(s string) (*int, *int) {
	re := regexp.MustCompile(`(?i)(?:от\s*)?(\d[\d\s]{1,8})\s*(?:₽|руб|р\.)`)
	matches := re.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil, nil
	}
	values := []int{}
	for _, m := range matches {
		n, err := strconv.Atoi(strings.ReplaceAll(m[1], " ", ""))
		if err == nil && n > 0 {
			values = append(values, n)
		}
	}
	if len(values) == 0 {
		return nil, nil
	}
	minV, maxV := values[0], values[0]
	for _, v := range values {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	return &minV, &maxV
}

func extractJSONLDBlocks(src string) []string {
	re := regexp.MustCompile(`(?is)<script[^>]+type=["']application/ld\+json["'][^>]*>(.*?)</script>`)
	matches := re.FindAllStringSubmatch(src, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, html.UnescapeString(strings.TrimSpace(m[1])))
	}
	return out
}

func normalizeJSONLD(raw string) []jsonLD {
	var one jsonLD
	if json.Unmarshal([]byte(raw), &one) == nil && one.Name != "" {
		return []jsonLD{one}
	}
	var many []jsonLD
	if json.Unmarshal([]byte(raw), &many) == nil {
		return many
	}
	var graph struct {
		Graph []jsonLD `json:"@graph"`
	}
	if json.Unmarshal([]byte(raw), &graph) == nil {
		return graph.Graph
	}
	return nil
}

func isEventType(v any) bool {
	s := strings.ToLower(fmt.Sprint(v))
	return strings.Contains(s, "event") || strings.Contains(s, "theaterevent") || strings.Contains(s, "musicevent")
}

func cleanText(s string) string {
	s = html.UnescapeString(s)
	s = removeSVGArtifacts(s)
	return strings.Join(strings.Fields(s), " ")
}

func stripTags(s string) string {
	// Go regexp (RE2) does not support backreferences like \1,
	// so script/style/noscript blocks are removed with separate patterns.
	for _, pattern := range []string{
		`(?is)<script[^>]*>.*?</script>`,
		`(?is)<style[^>]*>.*?</style>`,
		`(?is)<noscript[^>]*>.*?</noscript>`,
		`(?is)<svg[^>]*>.*?</svg>`,
	} {
		re := regexp.MustCompile(pattern)
		s = re.ReplaceAllString(s, " ")
	}
	reTags := regexp.MustCompile(`(?is)<[^>]+>`)
	return reTags.ReplaceAllString(s, " ")
}

func removeSVGArtifacts(s string) string {
	// Defensive cleanup for fragments that start inside SVG tags. Typical leaked
	// text looks like: clip-rule="evenodd" d="M12...Z" stroke="#FFF" ...
	patterns := []string{
		`(?is)fill-rule\s*=\s*["'][^"']*["']`,
		`(?is)clip-rule\s*=\s*["'][^"']*["']`,
		`(?is)stroke(?:-width|-linecap|-linejoin)?\s*=\s*["'][^"']*["']`,
		`(?is)fill\s*=\s*["'][^"']*["']`,
		`(?is)data-test-id\s*=\s*["'][^"']*["']`,
		`(?is)class\s*=\s*["'][^"']*["']`,
		`(?is)xmlns\s*=\s*["'][^"']*["']`,
		`(?is)\bd\s*=\s*["'][^"']{20,}["']`,
		`(?is)M\d[0-9.,\sCQLHVSAZ-]{40,}Z`,
	}
	for _, pattern := range patterns {
		s = regexp.MustCompile(pattern).ReplaceAllString(s, " ")
	}
	return s
}

func looksLikeSVGGarbage(s string) bool {
	lower := strings.ToLower(s)
	markers := []string{"clip-rule", "fill-rule", "stroke-width", "stroke-linecap", "data-test-id", "evenodd", "hearticon.path", "<span class="}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	// Long path-like strings are not human descriptions.
	return regexp.MustCompile(`[MCZ][0-9.,\s-]{60,}`).FindString(s) != ""
}

func safeDescription(c cardCandidate) string {
	text := cleanText(c.Text)
	if looksLikeSVGGarbage(text) || len([]rune(text)) > 900 {
		text = ""
	}
	if text == "" || len([]rune(text)) < len([]rune(c.Title)) {
		parts := []string{fmt.Sprintf("Событие «%s» импортировано из Яндекс Афиши.", c.Title)}
		if venue := extractVenue(c.Text); venue != "Самара" {
			parts = append(parts, "Площадка: "+venue+".")
		}
		if minPrice, _ := extractPrices(c.Text); minPrice != nil {
			parts = append(parts, fmt.Sprintf("Билеты от %d ₽.", *minPrice))
		}
		text = strings.Join(parts, " ")
	}
	return text
}

func truncate(s string, maxLen int) string {
	r := []rune(cleanText(s))
	if len(r) <= maxLen {
		return string(r)
	}
	return string(r[:maxLen]) + "…"
}

func firstString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		if len(x) > 0 {
			return firstString(x[0])
		}
	case map[string]any:
		if u, ok := x["url"]; ok {
			return firstString(u)
		}
	}
	return ""
}

func absolutize(raw, base string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.IsAbs() {
		return u.String()
	}
	b, err := url.Parse(base)
	if err != nil {
		return raw
	}
	return b.ResolveReference(u).String()
}

func normalizeDate(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
func optionalDate(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func venue(v any) *api.Venue {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	name := cleanText(fmt.Sprint(m["name"]))
	addr := ""
	if a, ok := m["address"].(map[string]any); ok {
		addr = cleanText(fmt.Sprint(a["streetAddress"]))
	}
	if addr == "" {
		addr = cleanText(fmt.Sprint(m["address"]))
	}
	if name == "" && addr == "" {
		return nil
	}
	return &api.Venue{Name: name, Address: addr}
}

func organizerName(v any) string {
	m, ok := v.(map[string]any)
	if ok {
		if n := cleanText(fmt.Sprint(m["name"])); n != "" {
			return n
		}
	}
	return "Яндекс Афиша"
}

func prices(v any) (*int, *int) {
	var nums []int
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case map[string]any:
			for k, val := range t {
				if strings.Contains(strings.ToLower(k), "price") {
					walk(val)
				}
			}
		case []any:
			for _, val := range t {
				walk(val)
			}
		case float64:
			if t > 0 {
				nums = append(nums, int(t))
			}
		case string:
			var f float64
			if _, err := fmt.Sscanf(t, "%f", &f); err == nil && f > 0 {
				nums = append(nums, int(f))
			}
		}
	}
	walk(v)
	if len(nums) == 0 {
		return nil, nil
	}
	minV, maxV := nums[0], nums[0]
	for _, n := range nums {
		if n < minV {
			minV = n
		}
		if n > maxV {
			maxV = n
		}
	}
	return &minV, &maxV
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
