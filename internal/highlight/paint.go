package highlight

// PaintTokens may retain colors whose predecessor states need revalidation.
// These tokens are display hints only; editing must use Tokens or ContextAt.
// Returned slices are borrowed and read-only until the next engine mutation.
func (e *Engine) PaintTokens(row int) ([]Token, uint64, bool) {
	line := e.Line(row)
	if line == nil || e.err != nil {
		return nil, 0, false
	}
	if e.syntax.Tokenizer() == nil {
		return nil, line.version, true
	}
	if line.loaded && line.definition == e.syntax {
		e.touch(line)
		return line.tokens, line.version, true
	}
	if e.work != nil && e.work.row == row {
		return e.work.tokens, e.work.version, true
	}
	return nil, line.version, false
}

// Map only surviving text ranges. An insertion strictly inside an old token
// inherits its display color; replacement text at unknown boundaries is plain.
// No old Class or state becomes valid for editing through this mapping.
func (e *Engine) mapPaint(lines []*Line, first, last *Line, prefix, suffixStart int, insertion bool) {
	available := func(line *Line) bool { return line.loaded && line.definition == e.syntax }
	appendToken := func(line *Line, token Token) {
		if len(line.tokens) < maxLineTokens {
			appendStyled(&line.tokens, token)
		}
	}
	copyRange := func(dst, src *Line, from, to, delta int) {
		if !available(src) {
			return
		}
		for _, token := range src.tokens {
			if token.End <= from {
				continue
			}
			if token.Start >= to {
				break
			}
			token.Start, token.End = max(from, token.Start)+delta, min(to, token.End)+delta
			appendToken(dst, token)
		}
	}
	copyRange(lines[0], first, 0, prefix, 0)
	end := lines[len(lines)-1]
	suffix := last.Bytes - suffixStart
	if insertion && available(first) {
		for _, token := range first.tokens {
			if token.Start < prefix && token.End > prefix {
				for i, line := range lines {
					start, stop := 0, line.Bytes
					if line.LF {
						stop--
					}
					if i == 0 {
						start = prefix
					}
					if i == len(lines)-1 {
						stop = line.Bytes - suffix
					}
					appendToken(line, Token{Start: start, End: stop, Kind: token.Kind, Class: token.Class})
				}
				break
			}
		}
	}
	copyRange(end, last, suffixStart, last.Bytes, end.Bytes-suffix-suffixStart)
	for i, line := range lines {
		if i == 0 && available(first) || i == len(lines)-1 && available(last) || len(line.tokens) > 0 {
			line.loaded, line.definition = true, e.syntax
			e.nextVersion++
			line.version = e.nextVersion
			e.payloadBytes += len(line.tokens) * 40
			e.touch(line)
		}
	}
}
