package main

// -case typing -auto uses real Window.DispatchEvent/CommitText. Test stable
// Popover identity/visibility through typing, deletion, pending providers and
// in-word navigation; reject stale acceptance; check history and cancellation.
// Native X11 map/unmap observation is performed externally in the private test
// desktop. It is not a physical IME/compositor or other-platform claim.
import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
)

func verifyTyping(app gui.Application, win gui.Window, v *source.SourceView, doc *source.Document, key func(events.Key, events.Modifiers), probe *scaleProbe, dark bool) {
	original := doc.Model().Text()
	start := strings.Index(original, "truffle")
	primary, _ := gui.ModPrimary.Resolve()
	var popup gui.Popover
	var steps []func() bool
	next := func(f func()) { steps = append(steps, func() bool { f(); return true }) }
	ready := func() bool {
		if len(win.Popovers()) != 1 {
			return false
		}
		info := candidates(win.Popovers()[0])
		return info.ItemCount > 0 && len(info.Children) == min(8, info.ItemCount) && info.Attributes["sourceview.completion.pending"] != true
	}
	stable := func() {
		check(len(win.Popovers()) == 1 && win.Popovers()[0] == popup && popup.Visible() && v.Focused(), "typing closed/recreated popup or lost focus")
	}
	labels := func() string {
		var out []string
		for _, r := range candidates(win.Popovers()[0]).Children {
			out = append(out, r.Text)
		}
		return strings.Join(out, ",")
	}
	begin := func(phase string) { stable(); must(win.SetTitle("SourceView completion typing STABLE_BEGIN " + phase)) }
	end := func(phase string) { stable(); must(win.SetTitle("SourceView completion typing STABLE_END " + phase)) }
	typeText := func(k events.Key, text string) { key(k, 0); stable(); must(win.CommitText(text)); stable() }
	steps = append(steps, func() bool {
		return probe.painter != "" && v.Focused() && v.Snapshot().TextEditing != nil && v.Snapshot().TextEditing.VisibleText != ""
	})
	next(func() { key(events.KeyF2, 0) })
	steps = append(steps, ready)
	next(func() { popup = win.Popovers()[0]; begin("default"); key(events.KeyArrowDown, 0) })
	next(func() {
		typeText(events.KeyA, "a")
	})
	steps = append(steps, ready)
	next(func() {
		check(labels() == "trace,trade", "typing did not refilter whole word")
		check(candidates(popup).Children[1].Selected, "typing lost surviving selection")
	})
	next(func() {
		key(events.KeyBackspace, 0)
		stable()
	})
	steps = append(steps, ready)
	next(func() { check(labels() == "trace,trade,true", "backspace did not restore candidates") })
	next(func() {
		key(events.KeyShift, events.ModifierShift)
		stable()
		key(events.KeyA, events.ModifierShift)
		stable()
		must(win.CommitText("A"))
		stable()
	})
	steps = append(steps, ready)
	next(func() {
		check(labels() == "trace,trade", "shift typing candidates")
		key(events.KeyBackspace, 0)
		stable()
	})
	steps = append(steps, ready)
	next(func() {
		key(events.KeyDelete, 0)
		stable()
		check(doc.Model().Text() == original[:start+2]+original[start+3:], "forward deletion changed wrong range")
	})
	steps = append(steps, ready)
	next(func() { key(events.KeyArrowLeft, 0); stable(); key(events.KeyArrowRight, 0); stable() })
	next(func() { end("default") })
	var before string
	var selection textedit.Selection
	next(func() {
		before = doc.Model().Text()
		selection = v.Selection()
		key(events.KeyEnter, 0)
		check(len(win.Popovers()) == 0 && doc.Model().Text() == "trade trace true\ntrade tail", "typed completion used stale range")
		key(events.KeyZ, primary)
		check(doc.Model().Text() == before && v.Selection() == selection, "typed completion undo")
	})
	reset := func() {
		key(events.KeyEscape, 0)
		v.SetAutomaticCompletion(true)
		v.SetCompletionProvider(nil)
		doc.Model().SetText(original)
		must(doc.SetLanguage(language.Plain))
		v.SetSelection(textedit.Selection{Anchor: start + 2, Caret: start + 2})
	}
	next(func() { reset(); v.SetAutomaticCompletion(false); key(events.KeyF2, 0) })
	steps = append(steps, ready)
	next(func() { popup = win.Popovers()[0]; begin("manual") })
	next(func() { typeText(events.KeyA, "a") })
	next(func() { key(events.KeyBackspace, 0); stable() })
	next(func() {
		end("manual")
		key(events.KeyEscape, 0)
		check(len(win.Popovers()) == 0, "escape kept completion")
	})
	requests := make(chan source.CompletionContext, 16)
	replies := make(chan []source.Completion, 16)
	provider := &source.CompletionProvider{Complete: func(ctx context.Context, c source.CompletionContext) ([]source.Completion, error) {
		requests <- c
		select {
		case out := <-replies:
			return out, nil
		case <-time.After(6 * time.Second):
			return nil, fmt.Errorf("typing provider release timed out")
		}
	}}
	var request source.CompletionContext
	await := func(prefix, trigger string) func() bool {
		return func() bool {
			select {
			case request = <-requests:
				check(request.Prefix == prefix && request.Trigger == trigger && request.Explicit && request.Range.Start == start, "wrong typing provider context")
				return true
			default:
				return false
			}
		}
	}
	next(func() { reset(); v.SetCompletionProvider(provider); key(events.KeyF2, 0) })
	steps = append(steps, await("tr", ""))
	next(func() { replies <- []source.Completion{{Label: "trace"}, {Label: "trade"}} })
	steps = append(steps, ready)
	next(func() { popup = win.Popovers()[0]; begin("pending") })
	next(func() { typeText(events.KeyA, "a") })
	steps = append(steps, await("tra", "a"))
	next(func() {
		info := candidates(popup)
		check(info.Attributes["sourceview.completion.pending"] == true && !info.Children[0].Enabled && len(info.Children[0].Actions) == 0, "pending typing exposed an action")
		old := doc.Model().Text()
		key(events.KeyEnter, 0)
		key(events.KeyTab, 0)
		key(events.KeyArrowDown, 0)
		stable()
		check(doc.Model().Text() == old, "pending typing committed a stale candidate")
		typeText(events.KeyD, "d")
		replies <- []source.Completion{{Label: "stale"}}
	})
	steps = append(steps, await("trad", "d"))
	next(func() {
		stable()
		check(labels() == "trace,trade", "stale provider replaced preview")
		replies <- []source.Completion{{Label: "trade"}}
	})
	steps = append(steps, ready)
	next(func() {
		end("pending")
		check(labels() == "trade", "latest provider was lost")
		before = doc.Model().Text()
		selection = v.Selection()
		key(events.KeyTab, 0)
		check(doc.Model().Text() == "trade trace true\ntrade tail", "latest typing range not committed")
		key(events.KeyZ, primary)
		check(doc.Model().Text() == before && v.Selection() == selection, "provider typing undo")
	})
	// Start typing before the first request has returned. Only the latest
	// request may create the first popup; no growing parallel provider queue.
	next(func() { reset(); v.SetCompletionProvider(provider); key(events.KeyF2, 0) })
	steps = append(steps, await("tr", ""))
	next(func() {
		key(events.KeyA, 0)
		must(win.CommitText("a"))
		check(len(win.Popovers()) == 0, "initial stale request created popup")
		replies <- []source.Completion{{Label: "stale"}}
	})
	steps = append(steps, await("tra", "a"))
	next(func() {
		check(len(win.Popovers()) == 0, "canceled initial request was published")
		replies <- []source.Completion{{Label: "trade"}}
	})
	steps = append(steps, ready)
	next(func() { check(labels() == "trade", "latest initial typing request lost"); key(events.KeyEscape, 0) })
	next(func() { reset(); key(events.KeyF2, 0) })
	steps = append(steps, ready)
	next(func() {
		key(events.KeySpace, 0)
		must(win.CommitText(" "))
		check(len(win.Popovers()) == 0, "word boundary retained completion")
	})
	next(func() { reset(); key(events.KeyF2, 0) })
	steps = append(steps, ready)
	next(func() {
		doc.Model().SetText("external")
		check(len(win.Popovers()) == 0, "external edit retained completion")
	})
	next(func() { reset(); key(events.KeyF2, 0) })
	steps = append(steps, ready)
	for _, sample := range []struct {
		phase       string
		id          language.ID
		text        string
		caret       int
		key         events.Key
		typed, want string
	}{
		{"go", language.Go, "package main\nfu", len("package main\nfu"), events.KeyN, "n", "func"},
		{"embedded", language.Markdown, "```json\ntr\n```", len("```json\ntr"), events.KeyU, "u", "true"},
	} {
		next(func() {
			reset()
			must(doc.SetLanguage(sample.id))
			doc.Model().SetText(sample.text)
			v.SetSelection(textedit.Selection{Anchor: sample.caret, Caret: sample.caret})
		})
		next(func() { key(events.KeyF2, 0) })
		steps = append(steps, ready)
		next(func() {
			popup = win.Popovers()[0]
			begin(sample.phase)
			check(labels() == sample.want, "initial code-language candidates")
		})
		next(func() { typeText(sample.key, sample.typed) })
		steps = append(steps, ready)
		next(func() {
			check(labels() == sample.want, "code-language typing candidates")
			key(events.KeyBackspace, 0)
			stable()
		})
		steps = append(steps, ready)
		next(func() {
			end(sample.phase)
			check(doc.Model().Text() == sample.text, "code-language Backspace range")
			key(events.KeyEscape, 0)
		})
	}
	next(func() {
		reset()
		doc.Model().SetText("中文字词 中文")
		at := len("中文字词 中文")
		v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
		key(events.KeyF2, 0)
	})
	steps = append(steps, ready)
	next(func() { popup = win.Popovers()[0]; begin("unicode") })
	next(func() { typeText(events.KeyUnknown, "字") })
	steps = append(steps, ready)
	next(func() { key(events.KeyBackspace, 0); stable() })
	steps = append(steps, ready)
	next(func() {
		end("unicode")
		check(doc.Model().Text() == "中文字词 中文", "unicode Backspace deleted wrong range")
		key(events.KeyEscape, 0)
	})
	next(func() { reset(); key(events.KeyF2, 0) })
	steps = append(steps, ready)
	var handle interface{ Disconnect() }
	next(func() {
		handle = v.ConnectChange(func(textedit.Change) {
			if v.Document() == doc {
				v.SetDocument(source.NewDocument("replacement"))
			}
		})
		key(events.KeyA, 0)
		must(win.CommitText("a"))
		check(v.Document().Model().Text() == "replacement" && len(win.Popovers()) == 0, "typing callback replacement was overwritten")
		handle.Disconnect()
	})
	index, done := 0, false
	deadline := time.Now().Add(20 * time.Second)
	timer := app.NewTimer()
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), fmt.Sprintf("typing step %d timed out; popups=%d text=%q", index, len(win.Popovers()), v.Document().Model().Text()))
		if index < len(steps) {
			if steps[index]() {
				index++
			}
			return
		}
		timer.Stop()
		done = true
		log.Printf("PASS completion typing: stable popup, default/manual refresh, Backspace, pending/stale/initial providers, history, boundaries and reentry; typography=%s painter=%s scale=%g dark=%t", app.Typography().Name(), probe.painter, probe.scale, dark)
		win.RequestClose()
	})
	must(timer.Start(35 * time.Millisecond))
	app.Run()
	check(done, "typing verification aborted")
}
