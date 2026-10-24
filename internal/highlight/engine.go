package highlight

import (
	"container/list"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

const ChunkSize = 16 * 1024

const tokenBudget = 16 * 1024 * 1024

const maxLineTokens = tokenBudget / 40

type interval struct{ start, end int }

// Splice describes logical-line replacement, before subsequent edits in a Change.
type Splice struct{ Index, Removed, Added int }

type work struct {
	row, at       int
	state, oldOut language.State
	oldComplete   bool
}

// Engine belongs to one active Document. All calls are sequential.
type Engine struct {
	model        *textedit.Model
	id           language.ID
	syntax       *language.Language
	revision     uint64
	lines        lineStore
	nextID       uint64
	building     bool
	buildRow     int
	dirty        []interval
	next         int
	work         *work
	lru          list.List
	elements     map[*Line]*list.Element
	payloadBytes int
	err          error
	scanned      int
}

// New creates an incremental analyzer without reading the document.
func New(model *textedit.Model, id language.ID) *Engine {
	l, _ := language.Builtin(id)
	return NewWithLanguage(model, l)
}

func NewWithLanguage(model *textedit.Model, l *language.Language) *Engine {
	id := l.ID()
	e := &Engine{model: model, id: id, syntax: l}
	e.Reset(id)
	return e
}

// SetLanguage invalidates lexical results while retaining unchanged text-line
// identities. The dirty interval keeps old results hidden until reanalyzed.
func (e *Engine) SetLanguage(id language.ID) { l, _ := language.Builtin(id); e.SetDefinition(l) }

func (e *Engine) SetDefinition(l *language.Language) {
	id := l.ID()
	if e.syntax == l {
		return
	}
	e.syntax = l
	if !e.Ready() {
		e.Reset(id)
		return
	}
	e.cancelWork()
	e.id, e.err, e.next, e.dirty = id, nil, -1, nil
	if e.syntax.Tokenizer() != nil {
		e.addDirty(0, e.LineCount())
		e.next = 0
	}
}

// Reset invalidates every old result and starts a bounded metadata rebuild.
func (e *Engine) Reset(id language.ID) {
	if e.syntax == nil || e.syntax.ID() != id {
		e.syntax, _ = language.Builtin(id)
	}
	e.id = id
	e.revision = e.model.Revision()
	e.lines = lineStore{}
	e.building = true
	e.buildRow = 0
	e.next = -1
	e.work = nil
	e.dirty = nil
	e.lru.Init()
	e.elements = make(map[*Line]*list.Element)
	e.payloadBytes = 0
	e.err = nil
}

func (e *Engine) newLine(bytes int, lf bool) *Line {
	e.nextID++
	return &Line{ID: e.nextID, Bytes: bytes, LF: lf}
}

func (e *Engine) Revision() uint64 { return e.revision }

func (e *Engine) Ready() bool { return !e.building && e.revision == e.model.Revision() }

func (e *Engine) Pending() bool { return e.err == nil && (e.building || e.next >= 0) }

func (e *Engine) Err() error { return e.err }

func (e *Engine) ScannedLines() int { return e.scanned }

func (e *Engine) Line(row int) *Line {
	if !e.Ready() {
		return nil
	}
	return e.lines.at(row)
}

func (e *Engine) Offset(row int) int { return e.lines.offset(row) }

func (e *Engine) LineCount() int { return count(e.lines.root) }

func (e *Engine) discard(line *Line) {
	if element := e.elements[line]; element != nil {
		e.lru.Remove(element)
		delete(e.elements, line)
	}
	e.payloadBytes -= len(line.tokens) * 40
	line.tokens = nil
	line.loaded = false
	line.limited = false
	line.version++
}

func (e *Engine) touch(line *Line) {
	if element := e.elements[line]; element != nil {
		e.lru.MoveToFront(element)
	} else {
		e.elements[line] = e.lru.PushFront(line)
	}
}

func (e *Engine) trim() {
	for e.payloadBytes > tokenBudget && e.lru.Len() > 1 {
		element := e.lru.Back()
		line := element.Value.(*Line)
		if e.work != nil && e.lines.at(e.work.row) == line {
			e.lru.MoveToFront(element)
			continue
		}
		e.discard(line)
	}
}

// Tokens returns borrowed read-only tokens and a paint version. Future stale
// rows are not exposed while an earlier lexical state is unresolved.
func (e *Engine) Tokens(row int) ([]Token, uint64, bool) {
	line := e.Line(row)
	if line == nil || e.err != nil {
		return nil, 0, false
	}
	if e.syntax.Tokenizer() == nil {
		return nil, line.version, true
	}
	valid := e.next < 0 || row < e.next || e.work != nil && row == e.work.row
	if !valid || !line.loaded {
		return nil, line.version, false
	}
	e.touch(line)
	return line.tokens, line.version, true
}

// Ensure schedules evicted visible payload without invalidating prior states.
func (e *Engine) Ensure(row int) {
	if e.syntax.Tokenizer() == nil {
		return
	}
	line := e.Line(row)
	if line == nil || line.loaded {
		return
	}
	e.addDirty(row, row+1)
	if e.next < 0 || row < e.next {
		e.cancelWork()
		e.next = row
	}
}

func (e *Engine) cancelWork() {
	if e.work != nil {
		if l := e.lines.at(e.work.row); l != nil {
			e.discard(l)
		}
		e.work = nil
	}
}

func (e *Engine) addDirty(start, end int) {
	if start >= end {
		return
	}
	e.dirty = append(e.dirty, interval{start, end})
	sort.Slice(e.dirty, func(i, j int) bool { return e.dirty[i].start < e.dirty[j].start })
	out := e.dirty[:0]
	for _, item := range e.dirty {
		if len(out) > 0 && item.start <= out[len(out)-1].end {
			out[len(out)-1].end = max(out[len(out)-1].end, item.end)
		} else {
			out = append(out, item)
		}
	}
	e.dirty = out
}

func (e *Engine) cleanRow(row int) {
	out := e.dirty[:0]
	for _, d := range e.dirty {
		if row < d.start || row >= d.end {
			out = append(out, d)
			continue
		}
		if d.start < row {
			out = append(out, interval{d.start, row})
		}
		if row+1 < d.end {
			out = append(out, interval{row + 1, d.end})
		}
	}
	e.dirty = out
}

func (e *Engine) firstDirty() int {
	if len(e.dirty) == 0 {
		return -1
	}
	return e.dirty[0].start
}

// Change updates old-coordinate metadata without reading current model text.
func (e *Engine) Change(c textedit.Change) ([]Splice, bool) {
	if c.Revision <= e.revision {
		return nil, false
	}
	if c.Reset || e.err != nil || e.building || c.Revision != e.revision+1 {
		e.Reset(e.id)
		return nil, true
	}
	e.cancelWork()
	var splices []Splice
	for _, edit := range c.Edits {
		first, last := e.lines.lineAt(edit.Range.Start), e.lines.lineAt(edit.Range.End)
		firstStart, lastStart := e.lines.offset(first), e.lines.offset(last)
		tail := e.lines.at(last)
		prefix := edit.Range.Start - firstStart
		suffix := tail.Bytes - (edit.Range.End - lastStart)
		var inserted []*Line
		length := prefix
		for i := 0; i < len(edit.Text); i++ {
			if edit.Text[i] == '\n' {
				inserted = append(inserted, e.newLine(length+1, true))
				length = 0
			} else {
				length++
			}
		}
		inserted = append(inserted, e.newLine(length+suffix, tail.LF))
		removed := last - first + 1
		for row := first; row <= last; row++ {
			e.discard(e.lines.at(row))
		}
		e.lines.splice(first, removed, inserted)
		delta := len(inserted) - removed
		for i, d := range e.dirty {
			oldEnd := first + removed
			if d.start >= oldEnd {
				d.start += delta
			} else if d.start >= first {
				d.start = first
			}
			if d.end >= oldEnd {
				d.end += delta
			} else if d.end > first {
				d.end = first + len(inserted)
			}
			e.dirty[i] = d
		}
		if e.next >= first+removed {
			e.next += delta
		}
		if e.next < 0 || first < e.next {
			e.next = first
		}
		e.addDirty(first, first+len(inserted))
		splices = append(splices, Splice{first, removed, len(inserted)})
	}
	if e.syntax.Tokenizer() == nil {
		e.next = -1
		e.dirty = nil
	}
	e.revision = c.Revision
	return splices, false
}

func (e *Engine) read(start, end int) (string, error) {
	for end > start && !e.model.IsRuneBoundary(end) {
		end--
	}
	return e.model.Slice(textedit.Range{Start: start, End: end})
}

func (e *Engine) buildStep() error {
	// Model already indexes logical newlines. Building lexical metadata must
	// not reread a multi-megabyte logical line just to count those same bytes.
	last := min(e.model.LineCount(), e.buildRow+128)
	block := make([]*Line, 0, last-e.buildRow)
	for e.buildRow < last {
		r, err := e.model.LineRange(e.buildRow)
		if err != nil {
			return err
		}
		lf := e.buildRow+1 < e.model.LineCount()
		length := r.End - r.Start
		if lf {
			length++
		}
		block = append(block, e.newLine(length, lf))
		e.buildRow++
	}
	e.lines.root = merge(e.lines.root, e.lines.tree(block))
	if e.buildRow < e.model.LineCount() {
		return nil
	}
	e.building = false
	if e.syntax.Tokenizer() != nil {
		e.addDirty(0, e.LineCount())
		e.next = 0
	}
	return nil
}

// Advance performs bounded pure analysis. It never invokes application callbacks.
func (e *Engine) Advance(deadline time.Time) bool {
	if e.err != nil || e.revision != e.model.Revision() {
		return false
	}
	changed := false
	for e.Pending() && (deadline.IsZero() || time.Now().Before(deadline)) {
		changed = true
		if e.building {
			if err := e.buildStep(); err != nil {
				e.err = err
			}
			continue
		}
		if e.next >= e.LineCount() {
			e.next = e.firstDirty()
			continue
		}
		if e.work == nil {
			line := e.lines.at(e.next)
			state := e.syntax.Tokenizer().InitialState()
			if e.next > 0 {
				state = e.lines.at(e.next - 1).out
			}
			e.work = &work{row: e.next, state: state, oldOut: line.out, oldComplete: line.complete}
			line.in = state
			e.discard(line)
			line.loaded = true
			e.touch(line)
		}
		w := e.work
		line := e.lines.at(w.row)
		length := line.Bytes
		if line.LF {
			length--
		}
		lineStart := e.lines.offset(w.row)
		absolute := lineStart + w.at
		end := lineStart + length
		text, err := e.read(absolute, min(end, absolute+ChunkSize+32))
		if err != nil {
			e.err = err
			break
		}
		limit := min(ChunkSize, len(text))
		for limit < len(text) && !e.model.IsRuneBoundary(absolute+limit) {
			limit--
		}
		in := language.Input{Text: text, Offset: w.at, Limit: limit, LineEnd: absolute+len(text) == end, DocumentEnd: absolute+len(text) == e.model.Len()}
		result, scanErr := e.syntax.Tokenizer().Scan(in, w.state)
		if scanErr == nil {
			scanErr = language.ValidateResult(in, result)
		}
		if scanErr != nil {
			e.err = fmt.Errorf("highlight: line %d: %w", w.row, scanErr)
			break
		}
		n, next := result.Consumed, result.State
		oldLen := len(line.tokens)
		if result.ReplaceFrom != nil {
			start := *result.ReplaceFrom
			if !e.model.IsRuneBoundary(lineStart + start) {
				e.err = fmt.Errorf("highlight: replacement splits UTF-8 at line %d", w.row)
				break
			}
			for len(line.tokens) > 0 && line.tokens[len(line.tokens)-1].End > start {
				last := &line.tokens[len(line.tokens)-1]
				if last.Start < start {
					last.End = start
					break
				}
				line.tokens = line.tokens[:len(line.tokens)-1]
			}
		}
		for _, token := range result.Tokens {
			if !e.model.IsRuneBoundary(lineStart+token.Start) || !e.model.IsRuneBoundary(lineStart+token.End) {
				e.err = fmt.Errorf("highlight: token splits UTF-8 at line %d", w.row)
				break
			}
			if len(line.tokens) >= maxLineTokens {
				line.limited = true
				break
			}
			appendStyled(&line.tokens, token)
		}
		e.payloadBytes += (len(line.tokens) - oldLen) * 40
		line.version++
		w.at += n
		w.state = next
		if w.at == length {
			line.out = next
			line.complete = true
			e.scanned++
			e.cleanRow(w.row)
			stop := w.oldComplete && language.StatesEqual(next, w.oldOut)
			e.next = w.row + 1
			e.work = nil
			if stop || e.next == e.LineCount() {
				e.next = e.firstDirty()
			}
		}
		e.trim()
	}
	return changed
}

// Text reconstructs only a requested logical line; used by independent tests.
func (e *Engine) Text(row int) (string, error) {
	l := e.Line(row)
	if l == nil {
		return "", fmt.Errorf("highlight: index not ready")
	}
	n := l.Bytes
	if l.LF {
		n--
	}
	start := e.lines.offset(row)
	return e.model.Slice(textedit.Range{Start: start, End: start + n})
}

// TokenText is a diagnostic helper for fixtures.
func TokenText(text string, tokens []Token) string {
	var parts []string
	for _, t := range tokens {
		parts = append(parts, t.Kind.String()+":"+text[t.Start:t.End])
	}
	return strings.Join(parts, "|")
}

type Context = language.Context

// ContextAt never forces unbounded analysis or guesses unresolved predecessors.
func (e *Engine) ContextAt(at int) (Context, bool) {
	if e.err != nil {
		return Context{}, false
	}
	row := e.model.LineAt(at)
	r, _ := e.model.LineRange(row)
	if at-r.Start > 16<<10 {
		return Context{}, false
	}
	tokenizer := e.syntax.Tokenizer()
	if tokenizer == nil {
		return Context{}, true
	}
	state := tokenizer.InitialState()
	if row > 0 {
		if !e.Ready() || e.next >= 0 && row-1 >= e.next {
			return Context{}, false
		}
		previous := e.Line(row - 1)
		if previous == nil || !previous.complete {
			return Context{}, false
		}
		state = previous.out
	}
	end := min(r.End, at+32)
	for !e.model.IsRuneBoundary(end) {
		end--
	}
	prefix, err := e.model.Slice(textedit.Range{Start: r.Start, End: end})
	if err != nil {
		return Context{}, false
	}
	in := language.Input{Text: prefix, Limit: at - r.Start}
	result, err := tokenizer.Scan(in, state)
	if err != nil || language.ValidateResult(in, result) != nil || result.Consumed != in.Limit {
		return Context{}, false
	}
	return result.State.Context(), true
}

type Token = language.Token

func appendStyled(out *[]Token, t Token) {
	for len(*out) > 0 && (*out)[len(*out)-1].End > t.Start {
		last := &(*out)[len(*out)-1]
		if last.Start < t.Start {
			last.End = t.Start
			break
		}
		*out = (*out)[:len(*out)-1]
	}
	if t.Start >= t.End {
		return
	}
	if t.Kind == language.KindPlain && t.Class == 0 {
		return
	}
	if len(*out) > 0 {
		last := &(*out)[len(*out)-1]
		if last.End == t.Start && last.Kind == t.Kind && last.Class == t.Class {
			last.End = t.End
			return
		}
	}
	*out = append(*out, t)
}
