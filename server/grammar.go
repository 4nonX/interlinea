package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf16"

	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/project/projectdb"
	"github.com/4nonX/interlinea/qa"
)

// Grammar services: a checker behind an address that answers the check
// request of the LanguageTool HTTP API, which several free checkers
// speak. The server sends the request, so the service's key stays here.
//
// The contract. A request is a POST with a form body
// (application/x-www-form-urlencoded):
//
//	text=The translation.&language=en-GB&disabledCategories=TYPOS
//
// and the answer, with status 200:
//
//	{"matches": [{"message": "…", "offset": 4, "length": 11,
//	  "replacements": [{"value": "…"}],
//	  "rule": {"id": "…", "category": {"id": "GRAMMAR", "name": "Grammar"}}}]}
//
// offset and length count UTF-16 code units, as the API has them.
// Spelling is left out (the category TYPOS): the project's dictionary and
// its own words decide what is spelled right.

// maxGrammarText is how long a text one check may read, in bytes.
const maxGrammarText = 20000

// maxReplacements is how many replacements of a finding are passed on.
const maxReplacements = 5

// grammarMatch is something a grammar service found in a text.
type grammarMatch struct {
	// Offset and Length say where, in UTF-16 code units of the text.
	Offset  int    `json:"offset"`
	Length  int    `json:"length"`
	Message string `json:"message"`
	// Found is the text there.
	Found        string   `json:"found"`
	Replacements []string `json:"replacements"`
	// Rule and Category name what the service checked, in its words.
	Rule     string `json:"rule,omitempty"`
	Category string `json:"category,omitempty"`
}

// providerCheck asks a grammar service what it finds in a text of a
// language. The text is sent to the service's address, outside the
// server. Spelling is not asked for.
//
// Access: signed in.
func (s *Server) providerCheck(w http.ResponseWriter, r *http.Request, me session) {
	p, err := s.cfg.Projects.Provider(r.Context(), r.PathValue("name"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if p.Kind != projectdb.ProviderCheck {
		invalid(w, "name", fmt.Sprintf("%s is a translation service, not a grammar service.", p.Name))
		return
	}
	var req struct {
		Lang string `json:"lang"`
		Text string `json:"text"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	lang, err := langtag.Normalize(req.Lang)
	if err != nil {
		invalid(w, "lang", "Say the language the text is in.")
		return
	}
	if len(req.Text) > maxGrammarText {
		invalid(w, "text", fmt.Sprintf("A text to check is at most %d bytes long.", maxGrammarText))
		return
	}
	matches, err := askChecker(r.Context(), s.providerClient(), p, lang, req.Text)
	if err != nil {
		writeError(w, http.StatusBadGateway, "provider", fmt.Sprintf("%s did not check: %v.", p.Name, err))
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Matches []grammarMatch `json:"matches"`
	}{matches})
}

// askChecker asks a grammar service about a text. A service that refuses
// a language with a region (pt-AO) is asked again for the language alone.
func askChecker(ctx context.Context, client *http.Client, p projectdb.Provider, lang, text string) ([]grammarMatch, error) {
	if strings.TrimSpace(text) == "" {
		return []grammarMatch{}, nil
	}
	out, err := askCheckerOnce(ctx, client, p, lang, text)
	var refused *checkerRefusal
	if errors.As(err, &refused) {
		if primary, _, has := strings.Cut(lang, "-"); has {
			if again, err2 := askCheckerOnce(ctx, client, p, primary, text); err2 == nil {
				return again, nil
			}
		}
	}
	return out, err
}

// checkerRefusal is an answer of a status in the 400s.
type checkerRefusal struct{ said string }

func (e *checkerRefusal) Error() string { return e.said }

func askCheckerOnce(ctx context.Context, client *http.Client, p projectdb.Provider, lang, text string) ([]grammarMatch, error) {
	form := url.Values{"text": {text}, "language": {lang}, "disabledCategories": {"TYPOS"}}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, p.URL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	hr.Header.Set("Accept", "application/json")
	if p.HeaderName != "" {
		hr.Header.Set(p.HeaderName, p.HeaderValue)
	}
	res, err := client.Do(hr)
	if err != nil {
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
		said = fmt.Sprintf("it answered %d %s", res.StatusCode, said)
		if res.StatusCode >= 400 && res.StatusCode < 500 {
			return nil, &checkerRefusal{said}
		}
		return nil, errors.New(said)
	}
	return readMatches(answer, text)
}

// readMatches reads a service's answer about a text: the findings that
// lie in the text and are not about spelling, in the text's order.
func readMatches(answer []byte, text string) ([]grammarMatch, error) {
	var got struct {
		Matches *[]struct {
			Message      string `json:"message"`
			Offset       int    `json:"offset"`
			Length       int    `json:"length"`
			Replacements []struct {
				Value string `json:"value"`
			} `json:"replacements"`
			Rule struct {
				ID        string `json:"id"`
				IssueType string `json:"issueType"`
				Category  struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"category"`
			} `json:"rule"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(answer, &got); err != nil || got.Matches == nil {
		return nil, fmt.Errorf("its answer is not the JSON the contract asks for")
	}
	units := utf16.Encode([]rune(text))
	out := []grammarMatch{}
	for _, m := range *got.Matches {
		if m.Offset < 0 || m.Length <= 0 || m.Offset+m.Length > len(units) || strings.TrimSpace(m.Message) == "" {
			continue
		}
		if m.Rule.Category.ID == "TYPOS" || m.Rule.IssueType == "misspelling" {
			continue
		}
		g := grammarMatch{
			Offset: m.Offset, Length: m.Length, Message: strings.TrimSpace(m.Message),
			Found:        string(utf16.Decode(units[m.Offset : m.Offset+m.Length])),
			Replacements: []string{}, Rule: m.Rule.ID, Category: m.Rule.Category.Name,
		}
		for _, r := range m.Replacements {
			if len(g.Replacements) < maxReplacements && r.Value != g.Found {
				g.Replacements = append(g.Replacements, r.Value)
			}
		}
		out = append(out, g)
	}
	return out, nil
}

// grammarIssues asks the project's grammar service about the translation
// of each segment, for the quality report. Without a service there are
// none.
func (s *Server) grammarIssues(ctx context.Context, p projectdb.Project, lang string, segs []qa.Segment) ([]qa.Issue, error) {
	name := p.Settings.QA.Grammar
	if name == "" {
		return nil, nil
	}
	svc, err := s.cfg.Projects.Provider(ctx, name)
	if err != nil || svc.Kind != projectdb.ProviderCheck {
		return nil, fmt.Errorf("The project's grammar service, %s, is no longer there", name)
	}
	var out []qa.Issue
	for _, sg := range segs {
		if sg.Skip {
			continue
		}
		text := sg.Target.Plain()
		if len(text) > maxGrammarText {
			continue
		}
		matches, err := askChecker(ctx, s.providerClient(), svc, lang, text)
		if err != nil {
			return nil, fmt.Errorf("%s did not check: %v", svc.Name, err)
		}
		for _, m := range matches {
			out = append(out, qa.Issue{Segment: sg.ID, Kind: qa.Grammar, Severity: qa.Warning, Message: m.Message, Found: m.Found, Suggestions: m.Replacements})
		}
	}
	return out, nil
}
