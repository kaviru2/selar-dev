package quiz

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Draft is an editable quiz definition: what the admin editor saves and what
// an imported file becomes. It always passes Validate before it is stored.
type Draft struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Kind        Kind       `json:"kind"`
	Settings    Settings   `json:"settings"`
	Questions   []Question `json:"questions"`
}

// ShuffledIDs returns a deterministic permutation of ids for the given seed
// (an attempt id), so a reload shows the same order.
func ShuffledIDs(ids []string, seed string) []string {
	out := append([]string(nil), ids...)
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	sum := h.Sum64()
	r := rand.New(rand.NewPCG(sum, sum^0x9e3779b97f4a7c15))
	r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// ---------- validation ----------

var validKinds = map[Kind]bool{KindInitial: true, KindFollowUp: true, KindPractice: true}
var validFeedback = map[FeedbackPolicy]bool{FeedbackNever: true, FeedbackAfterSubmit: true, FeedbackAfterClose: true}

// Normalise fills defaults, assigns positions and option ids, then validates.
func (d *Draft) Normalise() error {
	d.Title = strings.TrimSpace(d.Title)
	d.Description = strings.TrimSpace(d.Description)
	if d.Kind == "" {
		d.Kind = KindPractice
	}
	if d.Settings.Feedback == "" {
		// Showing answers after a study test is itself a learning event that
		// could contaminate a later recall test, so study kinds default to never.
		d.Settings.Feedback = FeedbackAfterSubmit
		if d.Kind != KindPractice {
			d.Settings.Feedback = FeedbackNever
		}
	}
	if d.Settings.Audience.Type == "" {
		d.Settings.Audience.Type = AudienceAll
	}
	for i := range d.Questions {
		q := &d.Questions[i]
		q.Position = i
		q.Prompt = strings.TrimSpace(q.Prompt)
		if q.Type == TypeTrueFalse && len(q.Options) == 0 {
			q.Options = []Option{{ID: "true", Text: "True"}, {ID: "false", Text: "False"}}
		}
		used := map[string]bool{}
		for _, o := range q.Options {
			used[o.ID] = true
		}
		next := 1
		for j := range q.Options {
			q.Options[j].Text = strings.TrimSpace(q.Options[j].Text)
			if q.Options[j].ID == "" {
				for used["o"+strconv.Itoa(next)] {
					next++
				}
				q.Options[j].ID = "o" + strconv.Itoa(next)
				used[q.Options[j].ID] = true
			}
		}
	}
	return d.Validate()
}

// Validate reports the first problem with a draft in admin-readable terms.
func (d Draft) Validate() error {
	if d.Title == "" {
		return errors.New("title is required")
	}
	if !validKinds[d.Kind] {
		return fmt.Errorf("kind must be initial, follow_up or practice (got %q)", d.Kind)
	}
	s := d.Settings
	if !validFeedback[s.Feedback] {
		return fmt.Errorf("feedback must be never, after_submit or after_close (got %q)", s.Feedback)
	}
	switch s.Audience.Type {
	case AudienceAll:
	case AudienceCohort, AudienceGroup:
		if strings.TrimSpace(s.Audience.Value) == "" {
			return errors.New("audience must be all, cohort:<name> or group:<label>")
		}
	default:
		return errors.New("audience must be all, cohort:<name> or group:<label>")
	}
	if s.MaxAttempts < 0 || s.TimeLimitSeconds < 0 {
		return errors.New("max_attempts and time limit must not be negative")
	}
	if s.OpenAt != nil && s.CloseAt != nil && !s.CloseAt.After(*s.OpenAt) {
		return errors.New("close_at must be after open_at")
	}
	if a := s.After; a != nil {
		switch a.Anchor {
		case AnchorFirstReading:
		case AnchorDocument:
			if strings.TrimSpace(a.Document) == "" {
				return errors.New("after.document is required for the document anchor")
			}
		case AnchorQuiz:
			if strings.TrimSpace(a.QuizID) == "" {
				return errors.New("after.quiz_id is required for the quiz_submitted anchor")
			}
		default:
			return fmt.Errorf("after.anchor must be first_reading, document or quiz_submitted (got %q)", a.Anchor)
		}
		if a.Days < 0 || a.WindowDays < 0 {
			return errors.New("after.days and after.window_days must not be negative")
		}
	}
	if len(d.Questions) == 0 {
		return errors.New("a quiz needs at least one question")
	}
	for i, q := range d.Questions {
		if err := validateQuestion(q); err != nil {
			return fmt.Errorf("question %d: %w", i+1, err)
		}
	}
	return nil
}

func validateQuestion(q Question) error {
	if !q.Type.Valid() {
		return fmt.Errorf("unsupported type %q (use one of %v)", q.Type, QuestionTypes)
	}
	if q.Prompt == "" {
		return errors.New("prompt is required")
	}
	if q.Points < 0 || math.IsNaN(q.Points) || math.IsInf(q.Points, 0) {
		return errors.New("points must be zero or more")
	}
	if !q.Type.IsChoice() {
		if len(q.Options) > 0 {
			return fmt.Errorf("%s questions do not take options", q.Type)
		}
		return nil
	}
	if len(q.Options) < 2 {
		return errors.New("choice questions need at least two options")
	}
	ids := map[string]bool{}
	correct := 0
	for _, o := range q.Options {
		if o.Text == "" {
			return errors.New("option text must not be empty")
		}
		if ids[o.ID] {
			return fmt.Errorf("duplicate option id %q", o.ID)
		}
		ids[o.ID] = true
		if o.Correct {
			correct++
		}
	}
	switch q.Type {
	case TypeMultipleChoice:
		if correct == 0 {
			return errors.New("mark at least one correct option")
		}
	case TypeTrueFalse:
		if correct != 1 || len(q.Options) != 2 {
			return errors.New("true_false needs answer: true or false")
		}
	default:
		if correct != 1 {
			return errors.New("single_choice needs exactly one correct option")
		}
	}
	return nil
}

// ---------- file schema (YAML / JSON) ----------

type fileQuiz struct {
	Title            string         `yaml:"title"`
	Description      string         `yaml:"description,omitempty"`
	Kind             string         `yaml:"kind,omitempty"`
	Feedback         string         `yaml:"feedback,omitempty"`
	MaxAttempts      *int           `yaml:"max_attempts,omitempty"`
	TimeLimitMinutes float64        `yaml:"time_limit_minutes,omitempty"`
	OpenAt           string         `yaml:"open_at,omitempty"`
	CloseAt          string         `yaml:"close_at,omitempty"`
	Audience         string         `yaml:"audience,omitempty"`
	After            *fileAfter     `yaml:"after,omitempty"`
	ShuffleQuestions bool           `yaml:"shuffle_questions,omitempty"`
	ShuffleOptions   bool           `yaml:"shuffle_options,omitempty"`
	NoGoingBack      bool           `yaml:"no_going_back,omitempty"`
	Questions        []fileQuestion `yaml:"questions"`
}

type fileAfter struct {
	Anchor     string `yaml:"anchor"`
	Days       int    `yaml:"days"`
	WindowDays int    `yaml:"window_days,omitempty"`
	Document   string `yaml:"document,omitempty"`
	QuizID     string `yaml:"quiz_id,omitempty"`
}

type fileQuestion struct {
	Type        string       `yaml:"type"`
	Prompt      string       `yaml:"prompt"`
	Cue         string       `yaml:"cue,omitempty"`
	Options     []fileOption `yaml:"options,omitempty"`
	Answer      *bool        `yaml:"answer,omitempty"`
	Answers     []string     `yaml:"answers,omitempty"`
	Points      *float64     `yaml:"points,omitempty"`
	Rubric      string       `yaml:"rubric,omitempty"`
	Explanation string       `yaml:"explanation,omitempty"`
	Source      *fileSource  `yaml:"source,omitempty"`
}

type fileSource struct {
	Document string `yaml:"document,omitempty"`
	URL      string `yaml:"url,omitempty"`
}

type fileOption struct {
	Text    string `yaml:"text"`
	Correct bool   `yaml:"correct,omitempty"`
}

// UnmarshalYAML accepts "*Correct option" / "Wrong option" shorthand or {text, correct}.
func (o *fileOption) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		text := strings.TrimSpace(n.Value)
		if strings.HasPrefix(text, "*") {
			o.Correct, text = true, strings.TrimSpace(text[1:])
		}
		o.Text = text
		return nil
	}
	type plain fileOption
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*o = fileOption(p)
	return nil
}

func parseTime(field, v string) (*time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, fmt.Errorf("%s must be an RFC 3339 time with a time zone, e.g. 2026-10-10T09:00:00+05:30", field)
	}
	return &t, nil
}

func parseAudience(v string) (Audience, error) {
	v = strings.TrimSpace(v)
	if v == "" || v == "all" {
		return Audience{Type: AudienceAll}, nil
	}
	kind, value, ok := strings.Cut(v, ":")
	value = strings.TrimSpace(value)
	if ok && value != "" && (kind == "cohort" || kind == "group") {
		return Audience{Type: AudienceType(kind), Value: value}, nil
	}
	return Audience{}, fmt.Errorf("audience must be all, cohort:<name> or group:<label> (got %q)", v)
}

func (f fileQuiz) toDraft() (Draft, error) {
	d := Draft{Title: f.Title, Description: f.Description, Kind: Kind(f.Kind)}
	s := &d.Settings
	s.Feedback = FeedbackPolicy(f.Feedback)
	s.MaxAttempts = 1
	if f.MaxAttempts != nil {
		s.MaxAttempts = *f.MaxAttempts
	}
	s.TimeLimitSeconds = int(math.Round(f.TimeLimitMinutes * 60))
	var err error
	if s.OpenAt, err = parseTime("open_at", f.OpenAt); err != nil {
		return d, err
	}
	if s.CloseAt, err = parseTime("close_at", f.CloseAt); err != nil {
		return d, err
	}
	if s.Audience, err = parseAudience(f.Audience); err != nil {
		return d, err
	}
	if f.After != nil {
		s.After = &RelativeWindow{Anchor: Anchor(f.After.Anchor), Days: f.After.Days, WindowDays: f.After.WindowDays, Document: f.After.Document, QuizID: f.After.QuizID}
	}
	s.ShuffleQuestions, s.ShuffleOptions, s.NoGoingBack = f.ShuffleQuestions, f.ShuffleOptions, f.NoGoingBack
	for i, fq := range f.Questions {
		q := Question{Type: QuestionType(strings.TrimSpace(fq.Type)), Prompt: fq.Prompt, Cue: strings.TrimSpace(fq.Cue),
			AcceptedAnswers: fq.Answers, Rubric: strings.TrimSpace(fq.Rubric), Explanation: strings.TrimSpace(fq.Explanation), Points: 1}
		if fq.Points != nil {
			q.Points = *fq.Points
		}
		if fq.Source != nil {
			q.SourceDocument, q.SourceURL = strings.TrimSpace(fq.Source.Document), strings.TrimSpace(fq.Source.URL)
		}
		for _, o := range fq.Options {
			q.Options = append(q.Options, Option{Text: o.Text, Correct: o.Correct})
		}
		if q.Type == TypeTrueFalse {
			if fq.Answer == nil && len(q.Options) == 0 {
				return d, fmt.Errorf("question %d: true_false needs answer: true or false", i+1)
			}
			if fq.Answer != nil {
				q.Options = []Option{{ID: "true", Text: "True", Correct: *fq.Answer}, {ID: "false", Text: "False", Correct: !*fq.Answer}}
			}
		}
		d.Questions = append(d.Questions, q)
	}
	return d, d.Normalise()
}

func fromDraft(d Draft) fileQuiz {
	s := d.Settings
	f := fileQuiz{Title: d.Title, Description: d.Description, Kind: string(d.Kind), Feedback: string(s.Feedback),
		TimeLimitMinutes: float64(s.TimeLimitSeconds) / 60, ShuffleQuestions: s.ShuffleQuestions, ShuffleOptions: s.ShuffleOptions, NoGoingBack: s.NoGoingBack}
	attempts := s.MaxAttempts
	f.MaxAttempts = &attempts
	if s.OpenAt != nil {
		f.OpenAt = s.OpenAt.Format(time.RFC3339)
	}
	if s.CloseAt != nil {
		f.CloseAt = s.CloseAt.Format(time.RFC3339)
	}
	if s.Audience.Type != AudienceAll && s.Audience.Type != "" {
		f.Audience = string(s.Audience.Type) + ":" + s.Audience.Value
	}
	if a := s.After; a != nil {
		f.After = &fileAfter{Anchor: string(a.Anchor), Days: a.Days, WindowDays: a.WindowDays, Document: a.Document, QuizID: a.QuizID}
	}
	for _, q := range d.Questions {
		points := q.Points
		fq := fileQuestion{Type: string(q.Type), Prompt: q.Prompt, Cue: q.Cue, Answers: q.AcceptedAnswers, Points: &points, Rubric: q.Rubric, Explanation: q.Explanation}
		if q.SourceDocument != "" || q.SourceURL != "" {
			fq.Source = &fileSource{Document: q.SourceDocument, URL: q.SourceURL}
		}
		if q.Type == TypeTrueFalse {
			answer := len(q.Options) > 0 && q.Options[0].Correct
			fq.Answer = &answer
		} else {
			for _, o := range q.Options {
				fq.Options = append(fq.Options, fileOption{Text: o.Text, Correct: o.Correct})
			}
		}
		f.Questions = append(f.Questions, fq)
	}
	return f
}

// ExportYAML renders a draft in the import format, so a quiz can be copied
// between environments or versioned in git.
func ExportYAML(d Draft) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(fromDraft(d)); err != nil {
		return nil, err
	}
	return buf.Bytes(), enc.Close()
}

func decodeStrict(data []byte, into any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("could not read quiz file: %w", err)
	}
	return nil
}

// Parse reads a quiz file. The format is chosen by extension: .md/.markdown
// use the Markdown format, everything else is parsed as YAML (a superset of JSON).
func Parse(name string, data []byte) (Draft, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return parseMarkdown(data)
	}
	var f fileQuiz
	if err := decodeStrict(data, &f); err != nil {
		return Draft{}, err
	}
	return f.toDraft()
}

// ---------- Markdown ----------

var (
	mdHeading = regexp.MustCompile(`^##\s+([A-Za-z_\-]+)\s*(?:\((\d+(?:\.\d+)?)\s*points?\))?\s*$`)
	mdOption  = regexp.MustCompile(`^[-*]\s+\[( |x|X)\]\s+(.+)$`)
	mdField   = regexp.MustCompile(`^(Answer|Cue|Rubric|Explanation|Source|Points):\s*(.*)$`)
)

func parseMarkdown(data []byte) (Draft, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	var f fileQuiz
	start := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		end := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				end = i
				break
			}
		}
		if end < 0 {
			return Draft{}, errors.New("line 1: front matter starts with --- but never closes")
		}
		if err := decodeStrict([]byte(strings.Join(lines[1:end], "\n")), &f); err != nil && !strings.Contains(err.Error(), "EOF") {
			return Draft{}, err
		}
		start = end + 1
	}
	var desc, prompt []string
	var cur *fileQuestion
	flush := func() {
		if cur != nil {
			cur.Prompt = strings.TrimSpace(strings.Join(prompt, "\n"))
			f.Questions = append(f.Questions, *cur)
		}
		cur, prompt = nil, nil
	}
	for i := start; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], " \t")
		trimmed := strings.TrimSpace(line)
		lineNo := i + 1
		switch {
		case strings.HasPrefix(trimmed, "## "):
			flush()
			m := mdHeading.FindStringSubmatch(trimmed)
			if m == nil || !QuestionType(strings.ToLower(m[1])).Valid() {
				return Draft{}, fmt.Errorf("line %d: expected a question heading like \"## single_choice (2 points)\" using one of %v", lineNo, QuestionTypes)
			}
			cur = &fileQuestion{Type: strings.ToLower(m[1])}
			if m[2] != "" {
				p, _ := strconv.ParseFloat(m[2], 64)
				cur.Points = &p
			}
		case strings.HasPrefix(trimmed, "# ") && cur == nil && f.Questions == nil:
			if f.Title != "" && len(desc) > 0 {
				return Draft{}, fmt.Errorf("line %d: only one # title is allowed", lineNo)
			}
			f.Title = strings.TrimSpace(trimmed[2:])
		case cur == nil:
			if f.Title != "" && (trimmed != "" || len(desc) > 0) {
				desc = append(desc, trimmed)
			}
		default:
			if m := mdOption.FindStringSubmatch(trimmed); m != nil {
				cur.Options = append(cur.Options, fileOption{Text: strings.TrimSpace(m[2]), Correct: m[1] != " "})
			} else if m := mdField.FindStringSubmatch(trimmed); m != nil {
				v := strings.TrimSpace(m[2])
				switch m[1] {
				case "Answer":
					cur.Answers = append(cur.Answers, v)
				case "Cue":
					cur.Cue = v
				case "Rubric":
					cur.Rubric = strings.TrimSpace(cur.Rubric + "\n" + v)
				case "Explanation":
					cur.Explanation = v
				case "Points":
					p, err := strconv.ParseFloat(v, 64)
					if err != nil {
						return Draft{}, fmt.Errorf("line %d: Points must be a number", lineNo)
					}
					cur.Points = &p
				case "Source":
					if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
						cur.Source = &fileSource{URL: v}
					} else {
						cur.Source = &fileSource{Document: v}
					}
				}
			} else if trimmed != "" || len(prompt) > 0 {
				prompt = append(prompt, trimmed)
			}
		}
	}
	flush()
	f.Description = strings.TrimSpace(strings.Join(desc, "\n"))
	for i := range f.Questions {
		q := &f.Questions[i]
		if q.Type == string(TypeTrueFalse) && len(q.Options) > 0 {
			answer := false
			for _, o := range q.Options {
				if o.Correct {
					answer = strings.EqualFold(o.Text, "true")
				}
			}
			q.Options, q.Answer = nil, &answer
		}
	}
	return f.toDraft()
}
