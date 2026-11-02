package language

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	KindAdded   Kind = "diff.added"
	KindRemoved Kind = "diff.removed"
)

type diffState struct {
	kind       Kind
	head       string
	old, next  uint64
	body, hunk bool
}

func (s diffState) Equal(other State) bool { t, ok := other.(diffState); return ok && s == t }
func (diffState) Context() Context         { return Context{String: true, Literal: true} }

type diffTokenizer struct{}

func (diffTokenizer) InitialState() State { return diffState{} }

func diffCount(text string, sign byte) (uint64, bool) {
	if len(text) < 2 || text[0] != sign {
		return 0, false
	}
	rangeParts := strings.Split(text[1:], ",")
	if len(rangeParts) > 2 {
		return 0, false
	}
	if _, err := strconv.ParseUint(rangeParts[0], 10, 64); err != nil {
		return 0, false
	}
	if len(rangeParts) == 1 {
		return 1, true
	}
	n, err := strconv.ParseUint(rangeParts[1], 10, 64)
	return n, err == nil
}

func (diffTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(diffState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign diff state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	if in.Offset == 0 {
		s.kind, s.head, s.body, s.hunk = KindPlain, "", false, false
		text := in.Text
		if s.old > 0 || s.next > 0 {
			s.body = strings.HasPrefix(text, "+") || strings.HasPrefix(text, "-") || strings.HasPrefix(text, " ")
		}
		switch {
		case strings.HasPrefix(text, "@@"):
			s.kind, s.hunk = KindMeta, true
			s.old, s.next = 0, 0
		case strings.HasPrefix(text, "\\"):
			s.kind = KindComment
		case !s.body && (strings.HasPrefix(text, "--- ") || strings.HasPrefix(text, "+++ ") || strings.HasPrefix(text, "*** ")):
			s.kind, s.old, s.next = KindHeading, 0, 0
		case strings.HasPrefix(text, "+"):
			s.kind = KindAdded
		case strings.HasPrefix(text, "-"):
			s.kind = KindRemoved
		default:
			for _, prefix := range []string{"diff ", "Index:", "index ", "===", "new file mode ", "deleted file mode ", "old mode ", "new mode ", "similarity index ", "dissimilarity index ", "rename from ", "rename to ", "copy from ", "copy to ", "Binary files ", "GIT binary patch"} {
				if strings.HasPrefix(text, prefix) {
					s.kind, s.old, s.next = KindMeta, 0, 0
					break
				}
			}
		}
	}
	if s.hunk && len(s.head) < 128 {
		s.head += in.Text[:min(limit, 128-len(s.head))]
	}
	w.add(in.Offset, in.Offset+limit, s.kind, ClassString)
	if in.LineEnd && limit == len(in.Text) {
		if s.hunk {
			fields := strings.Fields(s.head)
			if len(fields) >= 4 && fields[0] == "@@" && fields[3] == "@@" {
				a, aok := diffCount(fields[1], '-')
				b, bok := diffCount(fields[2], '+')
				if aok && bok {
					s.old, s.next = a, b
				}
			}
		} else if s.body {
			if s.kind != KindAdded && s.old > 0 {
				s.old--
			}
			if s.kind != KindRemoved && s.next > 0 {
				s.next--
			}
		}
		s.kind, s.head, s.body, s.hunk = KindPlain, "", false, false
	}
	return w.result(limit, s), nil
}
