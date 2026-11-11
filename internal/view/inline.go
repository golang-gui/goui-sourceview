package view

import (
	"image/color"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/platform/typography"
)

// InlineLayout owns an unpublished or cached multiline suggestion's layouts.
// The source paragraph retains its native text and geometry; the root editor
// reserves the measured extra extent without changing source row identities.
type InlineLayout struct {
	Head, Tail                 typography.TextLayout
	HeadGeometry, TailGeometry textcore.Geometry
	HeadText, TailText         string
	BodyWidth, HeadWidth       float32
	Wrap                       typography.WrapMode
	TabWidth                   float32
	Extra, Width               float32
	TailHeight, HeadExtra      float32
}

func (m *InlineLayout) Destroy() {
	if m.Head != nil {
		m.Head.Destroy()
		m.Head = nil
	}
	if m.Tail != nil {
		m.Tail.Destroy()
		m.Tail = nil
	}
}

// InlineMetrics borrows source metrics. Readers preserve their original lookup
// time after native callbacks; no document or selection state is retained.
type InlineMetrics struct {
	Font       typography.FontInfo
	Color      color.NRGBA
	LineHeight func() float32
	SourceLine typography.TextLine
	GeometryX  func() float64
	CaretX     float32
}

// Build owns all native probes until complete. A reentrant invalidation destroys
// unpublished resources and returns false without changing the editor's extent.
func (m *InlineLayout) Build(context func() typography.Context, first, rest string, metrics InlineMetrics, valid func() bool) (bool, error) {
	makeLayout := func(text string, width float32) (typography.TextLayout, textcore.Geometry, error) {
		l, err := context().NewTextLayout(text, typography.TextFormat{Font: metrics.Font, WrapMode: m.Wrap, TextColor: metrics.Color, TabWidth: m.TabWidth}, max(1, width), 1<<20)
		if err != nil {
			return nil, textcore.Geometry{}, err
		}
		if !valid() {
			l.Destroy()
			return nil, textcore.Geometry{}, nil
		}
		lines, clusters := l.MeasureMetrics()
		if !valid() {
			l.Destroy()
			return nil, textcore.Geometry{}, nil
		}
		return l, textcore.NewGeometry(lines, clusters, len(text), metrics.LineHeight()), nil
	}
	var err error
	overflow := ""
	if first != "" && m.HeadWidth >= 1 {
		m.HeadText = first
		m.Head, m.HeadGeometry, err = makeLayout(first, m.HeadWidth)
		if err != nil {
			m.Destroy()
			return false, err
		}
		if !valid() {
			m.Destroy()
			return false, nil
		}
		if m.HeadGeometry.LineCount() > 1 {
			at := m.HeadGeometry.LineMetrics(1).Start
			overflow = first[at:]
		}
	} else if first != "" {
		overflow = first
	}
	m.TailText = rest
	if overflow != "" {
		m.TailText = overflow + "\n" + rest
	}
	m.Tail, m.TailGeometry, err = makeLayout(m.TailText, m.BodyWidth)
	if err != nil {
		m.Destroy()
		return false, err
	}
	if !valid() {
		m.Destroy()
		return false, nil
	}
	tailWidth, tailHeight := m.Tail.MeasureSize()
	extraHead := float32(0)
	if m.Head != nil {
		h := m.HeadGeometry.LineMetrics(0)
		extraHead = max(0, (metrics.SourceLine.Baseline-metrics.SourceLine.Y)-(h.Baseline-h.Y)+h.Height-metrics.SourceLine.Height)
	}
	m.TailHeight, m.HeadExtra = tailHeight, extraHead
	m.Extra = extraHead + tailHeight
	m.Width = m.BodyWidth
	if m.Wrap == typography.WrapNone {
		m.Width = tailWidth
		if m.Head != nil {
			h := m.HeadGeometry.LineMetrics(0)
			m.Width = max(m.Width, CeilCoordinate(metrics.GeometryX()+float64(metrics.CaretX)+1+float64(h.Width)))
		}
	}
	if !valid() {
		m.Destroy()
		return false, nil
	}
	return true, nil
}
