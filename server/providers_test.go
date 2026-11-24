package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
)

func TestTranslationServices(t *testing.T) {
	te := newTeam(t)
	// A service that answers the contract: it shouts, keeps the tags of
	// the first segment and loses those of any other.
	var seenKey, seenBody string
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenKey = r.Header.Get("X-Api-Key")
		raw, _ := io.ReadAll(r.Body)
		seenBody = string(raw)
		var req struct {
			SourceLang, TargetLang string
			Segments               []wireSegment
		}
		if json.Unmarshal(raw, &req) != nil || seenKey != "secret" {
			http.Error(w, "who are you?", http.StatusUnauthorized)
			return
		}
		var out struct {
			Translations []wireSegment `json:"translations"`
		}
		for i, s := range req.Segments {
			text := strings.ToUpper(s.Text)
			if i > 0 {
				text = strings.NewReplacer("<1>", "", "</1>", "").Replace(text)
			}
			out.Translations = append(out.Translations, wireSegment{s.ID, req.TargetLang + ": " + text})
		}
		json.NewEncoder(w).Encode(out)
	}))
	defer svc.Close()

	var list []providerView
	te.call("GET", "/api/providers", te.ben, nil, 200, &list)
	if len(list) != 0 {
		t.Fatalf("services at the start: %+v", list)
	}
	body := map[string]any{"url": svc.URL + "/translate?v=1", "headerName": "X-Api-Key", "headerValue": "secret"}
	if r := te.do("PUT", "/api/providers/Shout", te.pm, body); r.status != 403 {
		t.Fatalf("someone who is no administrator adds a service: %d", r.status)
	}
	var saved providerView
	te.call("PUT", "/api/providers/Shout", te.admin, body, 200, &saved)
	if saved.Name != "Shout" || !saved.Signed || saved.HeaderName != "X-Api-Key" || !strings.HasSuffix(saved.URL, "/translate?v=1") {
		t.Fatalf("saved %+v", saved)
	}
	// Everyone sees that it is there and where; only administrators its address, and nobody the key.
	r := te.do("GET", "/api/providers", te.ben, nil)
	if strings.Contains(string(r.body), "secret") || strings.Contains(string(r.body), "X-Api-Key") || strings.Contains(string(r.body), "/translate") {
		t.Fatalf("a translator is told too much: %s", r.body)
	}
	te.call("GET", "/api/providers", te.ben, nil, 200, &list)
	if len(list) != 1 || list[0].Name != "Shout" || list[0].Host == "" {
		t.Fatalf("listed %+v", list)
	}
	if r := te.do("GET", "/api/providers", te.admin, nil); strings.Contains(string(r.body), "secret") || !strings.Contains(string(r.body), `"signed":true`) {
		t.Fatalf("an administrator's list: %s", r.body)
	}

	bold := content.Text("Der ").AppendTag(content.Tag{Kind: content.TagOpen, ID: "1", Type: "bold"}).Append("Mieter").AppendTag(content.Tag{Kind: content.TagClose, ID: "1", Type: "bold"}).Append(" zahlt.")
	ask := map[string]any{"sourceLang": "de-DE", "targetLang": "en-GB", "segments": []map[string]any{{"id": "a", "source": bold}, {"id": "b", "source": bold}}}
	var got struct {
		Translations []providerTranslation `json:"translations"`
	}
	te.call("POST", "/api/providers/shout/translate", te.ben, ask, 200, &got)
	if !strings.Contains(seenBody, `"text":"Der <1>Mieter</1> zahlt."`) && !strings.Contains(seenBody, `"text":"Der <1>Mieter</1> zahlt."`) {
		t.Fatalf("the service was sent %s", seenBody)
	}
	if len(got.Translations) != 2 {
		t.Fatalf("translations %+v", got)
	}
	a, b := got.Translations[0], got.Translations[1]
	if a.Target.Plain() != "en-GB: DER MIETER ZAHLT." || len(a.Target.Tags()) != 2 || a.TagsLost {
		t.Errorf("with its tags: %+v", a)
	}
	if b.Target.Plain() != "en-GB: DER MIETER ZAHLT." || len(b.Target.Tags()) != 0 || !b.TagsLost {
		t.Errorf("without its tags: %+v", b)
	}

	// Changed without a new key, the service keeps the one it has.
	te.call("PUT", "/api/providers/Shout", te.admin, map[string]any{"url": svc.URL, "headerName": "X-Api-Key"}, 200, &saved)
	te.call("POST", "/api/providers/Shout/translate", te.ben, ask, 200, &got)
	if seenKey != "secret" {
		t.Fatalf("the key was lost: %q", seenKey)
	}
	// With a wrong key the service refuses, and its answer is passed on in words.
	wrong := "nope"
	te.call("PUT", "/api/providers/Shout", te.admin, map[string]any{"url": svc.URL, "headerName": "X-Api-Key", "headerValue": wrong}, 200, &saved)
	if r := te.do("POST", "/api/providers/Shout/translate", te.ben, ask); r.status != 502 || !strings.Contains(r.err().Message, "Shout did not translate: it answered 401") || strings.Contains(string(r.body), wrong) {
		t.Fatalf("a refusing service: %d %s", r.status, r.body)
	}
	for field, bad := range map[string]map[string]any{
		"url":         {"url": "ftp://example.org"},
		"headerName":  {"url": "https://example.org", "headerName": "Bad Name"},
		"headerValue": {"url": "https://example.org", "headerName": "X", "headerValue": "a\nb"},
	} {
		if r := te.do("PUT", "/api/providers/Other", te.admin, bad); r.status != 422 || r.err().Problems[0].Field != field {
			t.Errorf("%s: %d %s", field, r.status, r.body)
		}
	}
	if r := te.do("POST", "/api/providers/Shout/translate", te.ben, map[string]any{"sourceLang": "de", "targetLang": "en", "segments": []any{}}); r.status != 422 {
		t.Fatalf("no segments: %d", r.status)
	}
	if r := te.do("POST", "/api/providers/Nobody/translate", te.ben, ask); r.status != 404 {
		t.Fatalf("no such service: %d", r.status)
	}
	te.call("DELETE", "/api/providers/Shout", te.admin, nil, 204, nil)
	te.call("GET", "/api/providers", te.ben, nil, 200, &list)
	if len(list) != 0 {
		t.Fatalf("after deleting: %+v", list)
	}
}
