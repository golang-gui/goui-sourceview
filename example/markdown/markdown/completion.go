package markdown

import (
	"context"
	"image/color"
	"strings"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/style"
)

// Provider returns only this application's candidates, in its chosen order.
// It reads a snapshot and can be moved to a project index without GUI access.
func Provider() *source.CompletionProvider {
	return &source.CompletionProvider{TriggerCharacters: "[@", Complete: complete}
}
func complete(ctx context.Context, c source.CompletionContext) ([]source.Completion, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.Lexical.Embedded != nil {
		return source.DefaultCompletions(c), nil
	}
	if !c.ContextKnown || c.Lexical.Comment || c.Lexical.String {
		return nil, nil
	}
	if strings.HasSuffix(c.Before, "[") {
		r := textedit.Range{Start: c.Caret - 1, End: c.Caret}
		if strings.HasPrefix(c.After, "]") {
			r.End++
		}
		at := len("[label](")
		return []source.Completion{{Label: "Link", Text: "[label](https://example.com)", Detail: "Choose a URL", Range: &r, Caret: &at}}, nil
	}
	if at := len(c.Before) - len(c.Prefix) - 1; at >= 0 && c.Before[at] == '@' {
		r := textedit.Range{Start: c.Range.Start - 1, End: c.Range.End}
		return []source.Completion{{Label: "GOUI reference", Text: "[GOUI][goui]", Detail: "Also add a reference definition", Range: &r, AdditionalEdits: []textedit.Edit{{Range: textedit.Range{Start: c.DocumentLength, End: c.DocumentLength}, Text: "\n\n[goui]: https://github.com/golang-gui/goui\n"}}}}, nil
	}
	if !c.Explicit && c.Prefix == "" {
		return nil, nil
	}
	entries := []source.Completion{{Label: "Heading", Text: "# Heading", Detail: "ATX heading"}, {Label: "Code block", Text: "```go\n\n```", Detail: "Fenced code"}, {Label: "Task list", Text: "- [ ] Task", Detail: "Unchecked task"}, {Label: "Link", Text: "[label](https://example.com)", Detail: "Inline link"}}
	var out []source.Completion
	for _, item := range entries {
		if c.Prefix == "" || strings.Contains(strings.ToLower(item.Label), strings.ToLower(c.Prefix)) {
			out = append(out, item)
		}
	}
	return out, nil
}

// Rules colors custom style keys through the normal GOUI stylesheet. UI chrome
// remains supplied by Modern. Syntax colors preserve the selected text's hue.
func Rules(dark bool) []style.Rule {
	colors := map[string]color.NRGBA{
		Heading.String(): {R: 20, G: 80, B: 180, A: 255}, Emphasis.String(): {R: 137, G: 40, B: 137, A: 255}, Code.String(): {R: 153, G: 60, B: 35, A: 255}, Fence.String(): {R: 100, G: 105, B: 115, A: 255}, Link.String(): {R: 0, G: 113, B: 135, A: 255}, Marker.String(): {R: 155, G: 100, B: 25, A: 255},
	}
	if dark {
		colors = map[string]color.NRGBA{Heading.String(): {R: 117, G: 179, B: 255, A: 255}, Emphasis.String(): {R: 218, G: 156, B: 228, A: 255}, Code.String(): {R: 221, G: 166, B: 128, A: 255}, Fence.String(): {R: 148, G: 158, B: 175, A: 255}, Link.String(): {R: 100, G: 204, B: 200, A: 255}, Marker.String(): {R: 221, G: 193, B: 112, A: 255}}
	}
	// Deterministic order also makes the example's stylesheet reproducible.
	var out []style.Rule
	for _, kind := range []string{Heading.String(), Emphasis.String(), Code.String(), Fence.String(), Link.String(), Marker.String()} {
		out = append(out, style.Name("source-view").Part("token-"+kind).ForegroundColor(colors[kind]))
	}
	return out
}

const Sample = "# Markdown editor\n\nA small editor built with **custom language rules**.\n\n- [ ] Type a list item and press Enter\n- [x] Try `inline code`, [a link](https://github.com/golang-gui/goui)\n\n> A quote keeps its prefix on Enter.\n\n```go\nfunc main() { println(\"Hello, SourceView\") }\n```\n\n<!-- Brackets [here] do not participate in matching. -->\n\nType [ for a link, @ for a reference, or F2 for templates.\n"
