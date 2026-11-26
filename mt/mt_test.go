package mt

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var segs = []Segment{{ID: "a", Text: "Der <1>Mieter</1> zahlt<2/> &amp; haftet."}, {ID: "b", Text: "Die Kaution &lt; 3 Mieten."}}

func body(t *testing.T, r Request) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(r.Body), &m); err != nil {
		var list []any
		if json.Unmarshal([]byte(r.Body), &list) == nil {
			return map[string]any{"list": list}
		}
		t.Fatalf("body %q: %v", r.Body, err)
	}
	return m
}

func TestTagsGoAndComeBack(t *testing.T) {
	for _, in := range []string{
		"Der <1>Mieter</1> zahlt<2/> &amp; haftet.",
		"<b1>fett</b1> und <x-2/> &lt;Platz&gt;",
		"ohne Tags",
	} {
		sent := toTags(in)
		if strings.Contains(sent, "<1>") || strings.Contains(sent, "<b1>") {
			t.Fatalf("%q sent as %q", in, sent)
		}
		if got := fromTags(sent); got != strings.ReplaceAll(in, "&gt;", ">") {
			t.Errorf("%q came back as %q (sent %q)", in, got, sent)
		}
	}
	// What the services do to text: entities of their own, spaces in tags,
	// markup they add.
	if got := fromTags(`The <t1>tenant</t1> pays<t2 /> &amp; is liable &#39;now&#39;<br>`); got != "The <1>tenant</1> pays<2/> &amp; is liable 'now'" {
		t.Fatalf("answer read as %q", got)
	}
}

func TestDeepL(t *testing.T) {
	s := Settings{Service: DeepL, Key: "k-123:fx"}
	r, err := s.Translate("de-DE", "en-GB", segs)
	if err != nil {
		t.Fatal(err)
	}
	b := body(t, r)
	if r.URL != deeplFree || r.Header["Authorization"] != "DeepL-Auth-Key k-123:fx" || b["source_lang"] != "DE" || b["target_lang"] != "EN-GB" || b["tag_handling"] != "xml" {
		t.Fatalf("request %+v %v", r, b)
	}
	if txt := b["text"].([]any)[0]; txt != "Der <t1>Mieter</t1> zahlt<t2/> &amp; haftet." {
		t.Fatalf("text %q", txt)
	}
	if r, _ := (Settings{Service: DeepL, Key: "paid"}).Translate("de", "pt-BR", segs); r.URL != deeplPro || body(t, r)["target_lang"] != "PT-BR" {
		t.Fatalf("paid key %+v", r)
	}
	got, err := s.Answer(segs, 200, []byte(`{"translations":[{"detected_source_language":"DE","text":"The <t1>tenant</t1> pays<t2/> &amp; is liable."},{"text":"The deposit &lt; 3 rents."}]}`))
	if err != nil || len(got) != 2 || got[0].ID != "a" || got[0].Text != "The <1>tenant</1> pays<2/> &amp; is liable." || got[1].Text != "The deposit &lt; 3 rents." {
		t.Fatalf("answer %+v %v", got, err)
	}
	if _, err := s.Answer(segs, 456, []byte(`{"message":"Quota exceeded"}`)); err == nil || !strings.Contains(err.Error(), "quota") {
		t.Fatalf("quota: %v", err)
	}
	if _, err := s.Answer(segs, 403, []byte(`{"message":"Wrong endpoint"}`)); err == nil || !strings.Contains(err.Error(), "key was not accepted") || !strings.Contains(err.Error(), "Wrong endpoint") {
		t.Fatalf("refused: %v", err)
	}
}

func TestGoogle(t *testing.T) {
	s := Settings{Service: Google, Key: "AIza x"}
	r, err := s.Translate("zh-TW", "de-DE", segs)
	if err != nil {
		t.Fatal(err)
	}
	b := body(t, r)
	if !strings.HasPrefix(r.URL, googleURL+"?key=AIza+x") || b["source"] != "zh-TW" || b["target"] != "de" || b["format"] != "html" {
		t.Fatalf("request %+v %v", r, b)
	}
	got, err := s.Answer(segs, 200, []byte(`{"data":{"translations":[{"translatedText":"Il <t1>locatario</t1> paga<t2/> &amp; risponde &#39;subito&#39;."},{"translatedText":"La cauzione &lt; 3 canoni."}]}}`))
	if err != nil || got[0].Text != "Il <1>locatario</1> paga<2/> &amp; risponde 'subito'." {
		t.Fatalf("answer %+v %v", got, err)
	}
	if _, err := s.Answer(segs, 400, []byte(`{"error":{"code":400,"message":"API key not valid."}}`)); err == nil || !strings.Contains(err.Error(), "API key not valid") {
		t.Fatalf("refused: %v", err)
	}
}

func TestMicrosoft(t *testing.T) {
	s := Settings{Service: Microsoft, Key: "m", Region: "westeurope"}
	r, err := s.Translate("de-DE", "sr-Latn-RS", segs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.URL, "from=de") || !strings.Contains(r.URL, "to=sr-Latn") || !strings.Contains(r.URL, "textType=html") || r.Header["Ocp-Apim-Subscription-Region"] != "westeurope" || r.Header["Ocp-Apim-Subscription-Key"] != "m" {
		t.Fatalf("request %+v", r)
	}
	if list := body(t, r)["list"].([]any); len(list) != 2 {
		t.Fatalf("body %s", r.Body)
	}
	if r, _ := (Settings{Service: Microsoft, Key: "m", Region: "global"}).Translate("de", "en", segs); r.Header["Ocp-Apim-Subscription-Region"] != "" {
		t.Fatalf("a global key names a region: %+v", r.Header)
	}
	got, err := s.Answer(segs, 200, []byte(`[{"translations":[{"text":"Zakupac <t1>plaća</t1><t2/> &amp;.","to":"sr-Latn"}]},{"translations":[{"text":"Depozit.","to":"sr-Latn"}]}]`))
	if err != nil || len(got) != 2 || got[0].Text != "Zakupac <1>plaća</1><2/> &amp;." || got[1].ID != "b" {
		t.Fatalf("answer %+v %v", got, err)
	}
	if _, err := s.Answer(segs, 401, []byte(`{"error":{"code":401000,"message":"The request is not authorized"}}`)); err == nil || !strings.Contains(err.Error(), "key was not accepted") {
		t.Fatalf("refused: %v", err)
	}
}

func TestLanguageModels(t *testing.T) {
	for _, s := range []Settings{{Service: OpenAI, Key: "sk", Model: "gpt-x"}, {Service: OpenAI, Key: "sk", Model: "mistral-small", URL: "https://api.mistral.ai/v1/"}, {Service: Anthropic, Key: "ak", Model: "claude-x"}} {
		r, err := s.Translate("de-DE", "en-GB", segs)
		if err != nil {
			t.Fatal(err)
		}
		b := body(t, r)
		if b["model"] != s.Model || !strings.Contains(r.Body, "German (Germany)") || !strings.Contains(r.Body, "English (United Kingdom)") || !strings.Contains(r.Body, `Der <1>Mieter</1> zahlt<2/>`) {
			t.Fatalf("%s request %+v", s.Service, r)
		}
		answer := `Here it is: {"translations": [{"id": "a", "text": "The <1>tenant</1> pays<2/> &amp; is liable."}, {"id": "z", "text": "not asked"}]}`
		var raw []byte
		switch s.Service {
		case OpenAI:
			want := "https://api.openai.com/v1/chat/completions"
			if s.URL != "" {
				want = "https://api.mistral.ai/v1/chat/completions"
			}
			if r.URL != want || r.Header["Authorization"] != "Bearer sk" {
				t.Fatalf("openai request %+v", r)
			}
			raw, _ = json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}}}})
		case Anthropic:
			if r.URL != anthropicURL+"/messages" || r.Header["x-api-key"] != "ak" || r.Header["anthropic-version"] == "" {
				t.Fatalf("anthropic request %+v", r)
			}
			raw, _ = json.Marshal(map[string]any{"content": []any{map[string]string{"type": "text", "text": answer}}})
		}
		got, err := s.Answer(segs, 200, raw)
		if err != nil || len(got) != 1 || got[0].ID != "a" || got[0].Text != "The <1>tenant</1> pays<2/> &amp; is liable." {
			t.Fatalf("%s answer %+v %v", s.Service, got, err)
		}
	}
	s := Settings{Service: OpenAI, Key: "sk", Model: "m"}
	if _, err := s.Answer(segs, 200, []byte(`{"choices":[{"message":{"content":"Sorry, I cannot."}}]}`)); err == nil {
		t.Fatal("an answer in another form read")
	}
	if _, err := s.Answer(segs, 404, []byte(`{"error":{"message":"The model does not exist"}}`)); err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("no model: %v", err)
	}
}

func TestModels(t *testing.T) {
	if _, ok, _ := (Settings{Service: DeepL, Key: "k"}).Models(); ok {
		t.Fatal("DeepL has models to choose")
	}
	s := Settings{Service: OpenAI, Key: "sk"}
	r, ok, err := s.Models()
	if !ok || err != nil || r.URL != openAIURL+"/models" || r.Method != "GET" {
		t.Fatalf("models request %+v %v %v", r, ok, err)
	}
	got, err := s.ModelsAnswer(200, []byte(`{"data":[{"id":"gpt-b"},{"id":"text-embedding-3"},{"id":"gpt-a"},{"id":"whisper-1"},{"id":"gpt-a-tts"}]}`))
	if err != nil || strings.Join(got, ",") != "gpt-a,gpt-b" {
		t.Fatalf("models %v %v", got, err)
	}
	if _, _, err := (Settings{Service: Anthropic}).Models(); err == nil {
		t.Fatal("models without a key")
	}
	if _, err := s.ModelsAnswer(401, []byte(`{"error":{"message":"Incorrect API key"}}`)); err == nil || !strings.Contains(err.Error(), "key was not accepted") {
		t.Fatalf("refused: %v", err)
	}
}

func TestOwnService(t *testing.T) {
	s := Settings{Service: Own, URL: "https://mt.example/translate", HeaderName: "Authorization", Key: "Bearer x"}
	r, err := s.Translate("de-DE", "en-GB", segs)
	if err != nil || r.URL != s.URL || r.Header["Authorization"] != "Bearer x" || !strings.Contains(r.Body, `"sourceLang":"de-DE"`) || !strings.Contains(r.Body, `Der <1>Mieter</1>`) {
		t.Fatalf("request %+v %v", r, err)
	}
	got, err := s.Answer(segs, 200, []byte(`{"translations":[{"id":"b","text":"The deposit."}]}`))
	if err != nil || len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("answer %+v %v", got, err)
	}
}

func TestCheck(t *testing.T) {
	for s, field := range map[Settings]string{
		{Service: "nope"}:           "service",
		{Service: DeepL}:            "key",
		{Service: OpenAI, Key: "k"}: "model",
		{Service: OpenAI, Key: "k", Model: "m", URL: "ftp://x"}: "url",
		{Service: Own, URL: "https://x", Key: "v"}:              "headerName",
		{Service: Microsoft, Key: "k", Region: "west europe"}:   "region",
	} {
		p := s.Check()
		if len(p) == 0 || p[0].Field != field {
			t.Errorf("%+v: %+v", s, p)
		}
	}
	if p := (Settings{Service: Anthropic, Key: "k", Model: "m"}).Check(); len(p) != 0 {
		t.Fatalf("good settings: %+v", p)
	}
	if h := (Settings{Service: DeepL, Key: "x:fx"}).Host(); h != "api-free.deepl.com" {
		t.Fatalf("host %q", h)
	}
}

func TestLanguages(t *testing.T) {
	for tag, want := range map[string][3]string{
		"en-GB":      {"EN-GB", "en", "en"},
		"en-US":      {"EN-US", "en", "en"},
		"en":         {"EN-US", "en", "en"},
		"pt-PT":      {"PT-PT", "pt-PT", "pt-pt"},
		"pt-BR":      {"PT-BR", "pt", "pt"},
		"zh-CN":      {"ZH-HANS", "zh-CN", "zh-Hans"},
		"zh-Hant-TW": {"ZH-HANT", "zh-TW", "zh-Hant"},
		"ko-KR":      {"KO", "ko", "ko"},
		"fr-CA":      {"FR", "fr", "fr-ca"},
	} {
		got := [3]string{deeplTarget(tag), googleLang(tag), microsoftLang(tag)}
		if got != want {
			t.Errorf("%s: %v, want %v", tag, got, want)
		}
	}
}

func TestDo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "DeepL-Auth-Key k" {
			w.WriteHeader(403)
			return
		}
		w.Write([]byte(`{"translations":[{"text":"ok"}]}`))
	}))
	defer srv.Close()
	r, _ := (Settings{Service: DeepL, Key: "k"}).Translate("de", "en", segs)
	r.URL = srv.URL
	status, b, err := Do(context.Background(), srv.Client(), r)
	if err != nil || status != 200 || !strings.Contains(string(b), "ok") {
		t.Fatalf("%d %s %v", status, b, err)
	}
	r.URL = "http://127.0.0.1:1/?key=secret"
	if _, _, err := Do(context.Background(), srv.Client(), r); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unreachable: %v", err)
	}
}
