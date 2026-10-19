package language

func (s builtinState) Context() Context {
	if t := s.toml; t != nil {
		return Context{String: t.quote != 0, Comment: t.comment, Literal: t.quote == '\'', Value: !t.key}
	}
	if y := s.yaml; y != nil {
		return Context{String: y.quote != 0 || y.block && !y.blockHeader, Comment: y.comment, Literal: y.quote == '\''}
	}
	if sh := s.shell; sh != nil {
		mode := shTop(sh.stack)
		return Context{String: sh.active != nil || sh.capture || mode == shSingle || mode == shDouble || mode == shANSI, Comment: sh.comment, Literal: mode == shSingle || sh.active != nil && sh.active.literal}
	}

	return Context{
		String:  s.mode == jsonString || s.mode == attrSingle || s.mode == attrDouble,
		Comment: s.mode == comment || s.mode == cdata || s.mode == pi || s.mode == declaration || s.mode == scriptData || s.mode == rawText || s.mode == plainText,
		Tag:     s.pendingTag || s.mode == tagName || s.mode == tagInside || s.mode == attrName || s.mode == afterAttr || s.mode == beforeValue,
		TagName: s.pendingTag || s.mode == tagName,
		Value:   s.mode == beforeValue,
	}
}
