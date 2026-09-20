// Package theme collects every colour decision in one place.
package theme

import "github.com/badvibecoder/cellsheet/internal/tui/render"

// Theme is the set of styles the interface draws with.
type Theme struct {
	Frame       render.Style
	ColHeader   render.Style
	ColHeaderHi render.Style
	RowHeader   render.Style
	RowHeaderHi render.Style
	Text        render.Style
	Number      render.Style
	Currency    render.Style
	Error       render.Style
	Dim         render.Style
	Cursor      render.Style
	CursorEdge  render.Style
	FormulaBar  render.Style
	RefBox      render.Style
	MenuTitle   render.Style
	TabActive   render.Style
	TabIdle     render.Style
	TabAdd      render.Style
	StatusKey   render.Style
	StatusVal   render.Style
	StatusMode  render.Style
	StatusWarn  render.Style
	PanelBorder render.Style
	PanelItem   render.Style
	PanelSel    render.Style
	PanelHint   render.Style
	SelText     render.Style
	SelEdge     render.Style
}

// Dark is the default theme, for a dark terminal.
func Dark() Theme {
	return Theme{
		Frame:       render.S(render.ColBrBlack),
		ColHeader:   render.S(render.ColBrBlack, render.AttrBold),
		ColHeaderHi: render.S(render.ColBrCyan, render.AttrBold),
		RowHeader:   render.S(render.ColBrBlack, render.AttrBold),
		RowHeaderHi: render.S(render.ColBrCyan, render.AttrBold),
		Text:        render.S(render.ColDefault),
		Number:      render.S(render.ColBrWhite),
		Currency:    render.S(render.ColBrGreen),
		Error:       render.S(render.ColBrRed, render.AttrBold),
		Dim:         render.S(render.ColBrBlack),
		Cursor:      render.S(render.ColBrWhite, render.AttrBold, render.AttrReverse),
		CursorEdge:  render.S(render.ColBrCyan, render.AttrBold),
		FormulaBar:  render.S(render.ColDefault),
		RefBox:      render.S(render.ColBrWhite, render.AttrBold),
		MenuTitle:   render.S(render.ColDefault),
		TabActive:   render.S(render.ColBrCyan, render.AttrBold),
		TabIdle:     render.S(render.ColBrBlack),
		TabAdd:      render.S(render.ColBrGreen, render.AttrBold),
		StatusKey:   render.S(render.ColBrBlack, render.AttrBold),
		StatusVal:   render.S(render.ColDefault),
		StatusMode:  render.S(render.ColBrGreen, render.AttrBold),
		StatusWarn:  render.S(render.ColBrYellow, render.AttrBold),
		PanelBorder: render.S(render.ColBrCyan),
		PanelItem:   render.S(render.ColDefault),
		PanelSel:    render.S(render.ColBrWhite, render.AttrBold, render.AttrReverse),
		PanelHint:   render.S(render.ColBrBlack),
		// Selection: a dark slate background reads clearly on a dark terminal
		// without hiding the text.
		SelText: render.Style{Fg: render.ColBrWhite, Bg: render.RGB(0x24, 0x3b, 0x59)},
		SelEdge: render.S(render.ColBrBlue, render.AttrBold),
	}
}

// Plain returns a theme with no colours at all, which is what the golden-file
// render tests compare against.
func Plain() Theme {
	var z render.Style
	return Theme{
		Frame: z, ColHeader: z, ColHeaderHi: z, RowHeader: z, RowHeaderHi: z,
		Text: z, Number: z, Currency: z, Error: z, Dim: z, Cursor: z,
		CursorEdge: z, FormulaBar: z, RefBox: z, MenuTitle: z, TabActive: z,
		TabIdle: z, TabAdd: z, StatusKey: z, StatusVal: z, StatusMode: z,
		StatusWarn: z, PanelBorder: z, PanelItem: z, PanelSel: z, PanelHint: z,
		SelText: render.Style{Bg: render.ColDefault}, SelEdge: z,
	}
}
