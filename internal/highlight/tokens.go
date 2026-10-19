package highlight

import "github.com/golang-gui/goui-sourceview/language"

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
