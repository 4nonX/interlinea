// Package mt asks machine translation services and language models for
// translations: DeepL, Google Cloud Translation, Microsoft Translator,
// any service that speaks the OpenAI chat API (OpenAI, Mistral, a model
// run at home), Anthropic's Claude, and a service of one's own that
// answers Interlinea's contract (docs/integration.md). People bring their
// own key; nothing else is needed to set one up.
//
// The package builds the HTTP request for a service and reads its
// answer, and sends nothing itself: the server and the desktop app's core
// send the request with Do, the browser client with fetch, so the same
// code decides what is asked everywhere.
//
// Segments go in and come out as markup: text in which a pair of tags is
// <1>…</1>, a tag on its own <2/>, and a < or & that is text is &lt; or
// &amp;. Each service gets the tags in the form it keeps.
package mt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/langtag"
)

// Service is a kind of translation service.
type Service string

// The services.
const (
	// Own is a service of one's own, at an address that answers
	// Interlinea's contract.
	Own       Service = "own"
	DeepL     Service = "deepl"
	Google    Service = "google"
	Microsoft Service = "microsoft"
	// OpenAI is any service that speaks the OpenAI chat API: OpenAI
	// itself, or another at the address given.
	OpenAI    Service = "openai"
	Anthropic Service = "anthropic"
)

// Services lists the services, the way the apps offer them.
var Services = []Info{
	{Service: DeepL, Name: "DeepL", Key: true, Signup: "https://www.deepl.com/pro-api",
		Note: "A key of the free or the paid API; a free key ends in :fx."},
	{Service: Google, Name: "Google Cloud Translation", Key: true, Browser: true, Signup: "https://cloud.google.com/translate",
		Note: "An API key of a Google Cloud project with the Cloud Translation API on."},
	{Service: Microsoft, Name: "Microsoft Translator", Key: true, Region: true, Browser: true, Signup: "https://azure.microsoft.com/products/ai-services/ai-translator",
		Note: "A key of an Azure Translator resource, and its region unless it is global."},
	{Service: OpenAI, Name: "OpenAI or a compatible service", Key: true, Model: true, URL: true, Browser: true, Signup: "https://platform.openai.com/api-keys",
		Note: "A key of OpenAI, or the address and key of a service that speaks the same API (Mistral, a model of your own)."},
	{Service: Anthropic, Name: "Anthropic (Claude)", Key: true, Model: true, Browser: true, Signup: "https://console.anthropic.com/settings/keys",
		Note: "A key of the Anthropic API."},
	{Service: Own, Name: "A service of your own", URL: true, Header: true, Browser: true,
		Note: "Any service at an address that answers as the integration guide describes."},
}

// Info describes a service for the apps: what to ask for to set it up.
type Info struct {
	Service Service `json:"service"`
	Name    string  `json:"name"`
	// Key, Region, Model, URL and Header say what the service needs: a
	// key; the region of the key; a model, chosen from those the service
	// lists for the key; an address (for OpenAI, optional: another
	// service that speaks its API); a header that signs requests.
	Key    bool `json:"key,omitempty"`
	Region bool `json:"region,omitempty"`
	Model  bool `json:"model,omitempty"`
	URL    bool `json:"url,omitempty"`
	Header bool `json:"header,omitempty"`
	// Browser is true when a web page may ask the service itself; one
	// that does not allow it is asked by a team server or the desktop app.
	Browser bool `json:"browser,omitempty"`
	// Signup is where a key is made.
	Signup string `json:"signup,omitempty"`
	Note   string `json:"note"`
}

// Lookup returns what a service needs.
func Lookup(s Service) (Info, bool) {
	for _, i := range Services {
		if i.Service == s {
			return i, true
		}
	}
	return Info{}, false
}

// Settings are what a service is asked with.
type Settings struct {
	Service Service `json:"service"`
	// URL is the address of a service of one's own, or of a service that
	// speaks the OpenAI API (empty for OpenAI itself).
	URL string `json:"url,omitempty"`
	// Key is the service's key; for a service of one's own, the value of
	// the header that signs requests.
	Key string `json:"key,omitempty"`
	// HeaderName is the header a service of one's own takes the key in.
	HeaderName string `json:"headerName,omitempty"`
	Model      string `json:"model,omitempty"`
	// Region is the region of a Microsoft Translator key.
	Region string `json:"region,omitempty"`
}

// The addresses of the services.
const (
	deeplFree      = "https://api-free.deepl.com/v2/translate"
	deeplPro       = "https://api.deepl.com/v2/translate"
	googleURL      = "https://translation.googleapis.com/language/translate/v2"
	microsoftURL   = "https://api.cognitive.microsofttranslator.com/translate"
	openAIURL      = "https://api.openai.com/v1"
	anthropicURL   = "https://api.anthropic.com/v1"
	anthropicLevel = "2023-06-01"
)

// Problem is something wrong with settings, by field.
type Problem struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Check says what is wrong with the settings.
func (s Settings) Check() []Problem {
	info, ok := Lookup(s.Service)
	if !ok {
		return []Problem{{"service", "Choose a service."}}
	}
	var out []Problem
	if info.Key && strings.TrimSpace(s.Key) == "" {
		out = append(out, Problem{"key", "Paste the key of your account at " + info.Name + "."})
	}
	if strings.ContainsAny(s.Key, "\r\n") {
		out = append(out, Problem{"key", "The key is one line."})
	}
	if info.Model && strings.TrimSpace(s.Model) == "" {
		out = append(out, Problem{"model", "Choose a model."})
	}
	if s.Service == Own || (s.Service == OpenAI && strings.TrimSpace(s.URL) != "") {
		if u, err := url.Parse(strings.TrimSpace(s.URL)); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			out = append(out, Problem{"url", "The address starts with https:// or http:// and names where requests are sent."})
		}
	}
	if s.Service == Own {
		if strings.ContainsFunc(s.HeaderName, func(r rune) bool { return r <= ' ' || r >= 0x7F || r == ':' }) {
			out = append(out, Problem{"headerName", "The header's name has no blanks and no colon."})
		}
		if strings.TrimSpace(s.HeaderName) == "" && s.Key != "" {
			out = append(out, Problem{"headerName", "Name the header its value is sent in."})
		}
	}
	if strings.ContainsAny(s.Region, "/?#&= ") {
		out = append(out, Problem{"region", "The region is one word, such as westeurope."})
	}
	return out
}

// Host is where the settings send requests, to show without the rest of
// the address.
func (s Settings) Host() string {
	var raw string
	switch s.Service {
	case DeepL:
		raw = deeplPro
		if strings.HasSuffix(strings.TrimSpace(s.Key), ":fx") {
			raw = deeplFree
		}
	case Google:
		raw = googleURL
	case Microsoft:
		raw = microsoftURL
	case OpenAI:
		raw = openAIURL
		if strings.TrimSpace(s.URL) != "" {
			raw = s.URL
		}
	case Anthropic:
		raw = anthropicURL
	default:
		raw = s.URL
	}
	if u, err := url.Parse(strings.TrimSpace(raw)); err == nil {
		return u.Host
	}
	return ""
}

// Segment is a segment in markup.
type Segment struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Request is an HTTP request to send.
type Request struct {
	Method string            `json:"method"`
	URL    string            `json:"url"`
	Header map[string]string `json:"header"`
	Body   string            `json:"body,omitempty"`
}

// MaxSegments is the most segments one request asks for.
const MaxSegments = 50

// Translate builds the request that asks for the translation of the
// segments from one language into another (BCP 47 tags).
func (s Settings) Translate(src, tgt string, segs []Segment) (Request, error) {
	if p := s.Check(); len(p) > 0 {
		return Request{}, errors.New(p[0].Message)
	}
	if len(segs) == 0 || len(segs) > MaxSegments {
		return Request{}, fmt.Errorf("ask for 1 to %d segments", MaxSegments)
	}
	key := strings.TrimSpace(s.Key)
	switch s.Service {
	case DeepL:
		u := deeplPro
		if strings.HasSuffix(key, ":fx") {
			u = deeplFree
		}
		texts := make([]string, len(segs))
		for i, sg := range segs {
			texts[i] = toTags(sg.Text)
		}
		body := map[string]any{"text": texts, "source_lang": deeplSource(src), "target_lang": deeplTarget(tgt), "tag_handling": "xml"}
		return jsonRequest(u, map[string]string{"Authorization": "DeepL-Auth-Key " + key}, body)
	case Google:
		q := make([]string, len(segs))
		for i, sg := range segs {
			q[i] = toTags(sg.Text)
		}
		body := map[string]any{"q": q, "source": googleLang(src), "target": googleLang(tgt), "format": "html"}
		return jsonRequest(googleURL+"?key="+url.QueryEscape(key), nil, body)
	case Microsoft:
		texts := make([]map[string]string, len(segs))
		for i, sg := range segs {
			texts[i] = map[string]string{"Text": toTags(sg.Text)}
		}
		q := url.Values{"api-version": {"3.0"}, "from": {microsoftLang(src)}, "to": {microsoftLang(tgt)}, "textType": {"html"}}
		h := map[string]string{"Ocp-Apim-Subscription-Key": key}
		if r := strings.TrimSpace(s.Region); r != "" && !strings.EqualFold(r, "global") {
			h["Ocp-Apim-Subscription-Region"] = r
		}
		return jsonRequest(microsoftURL+"?"+q.Encode(), h, texts)
	case OpenAI:
		system, user := prompt(src, tgt, segs)
		body := map[string]any{
			"model":           strings.TrimSpace(s.Model),
			"messages":        []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
			"response_format": map[string]string{"type": "json_object"},
		}
		return jsonRequest(s.openAIBase()+"/chat/completions", map[string]string{"Authorization": "Bearer " + key}, body)
	case Anthropic:
		system, user := prompt(src, tgt, segs)
		body := map[string]any{
			"model":      strings.TrimSpace(s.Model),
			"max_tokens": 8192,
			"system":     system,
			"messages":   []map[string]string{{"role": "user", "content": user}},
		}
		return jsonRequest(anthropicURL+"/messages", anthropicHeader(key), body)
	default:
		body := map[string]any{"sourceLang": src, "targetLang": tgt, "segments": segs}
		h := map[string]string{}
		if n := strings.TrimSpace(s.HeaderName); n != "" {
			h[n] = s.Key
		}
		return jsonRequest(strings.TrimSpace(s.URL), h, body)
	}
}

func (s Settings) openAIBase() string {
	if u := strings.TrimRight(strings.TrimSpace(s.URL), "/"); u != "" {
		return strings.TrimSuffix(u, "/chat/completions")
	}
	return openAIURL
}

func anthropicHeader(key string) map[string]string {
	return map[string]string{
		"x-api-key":         key,
		"anthropic-version": anthropicLevel,
		// Lets a web page ask the API itself, with the key the person
		// gave it for this.
		"anthropic-dangerous-direct-browser-access": "true",
	}
}

func jsonRequest(u string, header map[string]string, body any) (Request, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	// Tags and the text around them go as they are, not as escapes.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		return Request{}, err
	}
	h := map[string]string{"Content-Type": "application/json", "Accept": "application/json"}
	for k, v := range header {
		h[k] = v
	}
	return Request{Method: http.MethodPost, URL: u, Header: h, Body: b.String()}, nil
}

// Answer reads the service's answer to Translate: the translations, in
// markup, of the segments it has one for.
func (s Settings) Answer(segs []Segment, status int, body []byte) ([]Segment, error) {
	if status != http.StatusOK {
		return nil, failure(s.Service, status, body)
	}
	var texts []string
	switch s.Service {
	case DeepL:
		var got struct {
			Translations []struct {
				Text string `json:"text"`
			} `json:"translations"`
		}
		if json.Unmarshal(body, &got) != nil {
			return nil, errors.New("its answer could not be read")
		}
		for _, t := range got.Translations {
			texts = append(texts, fromTags(t.Text))
		}
	case Google:
		var got struct {
			Data struct {
				Translations []struct {
					Text string `json:"translatedText"`
				} `json:"translations"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &got) != nil {
			return nil, errors.New("its answer could not be read")
		}
		for _, t := range got.Data.Translations {
			texts = append(texts, fromTags(t.Text))
		}
	case Microsoft:
		var got []struct {
			Translations []struct {
				Text string `json:"text"`
			} `json:"translations"`
		}
		if json.Unmarshal(body, &got) != nil {
			return nil, errors.New("its answer could not be read")
		}
		for _, t := range got {
			if len(t.Translations) == 0 {
				texts = append(texts, "")
				continue
			}
			texts = append(texts, fromTags(t.Translations[0].Text))
		}
	case OpenAI:
		var got struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal(body, &got) != nil || len(got.Choices) == 0 {
			return nil, errors.New("its answer could not be read")
		}
		return modelAnswer(segs, got.Choices[0].Message.Content)
	case Anthropic:
		var got struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(body, &got) != nil {
			return nil, errors.New("its answer could not be read")
		}
		var b strings.Builder
		for _, c := range got.Content {
			if c.Type == "text" {
				b.WriteString(c.Text)
			}
		}
		return modelAnswer(segs, b.String())
	default:
		var got struct {
			Translations []Segment `json:"translations"`
		}
		if json.Unmarshal(body, &got) != nil {
			return nil, errors.New("its answer is not the JSON the contract asks for")
		}
		return keep(segs, got.Translations)
	}
	// The services that answer in the order asked.
	out := make([]Segment, 0, len(texts))
	for i, t := range texts {
		if i < len(segs) && t != "" {
			out = append(out, Segment{ID: segs[i].ID, Text: t})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("its answer has no translation of what was asked")
	}
	return out, nil
}

// keep returns the translations of the segments asked for, once each.
func keep(segs []Segment, got []Segment) ([]Segment, error) {
	asked := map[string]bool{}
	for _, s := range segs {
		asked[s.ID] = true
	}
	var out []Segment
	for _, t := range got {
		if asked[t.ID] {
			delete(asked, t.ID)
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("its answer has no translation of what was asked")
	}
	return out, nil
}

// prompt is what a language model is asked.
func prompt(src, tgt string, segs []Segment) (system, user string) {
	system = fmt.Sprintf(`You translate segments of a document from %s into %s for a professional translator, who reviews your translation.
Translate each segment's text faithfully and idiomatically, in the register of the source, as a whole document: the segments follow each other.
The texts contain tags: <1>...</1> marks a span (formatting, a link) and <2/> stands for something in its place (a picture, a field). Keep every tag, as it is written, around or at the words that correspond in your translation. "&lt;" is a < and "&amp;" an & of the text; write them the same way.
Answer with JSON only, in this form: {"translations": [{"id": "the segment's id", "text": "its translation"}]}, one entry for each segment, nothing else.`, languageName(src), languageName(tgt))
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(map[string]any{"segments": segs})
	return system, strings.TrimSpace(b.String())
}

// modelAnswer reads the JSON a language model was asked for, also when it
// wrote something around it.
func modelAnswer(segs []Segment, text string) ([]Segment, error) {
	text = strings.TrimSpace(text)
	if i, j := strings.Index(text, "{"), strings.LastIndex(text, "}"); i >= 0 && j > i {
		text = text[i : j+1]
	}
	var got struct {
		Translations []Segment `json:"translations"`
	}
	if json.Unmarshal([]byte(text), &got) != nil {
		return nil, errors.New("the model did not answer in the form it was asked for")
	}
	return keep(segs, got.Translations)
}

// failure says why a service refused, in words.
func failure(s Service, status int, body []byte) error {
	var said string
	var e struct {
		Message string `json:"message"`
		Error   any    `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil {
		switch x := e.Error.(type) {
		case map[string]any:
			if m, ok := x["message"].(string); ok {
				said = m
			}
		case string:
			said = x
		}
		if said == "" {
			said = e.Message
		}
	}
	if said == "" {
		var list []struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &list) == nil && len(list) > 0 {
			said = list[0].Error.Message
		}
	}
	if said == "" {
		said = strings.TrimSpace(string(body))
	}
	if len(said) > 200 {
		said = said[:200] + "…"
	}
	var why string
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		why = "the key was not accepted"
	case status == 456 && s == DeepL, status == http.StatusPaymentRequired:
		why = "the account's quota is used up"
	case status == http.StatusTooManyRequests:
		why = "it got too many requests; try again in a moment"
	case status == http.StatusNotFound && (s == OpenAI || s == Anthropic):
		why = "it does not know the model or the address"
	default:
		why = "it answered " + strconv.Itoa(status)
	}
	if said != "" {
		return fmt.Errorf("%s (%s)", why, said)
	}
	return errors.New(why)
}

// Models builds the request that lists the models the key may use, for
// the services that need a model; ok is false for the others.
func (s Settings) Models() (r Request, ok bool, err error) {
	key := strings.TrimSpace(s.Key)
	if key == "" {
		return Request{}, true, errors.New("paste the key first")
	}
	switch s.Service {
	case OpenAI:
		if strings.TrimSpace(s.URL) != "" {
			if p := (Settings{Service: OpenAI, URL: s.URL, Key: key, Model: "x"}).Check(); len(p) > 0 {
				return Request{}, true, errors.New(p[0].Message)
			}
		}
		return Request{Method: http.MethodGet, URL: s.openAIBase() + "/models", Header: map[string]string{"Authorization": "Bearer " + key, "Accept": "application/json"}}, true, nil
	case Anthropic:
		h := anthropicHeader(key)
		h["Accept"] = "application/json"
		return Request{Method: http.MethodGet, URL: anthropicURL + "/models?limit=1000", Header: h}, true, nil
	}
	return Request{}, false, nil
}

// notChat are parts of the names of OpenAI models that do not translate
// text: speech, pictures, embeddings and the like.
var notChat = []string{"embedding", "tts", "whisper", "dall-e", "image", "audio", "realtime", "transcribe", "moderation", "davinci", "babbage", "search", "computer-use", "sora"}

// ModelsAnswer reads the list of models, sorted by name.
func (s Settings) ModelsAnswer(status int, body []byte) ([]string, error) {
	if status != http.StatusOK {
		return nil, failure(s.Service, status, body)
	}
	var got struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &got) != nil {
		return nil, errors.New("its list of models could not be read")
	}
	var out []string
	for _, m := range got.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		skip := false
		for _, n := range notChat {
			skip = skip || strings.Contains(strings.ToLower(id), n)
		}
		if !skip {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil, errors.New("the key may use no model that translates text")
	}
	return out, nil
}

// MaxAnswer is the most of an answer that is read.
const MaxAnswer = 4 << 20

// Do sends a request and returns the status and body of the answer. The
// key never shows in an error: a failed request is described without its
// address's query.
func Do(ctx context.Context, client *http.Client, r Request) (int, []byte, error) {
	hr, err := http.NewRequestWithContext(ctx, r.Method, r.URL, strings.NewReader(r.Body))
	if err != nil {
		return 0, nil, errors.New("its address is not one a request can be sent to")
	}
	for k, v := range r.Header {
		hr.Header.Set(k, v)
	}
	res, err := client.Do(hr)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			return 0, nil, fmt.Errorf("its address could not be reached (%v)", ue.Err)
		}
		return 0, nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, MaxAnswer+1))
	if err != nil {
		return 0, nil, err
	}
	if len(body) > MaxAnswer {
		return 0, nil, errors.New("its answer is too large")
	}
	return res.StatusCode, body, nil
}

// Tags as the services that keep markup (XML for DeepL, HTML for Google
// and Microsoft) get them: <t1>…</t1> for the pair <1>…</1> and <t2/> for
// <2/>, where the number is the place of the tag's ID in the segment, so
// any ID becomes a name markup allows.

var markupTag = regexp.MustCompile(`<(/?)([^<>/\s]+)(/?)>`)

// toTags writes markup with the tags as elements. The IDs are written as
// they are, prefixed: the answer's tags are matched back by fromTags.
func toTags(text string) string {
	return markupTag.ReplaceAllStringFunc(text, func(m string) string {
		p := markupTag.FindStringSubmatch(m)
		return "<" + p[1] + "t" + idName(p[2]) + p[3] + ">"
	})
}

// idName writes a tag ID as the rest of an element name: digits stay,
// anything else as its code points.
func idName(id string) string {
	var b strings.Builder
	for _, r := range id {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			b.WriteRune(r)
			continue
		}
		fmt.Fprintf(&b, "_%x_", r)
	}
	return b.String()
}

var serviceTag = regexp.MustCompile(`<\s*(/?)\s*t([0-9A-Za-z_]+)\s*(/?)\s*>`)
var codePoint = regexp.MustCompile(`_([0-9a-f]+)_`)

// fromTags reads a service's answer back into markup: its tags become
// markup's, and its text, which may use any entity, is written with &lt;
// and &amp; only. Other markup the service added is left out.
func fromTags(text string) string {
	var b strings.Builder
	for text != "" {
		loc := serviceTag.FindStringSubmatchIndex(text)
		if loc == nil {
			b.WriteString(markupText(text))
			break
		}
		b.WriteString(markupText(text[:loc[0]]))
		closing, name, alone := text[loc[2]:loc[3]], text[loc[4]:loc[5]], text[loc[6]:loc[7]]
		id := codePoint.ReplaceAllStringFunc(name, func(m string) string {
			n, err := strconv.ParseInt(m[1:len(m)-1], 16, 32)
			if err != nil {
				return m
			}
			return string(rune(n))
		})
		b.WriteString("<" + closing + id + alone + ">")
		text = text[loc[1]:]
	}
	return b.String()
}

var otherTag = regexp.MustCompile(`<[^<>]*>`)

func markupText(t string) string {
	t = otherTag.ReplaceAllString(t, "")
	t = html.UnescapeString(t)
	return strings.NewReplacer("&", "&amp;", "<", "&lt;").Replace(t)
}

// Languages as the services name them.

func region(tag string) string {
	for _, p := range strings.Split(tag, "-")[1:] {
		if len(p) == 2 || (len(p) == 3 && p[0] >= '0' && p[0] <= '9') {
			return strings.ToUpper(p)
		}
	}
	return ""
}

func deeplSource(tag string) string { return strings.ToUpper(langtag.Primary(tag)) }

func deeplTarget(tag string) string {
	p := langtag.Primary(tag)
	switch p {
	case "en":
		switch region(tag) {
		case "US", "CA", "PH":
			return "EN-US"
		case "":
			return "EN-US"
		}
		return "EN-GB"
	case "pt":
		if region(tag) == "BR" {
			return "PT-BR"
		}
		return "PT-PT"
	case "zh":
		if langtag.Script(tag) == "Hant" {
			return "ZH-HANT"
		}
		return "ZH-HANS"
	}
	return strings.ToUpper(p)
}

func googleLang(tag string) string {
	p := langtag.Primary(tag)
	switch p {
	case "zh":
		if langtag.Script(tag) == "Hant" {
			return "zh-TW"
		}
		return "zh-CN"
	case "pt":
		if r := region(tag); r != "" && r != "BR" {
			return "pt-PT"
		}
	}
	return p
}

func microsoftLang(tag string) string {
	p := langtag.Primary(tag)
	switch p {
	case "zh":
		if langtag.Script(tag) == "Hant" {
			return "zh-Hant"
		}
		return "zh-Hans"
	case "pt":
		if r := region(tag); r != "" && r != "BR" {
			return "pt-pt"
		}
	case "fr":
		if region(tag) == "CA" {
			return "fr-ca"
		}
	case "sr":
		if langtag.Script(tag) == "Latn" {
			return "sr-Latn"
		}
		return "sr-Cyrl"
	}
	return p
}

func languageName(tag string) string {
	if l, ok := langtag.Lookup(tag); ok {
		return l.Name
	}
	if l, ok := langtag.Lookup(langtag.Primary(tag)); ok {
		return l.Name + " (" + tag + ")"
	}
	return tag
}
