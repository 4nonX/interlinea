package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/mt"
	"github.com/4nonX/interlinea/project/projectdb"
)

// Translation services: the services built in (DeepL, Google Cloud
// Translation, Microsoft Translator, a language model that speaks the
// OpenAI API, Anthropic's Claude), each with the key of the person's own
// account, and services of one's own at any address that answers the
// contract in docs/integration.md. Package mt builds each service's
// request and reads its answer; the server sends the request, so the key
// stays here and never reaches an app.

func (s *Server) providerRoutes() {
	s.handle("GET /api/providers", s.signedIn(s.listProviders))
	s.handle("GET /api/providers/services", s.signedIn(s.listServices))
	s.handle("POST /api/providers/models", s.admin(s.listModels))
	s.handle("PUT /api/providers/{name}", s.admin(s.saveProvider))
	s.handle("DELETE /api/providers/{name}", s.admin(s.deleteProvider))
	s.handle("POST /api/providers/{name}/translate", s.signedIn(s.providerTranslate))
	s.handle("POST /api/providers/{name}/check", s.signedIn(s.providerCheck))
}

type providerView struct {
	Name string `json:"name"`
	// Kind is "translate" for a translation service and "check" for a
	// grammar service.
	Kind string `json:"kind"`
	// Service is which service it is: "own" for one of one's own, or one
	// of those GET /api/providers/services lists.
	Service mt.Service `json:"service"`
	// Host is where the service is, without the rest of its address.
	Host string `json:"host"`
	// Model is the model a language model is asked with.
	Model string `json:"model,omitempty"`
	// URL, HeaderName, Region and Signed (a key or a header's value is
	// kept) are for administrators.
	URL        string `json:"url,omitempty"`
	HeaderName string `json:"headerName,omitempty"`
	Region     string `json:"region,omitempty"`
	Signed     bool   `json:"signed,omitempty"`
}

func viewProvider(p projectdb.Provider, admin bool) providerView {
	v := providerView{Name: p.Name, Kind: p.Kind, Service: p.Service, Host: p.Settings().Host(), Model: p.Model}
	if v.Service == "" {
		v.Service = mt.Own
	}
	if admin {
		v.URL, v.HeaderName, v.Region, v.Signed = p.URL, p.HeaderName, p.Region, p.HeaderValue != ""
	}
	return v
}

// listServices lists the translation services built in: for each, its
// name, what it needs to be set up (a key, a region, a model, an address)
// and where a key is made.
//
// Access: signed in.
func (s *Server) listServices(w http.ResponseWriter, r *http.Request, me session) {
	writeJSON(w, http.StatusOK, mt.Services)
}

type modelsRequest struct {
	// Service, URL, Key and Region are the settings being made.
	Service mt.Service `json:"service"`
	URL     string     `json:"url"`
	Key     string     `json:"key"`
	// Name names a service whose key is kept, when Key is empty.
	Name string `json:"name"`
}

// listModels asks a language model service which models the key may use,
// for choosing one while a service is set up. Without key, the key kept
// for the service of the name is used.
//
// Access: administrators.
func (s *Server) listModels(w http.ResponseWriter, r *http.Request, me session) {
	var req modelsRequest
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	set := mt.Settings{Service: req.Service, URL: req.URL, Key: req.Key}
	if set.Key == "" && req.Name != "" {
		if p, err := s.cfg.Projects.Provider(r.Context(), req.Name); err == nil && p.Service == req.Service {
			set.Key = p.HeaderValue
		}
	}
	ask, ok, err := set.Models()
	if !ok {
		invalid(w, "service", "This service has no models to choose from.")
		return
	}
	if err != nil {
		invalid(w, "key", "To list the models: "+err.Error()+".")
		return
	}
	status, body, err := mt.Do(r.Context(), s.providerClient(), ask)
	var models []string
	if err == nil {
		models, err = set.ModelsAnswer(status, body)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "provider", fmt.Sprintf("The models could not be listed: %v.", err))
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Models []string `json:"models"`
	}{models})
}

// listProviders lists the services: the translation services the editor
// can ask for a suggestion and the grammar services that read
// translations. Administrators also see each one's address and the name of
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

// saveProvider adds a service or changes the one of the name: its kind
// ("translate", the default, or "check" for a grammar service) and which
// service it is. A built-in one (service "deepl", "google", "microsoft",
// "openai", "anthropic") takes its key in headerValue, and a model or a
// region where it needs one; one of one's own ("own", the default) takes
// its address and a header sent with each request to sign it. Without
// headerValue, the service keeps the key or value it has.
//
// Access: administrators.
func (s *Server) saveProvider(w http.ResponseWriter, r *http.Request, me session) {
	var req struct {
		Kind        string     `json:"kind"`
		Service     mt.Service `json:"service"`
		URL         string     `json:"url"`
		HeaderName  string     `json:"headerName"`
		HeaderValue *string    `json:"headerValue"`
		Model       string     `json:"model"`
		Region      string     `json:"region"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	p := projectdb.Provider{Name: r.PathValue("name"), Kind: req.Kind, Service: req.Service, URL: req.URL, HeaderName: req.HeaderName, Model: req.Model, Region: req.Region}
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

// deleteProvider removes a service.
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
const maxProviderSegments = mt.MaxSegments

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
	if p.Kind != projectdb.ProviderTranslate {
		invalid(w, "name", fmt.Sprintf("%s is a grammar service, not a translation service.", p.Name))
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
	return &http.Client{Timeout: 60 * time.Second, CheckRedirect: sameHost}
}

// sameHost follows a service's redirects only on its own host: a request
// carries the service's secret header, which another host must not get.
func sameHost(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	if !strings.EqualFold(req.URL.Host, via[0].URL.Host) || (via[0].URL.Scheme == "https" && req.URL.Scheme != "https") {
		return fmt.Errorf("the service sent the request on to %s", req.URL.Host)
	}
	return nil
}

type wireSegment struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

func askProvider(ctx context.Context, client *http.Client, p projectdb.Provider, src, tgt string, req providerRequest) ([]providerTranslation, error) {
	set := p.Settings()
	if set.Service == "" {
		set.Service = mt.Own
	}
	segs := make([]mt.Segment, len(req.Segments))
	sources := map[string]content.Content{}
	for i, sg := range req.Segments {
		segs[i] = mt.Segment{ID: sg.ID, Text: markup(sg.Source)}
		sources[sg.ID] = sg.Source
	}
	ask, err := set.Translate(src, tgt, segs)
	if err != nil {
		return nil, err
	}
	status, answer, err := mt.Do(ctx, client, ask)
	if err != nil {
		return nil, err
	}
	got, err := set.Answer(segs, status, answer)
	if err != nil {
		return nil, err
	}
	out := []providerTranslation{}
	for _, t := range got {
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
