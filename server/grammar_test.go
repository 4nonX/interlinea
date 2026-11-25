package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/project"
)

// grammarService answers the check request as the LanguageTool HTTP API
// does: "a apple" is wrong, "teh" is a misspelling, and it only knows
// English without a region, and German.
func grammarService(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Header.Get("X-Api-Key") != "secret" {
			http.Error(w, "who are you?", http.StatusUnauthorized)
			return
		}
		lang, text := r.PostForm.Get("language"), r.PostForm.Get("text")
		*seen = append(*seen, lang+"|"+r.PostForm.Get("disabledCategories")+"|"+text)
		if lang != "en" && lang != "de-DE" {
			http.Error(w, "Error: '"+lang+"' is not a language code known to this checker", http.StatusBadRequest)
			return
		}
		type match map[string]any
		matches := []match{}
		units := func(s string) int { return len([]rune(s)) + strings.Count(s, "😀") }
		if i := strings.Index(text, "a apple"); i >= 0 {
			matches = append(matches, match{
				"message": "Use “an” instead of ‘a’ if the following word starts with a vowel sound.",
				"offset":  units(text[:i]), "length": 1,
				"replacements": []map[string]string{{"value": "an"}},
				"rule":         map[string]any{"id": "EN_A_VS_AN", "issueType": "misspelling-not", "category": map[string]string{"id": "MISC", "name": "Miscellaneous"}},
			})
		}
		if i := strings.Index(text, "teh"); i >= 0 {
			matches = append(matches, match{
				"message": "Possible spelling mistake found.", "offset": units(text[:i]), "length": 3,
				"replacements": []map[string]string{{"value": "the"}},
				"rule":         map[string]any{"id": "MORFOLOGIK_RULE_EN_GB", "issueType": "misspelling", "category": map[string]string{"id": "TYPOS", "name": "Possible Typo"}},
			})
		}
		// A finding outside the text is not passed on.
		matches = append(matches, match{"message": "Beyond the end.", "offset": units(text), "length": 4})
		json.NewEncoder(w).Encode(map[string]any{"matches": matches})
	}))
}

func TestGrammarServices(t *testing.T) {
	te := newTeam(t)
	var seen []string
	svc := grammarService(t, &seen)
	defer svc.Close()

	body := map[string]any{"kind": "check", "url": svc.URL + "/v2/check", "headerName": "X-Api-Key", "headerValue": "secret"}
	if r := te.do("PUT", "/api/providers/Grammar", te.pm, body); r.status != 403 {
		t.Fatalf("someone who is no administrator adds a service: %d", r.status)
	}
	if r := te.do("PUT", "/api/providers/Grammar", te.admin, map[string]any{"kind": "proofread", "url": svc.URL}); r.status != 422 {
		t.Fatalf("a service of an unknown kind: %d %s", r.status, r.body)
	}
	var saved providerView
	te.call("PUT", "/api/providers/Grammar", te.admin, body, 200, &saved)
	if saved.Kind != "check" || !saved.Signed {
		t.Fatalf("saved %+v", saved)
	}
	te.call("PUT", "/api/providers/Shout", te.admin, map[string]any{"url": svc.URL}, 200, &saved)
	if saved.Kind != "translate" {
		t.Fatalf("a service without a kind is %q", saved.Kind)
	}
	var list []providerView
	te.call("GET", "/api/providers", te.ben, nil, 200, &list)
	if len(list) != 2 || list[0].Name != "Grammar" || list[0].Kind != "check" || list[0].URL != "" || list[1].Kind != "translate" {
		t.Fatalf("listed %+v", list)
	}
	// Each kind answers its own request only.
	if r := te.do("POST", "/api/providers/Shout/check", te.ben, map[string]any{"lang": "en-GB", "text": "x"}); r.status != 422 {
		t.Fatalf("checking with a translation service: %d %s", r.status, r.body)
	}
	if r := te.do("POST", "/api/providers/Grammar/translate", te.ben, map[string]any{"sourceLang": "de-DE", "targetLang": "en-GB",
		"segments": []map[string]any{{"id": "1", "source": content.Text("x")}}}); r.status != 422 {
		t.Fatalf("translating with a grammar service: %d %s", r.status, r.body)
	}

	// The finding comes back with its place in UTF-16 code units; the
	// misspelling and the finding outside the text do not. The service
	// does not know en-GB and is asked again for en.
	var got struct{ Matches []grammarMatch }
	te.call("POST", "/api/providers/Grammar/check", te.ben, map[string]any{"lang": "en-GB", "text": "😀 I ate a apple and teh pear."}, 200, &got)
	if len(got.Matches) != 1 {
		t.Fatalf("matches %+v", got.Matches)
	}
	m := got.Matches[0]
	if m.Offset != 9 || m.Length != 1 || m.Found != "a" || len(m.Replacements) != 1 || m.Replacements[0] != "an" || m.Rule != "EN_A_VS_AN" || m.Category != "Miscellaneous" {
		t.Fatalf("match %+v", m)
	}
	if len(seen) != 2 || !strings.HasPrefix(seen[0], "en-GB|TYPOS|") || !strings.HasPrefix(seen[1], "en|TYPOS|") {
		t.Fatalf("asked %q", seen)
	}
	// Nothing to read is not sent.
	seen = nil
	te.call("POST", "/api/providers/Grammar/check", te.ben, map[string]any{"lang": "en-GB", "text": "  "}, 200, &got)
	if len(got.Matches) != 0 || len(seen) != 0 {
		t.Fatalf("an empty text: %+v, asked %q", got.Matches, seen)
	}
	if r := te.do("POST", "/api/providers/Grammar/check", te.ben, map[string]any{"lang": "", "text": "x"}); r.status != 422 {
		t.Fatalf("without a language: %d", r.status)
	}
	if r := te.do("POST", "/api/providers/Grammar/check", te.ben, map[string]any{"lang": "en-GB", "text": strings.Repeat("x", maxGrammarText+1)}); r.status != 422 {
		t.Fatalf("a text too long: %d", r.status)
	}
	// A language the service does not know at all is its refusal, said as it is.
	if r := te.do("POST", "/api/providers/Grammar/check", te.ben, map[string]any{"lang": "fr-FR", "text": "Bonjour."}); r.status != 502 || !strings.Contains(r.err().Message, "Grammar did not check: it answered 400") {
		t.Fatalf("an unknown language: %d %s", r.status, r.body)
	}

	// A project names its grammar service, and the report has what it finds.
	req := leaseRequest()
	req.Settings.QA.Grammar = "Grammar"
	var p projectView
	te.call("POST", "/api/projects", te.pm, req, 201, &p)
	if p.Settings.QA.Grammar != "Grammar" {
		t.Fatalf("settings %+v", p.Settings.QA)
	}
	te.call("PUT", segPath(p.ID, "Mietvertrag.txt", "en-GB", "s2", 1), te.pm, project.Edit{Target: content.Text("The tenant eats a apple."), Status: project.StatusConfirmed}, 200, nil)
	r := te.download(fmt.Sprintf("/api/projects/%d/qa/report?file=Mietvertrag.txt&lang=en-GB&format=csv", p.ID), te.pm)
	if !bytes.Contains(r.body, []byte("Grammar")) || !bytes.Contains(r.body, []byte("Use “an” instead")) {
		t.Fatalf("report %s", r.body)
	}
	// Without the service the report says so and is not written without it.
	te.call("DELETE", "/api/providers/Grammar", te.admin, nil, 204, nil)
	if r := te.do("GET", fmt.Sprintf("/api/projects/%d/qa/report?file=Mietvertrag.txt&lang=en-GB&format=csv", p.ID), te.pm, nil); r.status != 502 || !strings.Contains(r.err().Message, "no longer there") {
		t.Fatalf("report without the service: %d %s", r.status, r.body)
	}
}
