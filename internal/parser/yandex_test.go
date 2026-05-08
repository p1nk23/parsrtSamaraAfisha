package parser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseJSONLDEvents(t *testing.T) {
	html := `<!doctype html><html><head>
<script type="application/ld+json">
{
  "@context":"https://schema.org",
  "@type":"Event",
  "name":"Большой концерт",
  "description":"Описание концерта",
  "startDate":"2026-06-01T19:00:00+04:00",
  "endDate":"2026-06-01T21:00:00+04:00",
  "image":"/images/concert.jpg",
  "url":"/events/concert-1",
  "location":{"name":"Филармония","address":{"streetAddress":"ул. Фрунзе, 141"}},
  "organizer":{"name":"Организатор"},
  "offers":{"lowPrice":500,"highPrice":1200}
}
</script>
</head><body></body></html>`

	events := parseHTML(html, "https://afisha.yandex.ru/samara/selections/hot")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Title != "Большой концерт" {
		t.Fatalf("unexpected title: %q", e.Title)
	}
	if e.CoverImageURL != "https://afisha.yandex.ru/images/concert.jpg" {
		t.Fatalf("unexpected image url: %q", e.CoverImageURL)
	}
	if len(e.EventSessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(e.EventSessions))
	}
	s := e.EventSessions[0]
	if s.StartAt != "2026-06-01T19:00:00+04:00" {
		t.Fatalf("unexpected startAt: %q", s.StartAt)
	}
	if s.TicketMinPrice == nil || *s.TicketMinPrice != 500 {
		t.Fatalf("unexpected min price: %#v", s.TicketMinPrice)
	}
	if s.TicketMaxPrice == nil || *s.TicketMaxPrice != 1200 {
		t.Fatalf("unexpected max price: %#v", s.TicketMaxPrice)
	}
	if s.Venue == nil || s.Venue.Name != "Филармония" || s.Venue.Address != "ул. Фрунзе, 141" {
		t.Fatalf("unexpected venue: %#v", s.Venue)
	}
}

func TestParseCardEventsFallback(t *testing.T) {
	html := `<!doctype html><html><body>
<article>
  <h2>Спектакль Ревизор</h2>
  <img src="/poster.jpg">
  <p>Театр драмы 16+ билеты от 700 ₽ до 1500 ₽</p>
  <a href="/events/revizor">Купить билеты</a>
</article>
</body></html>`

	events := parseHTML(html, "https://afisha.yandex.ru/samara/selections/hot")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Title != "Спектакль Ревизор" {
		t.Fatalf("unexpected title: %q", e.Title)
	}
	if e.AgeRestriction == nil || *e.AgeRestriction != 16 {
		t.Fatalf("unexpected age restriction: %#v", e.AgeRestriction)
	}
	s := e.EventSessions[0]
	if s.TicketMinPrice == nil || *s.TicketMinPrice != 700 {
		t.Fatalf("unexpected min price: %#v", s.TicketMinPrice)
	}
	if s.TicketMaxPrice == nil || *s.TicketMaxPrice != 1500 {
		t.Fatalf("unexpected max price: %#v", s.TicketMaxPrice)
	}
	if s.TicketURL == nil || *s.TicketURL != "https://afisha.yandex.ru/events/revizor" {
		t.Fatalf("unexpected ticket url: %#v", s.TicketURL)
	}
}

func TestStripTagsRemovesScriptStyleNoscriptWithoutPanic(t *testing.T) {
	input := `<div>Текст</div><script>var x = "bad";</script><style>.x{}</style><noscript>no</noscript>`
	got := cleanText(stripTags(input))
	if got != "Текст" {
		t.Fatalf("unexpected stripped text: %q", got)
	}
}

func TestParseDetailedHTTPOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<script type="application/ld+json">{"@type":"Event","name":"HTTP Event","startDate":"2026-06-01T19:00:00+04:00"}</script>`))
	}))
	defer srv.Close()

	p := YandexParser{Client: srv.Client(), UseBrowser: false, BrowserTimeout: time.Second}
	result, err := p.ParseDetailed(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("ParseDetailed returned error: %v", err)
	}
	if result.Count != 1 || result.Meta.HTTPEvents != 1 || result.Meta.BrowserEvents != 0 {
		t.Fatalf("unexpected result/meta: %#v", result)
	}
}

func TestParseDetailedEmptyPageHTTPOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>empty</body></html>`))
	}))
	defer srv.Close()

	p := YandexParser{Client: srv.Client(), UseBrowser: false, BrowserTimeout: time.Second}
	result, err := p.ParseDetailed(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("ParseDetailed returned error: %v", err)
	}
	if result.Count != 0 || len(result.Events) != 0 || result.Meta.HTTPEvents != 0 {
		t.Fatalf("unexpected result/meta: %#v", result)
	}
}

func TestHelpers(t *testing.T) {
	if got := truncate("123456", 3); got != "123…" {
		t.Fatalf("unexpected truncate: %q", got)
	}
	if got := absolutize("//cdn.example.com/a.jpg", "https://afisha.yandex.ru/x"); got != "https://cdn.example.com/a.jpg" {
		t.Fatalf("unexpected protocol-relative url: %q", got)
	}
	if got := absolutize("/events/a", "https://afisha.yandex.ru/samara/"); got != "https://afisha.yandex.ru/events/a" {
		t.Fatalf("unexpected absolute url: %q", got)
	}
}

func TestLooksLikeEventURLAcceptsYandexCategoryPages(t *testing.T) {
	cases := []string{
		"https://afisha.yandex.ru/samara/concert/krovostok",
		"https://afisha.yandex.ru/samara/theatre/revizor",
		"https://afisha.yandex.ru/samara/kids/elka",
		"https://afisha.yandex.ru/samara/standup/open-mic",
		"https://afisha.yandex.ru/samara/exhibitions/art",
	}
	for _, raw := range cases {
		if !looksLikeEventURL(raw) {
			t.Fatalf("expected %s to look like event URL", raw)
		}
	}
}

func TestLooksLikeEventURLRejectsNonEventPages(t *testing.T) {
	cases := []string{
		"https://afisha.yandex.ru/samara/selections/hot",
		"https://afisha.yandex.ru/samara/places/teatr",
		"https://afisha.yandex.ru/samara/cinema",
		"https://afisha.yandex.ru/samara/concert/venues/foo",
	}
	for _, raw := range cases {
		if looksLikeEventURL(raw) {
			t.Fatalf("expected %s to be rejected", raw)
		}
	}
}

func TestParseCardEventsWithCategoryURL(t *testing.T) {
	html := `<html><body><article><a href="/samara/concert/test-band"><h2>Тестовая группа</h2><img src="/poster.jpg"/>от 1200 ₽ 16+ Клуб Звезда</a></article></body></html>`
	events := parseCardEvents(html, DefaultURL)
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Title != "Тестовая группа" {
		t.Fatalf("unexpected title: %q", events[0].Title)
	}
	if events[0].EventSessions[0].TicketURL == nil || !strings.Contains(*events[0].EventSessions[0].TicketURL, "/samara/concert/test-band") {
		t.Fatalf("unexpected ticket url: %#v", events[0].EventSessions[0].TicketURL)
	}
}

func TestCleanTextRemovesLeakedSVGAttributes(t *testing.T) {
	input := `l-rule="evenodd" clip-rule="evenodd" d="M12.9476 21.4473C6.65888 16.5508 2.96701 13.0385 1.87193 10.9102C0.318814 7.89182 1.06246 4.89043 2.37464 3.24923C3.71899 1.5678 5.30995 1.00388 7.02621 1.00388Z" stroke="#FFF" fill="transparent" stroke-width="2" data-test-id="heartIcon.path"> Женский стендап 17 мая, 19:00 МТЛ Арена от 2 500 ₽`
	got := cleanText(input)
	for _, bad := range []string{"clip-rule", "stroke-width", "heartIcon.path", "M12.9476"} {
		if strings.Contains(got, bad) {
			t.Fatalf("expected SVG garbage to be removed, got %q", got)
		}
	}
	if !strings.Contains(got, "Женский стендап") {
		t.Fatalf("expected useful text to remain, got %q", got)
	}
}

func TestSafeDescriptionFallsBackWhenCardTextIsSVGGarbage(t *testing.T) {
	candidate := cardCandidate{
		Title: "Женский стендап",
		URL:   "https://afisha.yandex.ru/samara/standup/foo",
		Text:  `clip-rule="evenodd" d="M12.9476 21.4473C6.65888 16.5508 2.96701 13.0385 1.87193 10.9102C0.318814 7.89182 1.06246 4.89043 2.37464 3.24923C3.71899 1.5678 5.30995 1.00388 7.02621 1.00388Z" stroke-width="2" data-test-id="heartIcon.path"> от 2 500 ₽`,
	}
	got := safeDescription(candidate)
	if strings.Contains(got, "clip-rule") || strings.Contains(got, "stroke-width") || strings.Contains(got, "M12.9476") {
		t.Fatalf("expected fallback description without SVG garbage, got %q", got)
	}
	if !strings.Contains(got, "Женский стендап") || !strings.Contains(got, "Яндекс Афиши") {
		t.Fatalf("expected meaningful fallback description, got %q", got)
	}
}
