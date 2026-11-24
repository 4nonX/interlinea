package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/project/projectdb"
)

// Translation services: any machine translation engine or language model
// behind an address that answers the contract below. The server sends the
// request, so the service's key stays here and never reaches an app.
//
// The contract. A request is a POST with a JSON body:
//
//	{"sourceLang": "de-DE", "targetLang": "en-GB",
//	 "segments": [{"id": "1", "text": "Der <1>Mieter</1> zahlt."}]}
//
// and the answer, with status 200:
//
//	{"translations": [{"id": "1", "text": "The <1>tenant</1> pays."}]}
//
// In a text, <1>…</1> is a pair of tags and <2/> a tag on its own; a <
// that is text is &lt; and an & is &amp;. A translation should keep the
// tags; one that does not is taken without them.

func (s *Server) providerRoutes() {
	s.handle("GET /api/providers", s.signedIn(s.listProviders))
	s.handle("PUT /api/providers/{name}", s.admin(s.saveProvider))
	s.handle("DELETE /api/providers/{name}", s.admin(s.deleteProvider))
	s.handle("POST /api/providers/{name}/translate", s.signedIn(s.providerTranslate))
}

type providerView struct {
	Name string `json:"name"`
	// Host is where the service is, without the rest of its address.
	Host string `json:"host"`
	// URL, HeaderName and Signed (a header's value is kept) are for
	// administrators.
	URL        string `json:"url,omitempty"`
	HeaderName string `json:"headerName,omitempty"`
	Signed     bool   `json:"signed,omitempty"`
}

func viewProvider(p projectdb.Provider, admin bool) providerView {
	v := providerView{Name: p.Name}
	if u, err := url.Parse(p.URL); err == nil {
		v.Host = u.Host
	}
	if admin {
		v.URL, v.HeaderName, v.Signed = p.URL, p.HeaderName, p.HeaderValue != ""
	}
	return v
}

// listProviders lists the translation services the editor can ask for a
// suggestion. Administrators also see each one's address and the name of
// its header; the header's value is never given out.
//
// Access: signed in.
func (s *Server) listProviders(w http.ResponseWriter, r *http.Request, me session) {
	ps, err := s.cfg.Projects.Providers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]providerView, len(ps))
	for i, p := range ps {
		out[i] = viewProvider(p, me.User.Admin)
	}
	writeJSON(w, http.StatusOK, out)
}

// saveProvider adds a translation service or changes the one of the name:
// its address, and a header sent with each request to sign it. Without
// headerValue, the service keeps the value it has.
//
// Access: administrators.
func (s *Server) saveProvider(w http.ResponseWriter, r *http.Request, me session) {
	var req struct {
		URL         string  `json:"url"`
		HeaderName  string  `json:"headerName"`
		HeaderValue *string `json:"headerValue"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	p := projectdb.Provider{Name: r.PathValue("name"), URL: req.URL, HeaderName: req.HeaderName}
	if req.HeaderValue != nil {
		p.HeaderValue = *req.HeaderValue
	}
	saved, err := s.cfg.Projects.SaveProvider(r.Context(), p, req.HeaderValue == nil, me.by())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, viewProvider(saved, true))
}

// deleteProvider removes a translation service.
//
// Access: administrators.
func (s *Server) deleteProvider(w http.ResponseWriter, r *http.Request, me session) {
	if err := s.cfg.Projects.DeleteProvider(r.Context(), r.PathValue("name")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// maxProviderSegments is how many segments one request may ask for.
const maxProviderSegments = 50

type providerRequest struct {
	SourceLang string `json:"sourceLang"`
	TargetLang string `json:"targetLang"`
	Segments   []struct {
		ID     string          `json:"id"`
		Source content.Content `json:"source"`
	} `json:"segments"`
}

type providerTranslation struct {
	ID     string          `json:"id"`
	Target content.Content `json:"target"`
	// TagsLost is set where the service's translation did not have the
	// source's tags as they are, and was taken without tags.
	TagsLost bool `json:"tagsLost,omitempty"`
}

// providerTranslate asks a translation service for the translations of
// segments. The texts are sent to the service's address, outside the
// server; nothing is sent to a service that is not asked. A translation
// that does not keep the source's tags comes back without tags, marked.
//
// Access: signed in.
func (s *Server) providerTranslate(w http.ResponseWriter, r *http.Request, me session) {
	p, err := s.cfg.Projects.Provider(r.Context(), r.PathValue("name"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req providerRequest
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	src, err := langtag.Normalize(req.SourceLang)
	if err != nil {
		invalid(w, "sourceLang", "Say the language the segments are in.")
		return
	}
	tgt, err := langtag.Normalize(req.TargetLang)
	if err != nil {
		invalid(w, "targetLang", "Say the language to translate into.")
		return
	}
	if len(req.Segments) == 0 || len(req.Segments) > maxProviderSegments {
		invalid(w, "segments", fmt.Sprintf("Send between 1 and %d segments.", maxProviderSegments))
		return
	}
	out, err := askProvider(r.Context(), s.providerClient(), p, src, tgt, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "provider", fmt.Sprintf("%s did not translate: %v.", p.Name, err))
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Translations []providerTranslation `json:"translations"`
	}{out})
}

func (s *Server) providerClient() *http.Client {
	if s.cfg.ProviderClient != nil {
		return s.cfg.ProviderClient
	}
	return &http.Client{Timeout: 60 * time.Second}
}

type wireSegment struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

func askProvider(ctx context.Context, client *http.Client, p projectdb.Provider, src, tgt string, req providerRequest) ([]providerTranslation, error) {
	body := struct {
		SourceLang string        `json:"sourceLang"`
		TargetLang string        `json:"targetLang"`
		Segments   []wireSegment `json:"segments"`
	}{SourceLang: src, TargetLang: tgt}
	sources := map[string]content.Content{}
	for _, sg := range req.Segments {
		body.Segments = append(body.Segments, wireSegment{sg.ID, markup(sg.Source)})
		sources[sg.ID] = sg.Source
	}
	// The tags are written as they are, not as escapes of < and >.
	var raw bytes.Buffer
	enc := json.NewEncoder(&raw)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, p.URL, &raw)
	if err != nil {
		return nil, err
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Accept", "application/json")
	if p.HeaderName != "" {
		hr.Header.Set(p.HeaderName, p.HeaderValue)
	}
	res, err := client.Do(hr)
	if err != nil {
		// What went wrong, without the address's query, which may hold a key.
		var ue *url.Error
		if errors.As(err, &ue) {
			return nil, fmt.Errorf("its address could not be reached (%v)", ue.Err)
		}
		return nil, err
	}
	defer res.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(answer) > maxBody {
		return nil, fmt.Errorf("its answer is too large")
	}
	if res.StatusCode != http.StatusOK {
		said := strings.TrimSpace(string(answer))
		if len(said) > 200 {
			said = said[:200] + "…"
		}
		return nil, fmt.Errorf("it answered %d %s", res.StatusCode, said)
	}
	var got struct {
		Translations []wireSegment `json:"translations"`
	}
	if err := json.Unmarshal(answer, &got); err != nil {
		return nil, fmt.Errorf("its answer is not the JSON the contract asks for")
	}
	out := []providerTranslation{}
	for _, t := range got.Translations {
		source, ok := sources[t.ID]
		if !ok {
			continue
		}
		delete(sources, t.ID)
		target, err := parseMarkup(t.Text, source)
		if err == nil && !sameTags(source, target) {
			err = fmt.Errorf("tags differ")
		}
		if err != nil {
			out = append(out, providerTranslation{ID: t.ID, Target: content.Text(stripMarkup(t.Text)), TagsLost: source.HasTags() || strings.Contains(t.Text, "<")})
			continue
		}
		out = append(out, providerTranslation{ID: t.ID, Target: target})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("its answer has no translation of what was asked")
	}
	return out, nil
}

// sameTags reports whether a translation has each of the source's tags
// as often as the source.
func sameTags(source, target content.Content) bool {
	count := map[string]int{}
	for _, t := range source.Tags() {
		count[t.Kind.String()+":"+t.ID]++
	}
	for _, t := range target.Tags() {
		count[t.Kind.String()+":"+t.ID]--
	}
	for _, n := range count {
		if n != 0 {
			return false
		}
	}
	return true
}

// stripMarkup takes the tags out of a text as markup writes it.
func stripMarkup(text string) string {
	var b strings.Builder
	for text != "" {
		lt := strings.IndexByte(text, '<')
		if lt < 0 {
			b.WriteString(text)
			break
		}
		gt := strings.IndexByte(text[lt:], '>')
		if gt < 0 {
			b.WriteString(text)
			break
		}
		b.WriteString(text[:lt])
		text = text[lt+gt+1:]
	}
	return strings.NewReplacer("&lt;", "<", "&amp;", "&").Replace(b.String())
}
