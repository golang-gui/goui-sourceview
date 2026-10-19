package highlight

import "github.com/golang-gui/goui-sourceview/language"

// The original scanner regression corpus now exercises the public streaming
// contract. Keep its fragment-relative harness and independent expectations.
type State struct {
	raw    language.State
	id     language.ID
	offset int
}

func Initial() State { return State{} }
func (s State) Equal(t State) bool {
	if s.raw == nil && t.raw == nil {
		return true
	}
	if s.raw == nil {
		l, _ := language.Builtin(t.id)
		s.raw = l.Tokenizer().InitialState()
	}
	if t.raw == nil {
		l, _ := language.Builtin(s.id)
		t.raw = l.Tokenizer().InitialState()
	}
	return language.StatesEqual(s.raw, t.raw)
}
func Scan(id language.ID, s State, text string, limit int, lineEnd bool) ([]Token, State, int) {
	l, _ := language.Builtin(id)
	scanner := l.Tokenizer()
	if scanner == nil {
		return nil, State{raw: language.EmptyState{}, id: id}, limit
	}
	if s.raw == nil {
		s.raw = scanner.InitialState()
	}
	in := language.Input{Text: text, Offset: s.offset, Limit: limit, LineEnd: lineEnd}
	result, err := scanner.Scan(in, s.raw)
	if err != nil {
		panic(err)
	}
	if err = language.ValidateResult(in, result); err != nil {
		panic(err)
	}
	tokens := result.Tokens
	for i := range tokens {
		tokens[i].Start -= s.offset
		tokens[i].End -= s.offset
	}
	next := State{raw: result.State, id: id, offset: s.offset + result.Consumed}
	if lineEnd && result.Consumed == len(text) {
		next.offset = 0
	}
	return tokens, next, result.Consumed
}
func emit(out *[]Token, start, end int, kind language.Kind) {
	appendStyled(out, Token{Start: start, End: end, Kind: kind})
}
func appendToken(out *[]Token, start, end int, kind language.Kind) { emit(out, start, end, kind) }
