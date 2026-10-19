package language

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Class describes lexical behavior independently of its color/style key.
type Class uint8

const (
	ClassString Class = 1 << iota
	ClassComment
	ClassDelimiter
)

// Token is a logical-line-relative UTF-8 byte range. Tokens exclude LF.
type Token struct {
	Start, End int
	Kind       Kind
	Class      Class
}

// Context describes the lexical position after consumed input. Data, when used,
// must be immutable. Tag/TagName/Value are optional markup/value hints.
type Context struct {
	String, Comment, Literal, Tag, TagName, Value bool
	Data                                          any
}

// State is an immutable scanner snapshot. Equal compares all future-relevant
// information exactly. Returned states must never be mutated by later calls.
type State interface {
	Equal(State) bool
	Context() Context
}

// EmptyState is available to stateless tokenizers.
type EmptyState struct{}

func (EmptyState) Equal(other State) bool { _, ok := other.(EmptyState); return ok }
func (EmptyState) Context() Context       { return Context{} }

// Input is one UTF-8 fragment excluding LF. Offset is its logical-line byte
// offset. Consume at least Limit bytes, with optional lookahead in Text. Only
// apply line-end transitions when all of Text has actually been consumed.
type Input struct {
	Text                 string
	Offset, Limit        int
	LineEnd, DocumentEnd bool
}

// Result appends line-relative tokens. ReplaceFrom explicitly retracts an
// earlier prefix of the CURRENT logical line before applying Tokens in order.
// It must lie in [0, Input.Offset]. Nil means append. Never revise another line.
// Tokens are ordered/non-overlapping and end at or before Offset+Consumed.
type Result struct {
	Consumed    int
	State       State
	Tokens      []Token
	ReplaceFrom *int
}

// Tokenizer is immutable and safe to share between documents. Scan is bounded
// by input size; no GUI objects or mutable document are accessible here.
type Tokenizer interface {
	InitialState() State
	Scan(Input, State) (Result, error)
}

// Pair defines an opening/closing sequence. Exclusions affect automatic
// insertion/surrounding; Brackets uses ClassDelimiter for structural matching.
type Pair struct {
	Open, Close               string
	NotInString, NotInComment bool
}
type PairContext struct {
	Typed, Before, After string
	Context              Context
	Known, Selected      bool
}
type NewlineContext struct {
	Before, After, Indent, Unit string
	Context                     Context
	Known                       bool
}

// NewlineEdit replaces the selection plus adjacent bytes. Caret is relative to
// the inserted Text. Returning handled=false preserves ordinary indentation.
type NewlineEdit struct {
	DeleteBefore, DeleteAfter int
	Text                      string
	Caret                     int
}
type Comments struct {
	Line  string
	Block Pair
}

// EditingRules are pure language policy. Word must be safe for concurrent use
// by completion helpers. AllowPair/Newline run on GUI and must not modify the
// document or retain mutable application state.
type EditingRules struct {
	Word                                         func(rune) bool
	AutoClosingPairs, SurroundingPairs, Brackets []Pair
	AutoCloseBefore                              string
	AllowPair                                    func(PairContext) bool
	Newline                                      func(NewlineContext) (edit NewlineEdit, handled bool)
	Comments                                     Comments
}

// Definition is editable input. New/Definition copy metadata and all slices.
// Functions and the Tokenizer must reference immutable language configuration.
type Definition struct {
	Info      Info
	Tokenizer Tokenizer
	Editing   EditingRules
	Styles    []Kind
	// Suggest returns static candidates and must be safe for concurrent use.
	Suggest func(Context) []string
}

// Language is an immutable, directly bindable definition. ID is descriptive,
// not a global registration key. Distinct objects with the same ID are valid.
type Language struct{ definition Definition }

func New(d Definition) (*Language, error) {
	if d.Info.ID == Plain && d.Tokenizer != nil {
		return nil, fmt.Errorf("language: tokenized language needs an ID")
	}
	if d.Info.Name == "" {
		return nil, fmt.Errorf("language: name is required")
	}
	for _, kind := range d.Styles {
		if !kind.Valid() {
			return nil, fmt.Errorf("language: invalid style %q", kind)
		}
	}
	for _, pairs := range [][]Pair{d.Editing.AutoClosingPairs, d.Editing.SurroundingPairs, d.Editing.Brackets} {
		for _, p := range pairs {
			if p.Open == "" || p.Close == "" || len(p.Open) > 128 || len(p.Close) > 128 || !utf8.ValidString(p.Open+p.Close) || strings.ContainsAny(p.Open+p.Close, "\r\n") {
				return nil, fmt.Errorf("language: invalid pair %q/%q", p.Open, p.Close)
			}
		}
	}
	for _, p := range d.Editing.Brackets {
		if p.Open == p.Close {
			return nil, fmt.Errorf("language: structural brackets need distinct sides")
		}
	}
	c := d.Editing.Comments
	if !utf8.ValidString(c.Line+c.Block.Open+c.Block.Close) || strings.ContainsAny(c.Line+c.Block.Open+c.Block.Close, "\r\n") || len(c.Line) > 128 || len(c.Block.Open) > 128 || len(c.Block.Close) > 128 || (c.Block.Open == "") != (c.Block.Close == "") {
		return nil, fmt.Errorf("language: invalid comment markers")
	}
	if d.Editing.Word == nil {
		d.Editing.Word = IdentifierRune
	}
	return &Language{copyDefinition(d)}, nil
}
func copyDefinition(d Definition) Definition {
	d.Info.Extensions = slices.Clone(d.Info.Extensions)
	d.Info.MIMETypes = slices.Clone(d.Info.MIMETypes)
	d.Editing.AutoClosingPairs = slices.Clone(d.Editing.AutoClosingPairs)
	d.Editing.SurroundingPairs = slices.Clone(d.Editing.SurroundingPairs)
	d.Editing.Brackets = slices.Clone(d.Editing.Brackets)
	d.Styles = slices.Clone(d.Styles)
	return d
}
func (l *Language) Definition() Definition { return copyDefinition(l.definition) }
func (l *Language) Info() Info             { return l.Definition().Info }
func (l *Language) ID() ID                 { return l.definition.Info.ID }
func (l *Language) Tokenizer() Tokenizer   { return l.definition.Tokenizer }
func (l *Language) Editing() EditingRules  { return l.Definition().Editing }
func (l *Language) Styles() []Kind         { return slices.Clone(l.definition.Styles) }
func (l *Language) Suggestions(c Context) []string {
	if l.definition.Suggest == nil {
		return nil
	}
	return slices.Clone(l.definition.Suggest(c))
}
func (l *Language) IsWord(r rune) bool { return l.definition.Editing.Word(r) }
func IdentifierRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r) || r == '_'
}
func StatesEqual(a, b State) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(b)
}

// ValidateResult checks an untrusted scanner result before publication.
func ValidateResult(in Input, r Result) error {
	if in.Limit < 0 || in.Limit > len(in.Text) || in.Offset < 0 || !utf8.ValidString(in.Text) || strings.ContainsRune(in.Text, '\n') {
		return fmt.Errorf("language: invalid scan input")
	}
	if r.State == nil || r.Consumed < in.Limit || r.Consumed > len(in.Text) || r.Consumed == 0 && in.Limit > 0 || !utf8.ValidString(in.Text[:r.Consumed]) {
		return fmt.Errorf("language: invalid scan progress/state")
	}
	start := in.Offset
	if r.ReplaceFrom != nil {
		start = *r.ReplaceFrom
		if start < 0 || start > in.Offset {
			return fmt.Errorf("language: invalid replacement start")
		}
	}
	for _, t := range r.Tokens {
		if t.Start < start || t.Start >= t.End || t.End > in.Offset+r.Consumed || !t.Kind.Valid() || t.Class&^(ClassString|ClassComment|ClassDelimiter) != 0 {
			return fmt.Errorf("language: invalid token %+v", t)
		}
		for _, at := range []int{t.Start, t.End} {
			if at >= in.Offset && at < in.Offset+len(in.Text) && !utf8.RuneStart(in.Text[at-in.Offset]) {
				return fmt.Errorf("language: token splits UTF-8")
			}
		}
		start = t.End
	}
	return nil
}
