package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/mt"
)

// upper upper-cases the text of a segment, not its tags.
func upper(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			r = []rune(strings.ToUpper(string(r)))[0]
		}
		b.WriteRune(r)
	}
	return b.String()
}

// services answers for the services built in, at their own addresses:
// each translates by upper-casing the text and keeps the tags as it got
// them, and checks the key.
type services struct {
	seen map[string]*http.Request
}

func (s *services) RoundTrip(r *http.Request) (*http.Response, error) {
	s.seen[r.URL.Host] = r
	raw, _ := io.ReadAll(r.Body)
	answer := func(status int, body any) (*http.Response, error) {
		rec := httptest.NewRecorder()
		rec.WriteHeader(status)
		json.NewEncoder(rec).Encode(body)
		return rec.Result(), nil
	}
	switch r.URL.Host {
	case "api-free.deepl.com":
		if r.Header.Get("Authorization") != "DeepL-Auth-Key good:fx" {
			return answer(403, map[string]string{"message": "Authorization failure"})
		}
		var req struct{ Text []string }
		json.Unmarshal(raw, &req)
		var out []map[string]string
		for _, t := range req.Text {
			out = append(out, map[string]string{"text": upper(t)})
		}
		return answer(200, map[string]any{"translations": out})
	case "api.openai.com":
		if r.Header.Get("Authorization") != "Bearer sk-good" {
			return answer(401, map[string]any{"error": map[string]string{"message": "Incorrect API key"}})
		}
		if r.Method == http.MethodGet {
			return answer(200, map[string]any{"data": []map[string]string{{"id": "gpt-b"}, {"id": "whisper-1"}, {"id": "gpt-a"}}})
		}
		var req struct {
			Messages []struct{ Content string }
		}
		json.Unmarshal(raw, &req)
		var asked struct{ Segments []mt.Segment }
		json.Unmarshal([]byte(req.Messages[1].Content), &asked)
		for i := range asked.Segments {
			asked.Segments[i].Text = upper(asked.Segments[i].Text)
		}
		content, _ := json.Marshal(map[string]any{"translations": asked.Segments})
		return answer(200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(content)}}}})
	}
	return answer(404, map[string]string{"message": "unknown host " + r.URL.Host})
}

// The services built in are set up with a key and asked like any other;
// the key stays on the server.
func TestBuiltInServices(t *testing.T) {
	fake := &services{seen: map[string]*http.Request{}}
	e := newEnv(t, func(c *Config) { c.ProviderClient = &http.Client{Transport: fake} })
	_, admin := e.user("anna", true)
	_, ben := e.user("ben", false)

	var list []mt.Info
	e.call("GET", "/api/providers/services", ben, nil, 200, &list)
	if len(list) != len(mt.Services) || list[0].Service != mt.DeepL || !list[0].Key {
		t.Fatalf("services %+v", list)
	}

	// DeepL: a service and a key.
	if r := e.do("PUT", "/api/providers/DeepL", admin, map[string]any{"service": "deepl"}); r.status != 422 || r.err().Problems[0].Field != "headerValue" {
		t.Fatalf("DeepL without a key: %d %s", r.status, r.body)
	}
	var saved providerView
	e.call("PUT", "/api/providers/DeepL", admin, map[string]any{"service": "deepl", "headerValue": "good:fx"}, 200, &saved)
	if saved.Service != mt.DeepL || saved.Host != "api-free.deepl.com" || !saved.Signed || saved.URL != "" {
		t.Fatalf("saved %+v", saved)
	}
	ask := map[string]any{"sourceLang": "de-DE", "targetLang": "en-GB", "segments": []any{
		map[string]any{"id": "1", "source": content.Content{content.Text("Der ")[0], {Tag: &content.Tag{Kind: content.TagOpen, ID: "1"}}, content.Text("Mieter")[0], {Tag: &content.Tag{Kind: content.TagClose, ID: "1"}}, content.Text(" zahlt.")[0]}},
	}}
	var got struct {
		Translations []providerTranslation `json:"translations"`
	}
	e.call("POST", "/api/providers/DeepL/translate", ben, ask, 200, &got)
	if len(got.Translations) != 1 || got.Translations[0].TagsLost || got.Translations[0].Target.Plain() != "DER MIETER ZAHLT." || !got.Translations[0].Target.HasTags() {
		t.Fatalf("DeepL translated %+v", got)
	}
	// Changing it without the key keeps the key; a wrong key is refused by
	// the service, in words, and never shown.
	e.call("PUT", "/api/providers/DeepL", admin, map[string]any{"service": "deepl"}, 200, &saved)
	e.call("POST", "/api/providers/DeepL/translate", ben, ask, 200, &got)
	e.call("PUT", "/api/providers/DeepL", admin, map[string]any{"service": "deepl", "headerValue": "wrong:fx"}, 200, &saved)
	if r := e.do("POST", "/api/providers/DeepL/translate", ben, ask); r.status != 502 || !strings.Contains(r.err().Message, "the key was not accepted") || strings.Contains(string(r.body), "wrong:fx") {
		t.Fatalf("a wrong key: %d %s", r.status, r.body)
	}

	// A language model: the models the key may use are listed, one is chosen.
	if r := e.do("POST", "/api/providers/models", ben, map[string]any{"service": "openai", "key": "sk-good"}); r.status != 403 {
		t.Fatalf("someone who is no administrator lists models: %d", r.status)
	}
	var models struct{ Models []string }
	e.call("POST", "/api/providers/models", admin, map[string]any{"service": "openai", "key": "sk-good"}, 200, &models)
	if strings.Join(models.Models, ",") != "gpt-a,gpt-b" {
		t.Fatalf("models %v", models)
	}
	if r := e.do("POST", "/api/providers/models", admin, map[string]any{"service": "openai", "key": "sk-bad"}); r.status != 502 || !strings.Contains(r.err().Message, "key was not accepted") {
		t.Fatalf("models with a wrong key: %d %s", r.status, r.body)
	}
	if r := e.do("PUT", "/api/providers/GPT", admin, map[string]any{"service": "openai", "headerValue": "sk-good"}); r.status != 422 || r.err().Problems[0].Field != "model" {
		t.Fatalf("a model service without a model: %d %s", r.status, r.body)
	}
	e.call("PUT", "/api/providers/GPT", admin, map[string]any{"service": "openai", "headerValue": "sk-good", "model": "gpt-a"}, 200, &saved)
	// The kept key lists the models again, as when the model is changed later.
	e.call("POST", "/api/providers/models", admin, map[string]any{"service": "openai", "name": "GPT"}, 200, &models)
	e.call("POST", "/api/providers/GPT/translate", ben, ask, 200, &got)
	if got.Translations[0].Target.Plain() != "DER MIETER ZAHLT." || !got.Translations[0].Target.HasTags() {
		t.Fatalf("the model translated %+v", got)
	}
	if fake.seen["api.openai.com"] == nil {
		t.Fatal("the model was not asked")
	}

	// Everyone sees which service each is and its model; only
	// administrators what is kept, and nobody a key.
	var seen []providerView
	r := e.do("GET", "/api/providers", ben, nil)
	json.Unmarshal(r.body, &seen)
	if len(seen) != 2 || seen[1].Service != mt.OpenAI || seen[1].Model != "gpt-a" || seen[1].Signed || strings.Contains(string(r.body), "sk-good") {
		t.Fatalf("listed %s", r.body)
	}
	// A grammar service is one of one's own.
	if r := e.do("PUT", "/api/providers/LT", admin, map[string]any{"kind": "check", "service": "deepl", "headerValue": "good:fx"}); r.status != 422 {
		t.Fatalf("a built-in grammar service: %d", r.status)
	}
}
