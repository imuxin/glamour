package ansi

import (
	"bytes"
	"fmt"
	"io"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	astext "github.com/yuin/goldmark/extension/ast"
)

// A TableElement is used to render tables.
type TableElement struct {
	lipgloss *table.Table
	table    *astext.Table
	header   []string
	row      []string
	source   []byte

	tableImages []tableLink
	tableLinks  []tableLink
}

// A TableRowElement is used to render a single row in a table.
type TableRowElement struct{}

// A TableHeadElement is used to render a table's head element.
type TableHeadElement struct{}

// A TableCellElement is used to render a single cell in a row.
type TableCellElement struct {
	Children []ElementRenderer
	Head     bool
}

// Render renders a TableElement.
func (e *TableElement) Render(w io.Writer, ctx RenderContext) error {
	bs := ctx.blockStack

	var indentation uint
	var margin uint
	rules := ctx.options.Styles.Table
	if rules.Indent != nil {
		indentation = *rules.Indent
	}
	if rules.Margin != nil {
		margin = *rules.Margin
	}

	iw := NewIndentWriter(w, int(indentation+margin), func(_ io.Writer) {
		_, _ = renderText(w, bs.Current().Style.StylePrimitive, " ")
	})
	defer iw.Close() //nolint:errcheck

	style := bs.With(rules.StylePrimitive)

	_, _ = renderText(iw, bs.Current().Style.StylePrimitive, rules.BlockPrefix)
	_, _ = renderText(iw, style, rules.Prefix)
	width := int(ctx.blockStack.Width(ctx))

	wrap := true
	if ctx.options.TableWrap != nil {
		wrap = *ctx.options.TableWrap
	}
	ctx.table.lipgloss = table.New().Width(width).Wrap(wrap)

	if err := e.collectLinksAndImages(ctx); err != nil {
		return err
	}

	return nil
}

func (e *TableElement) isGrid(ctx RenderContext) bool {
	return ctx.options.Styles.Table.Grid != nil && *ctx.options.Styles.Table.Grid
}

func (e *TableElement) isFitContent(ctx RenderContext) bool {
	return ctx.options.Styles.Table.FitContent != nil && *ctx.options.Styles.Table.FitContent
}

func (e *TableElement) cellStyle(ctx RenderContext, col, cellMargin int) lipgloss.Style {
	st := lipgloss.NewStyle().Inline(false)
	// Default horizontal margin unless squeezed out by an ultra-narrow viewport.
	if cellMargin > 0 {
		st = st.Margin(0, cellMargin)
	}

	// Override with custom styles
	if m := ctx.options.Styles.Table.Margin; m != nil {
		st = st.Padding(0, int(*m))
	}
	if col < len(e.table.Alignments) {
		switch e.table.Alignments[col] {
		case astext.AlignLeft:
			st = st.Align(lipgloss.Left).PaddingRight(0)
		case astext.AlignCenter:
			st = st.Align(lipgloss.Center)
		case astext.AlignRight:
			st = st.Align(lipgloss.Right).PaddingLeft(0)
		case astext.AlignNone:
			// do nothing
		}
	}

	return st
}

func (e *TableElement) setStyles(ctx RenderContext, cellMargin int) {
	docRules := ctx.options.Styles.Document
	if docRules.BackgroundColor != nil {
		baseStyle := lipgloss.NewStyle().Background(lipgloss.Color(*docRules.BackgroundColor))
		ctx.table.lipgloss.BaseStyle(baseStyle)
	}

	ctx.table.lipgloss = ctx.table.lipgloss.StyleFunc(func(_, col int) lipgloss.Style {
		return e.cellStyle(ctx, col, cellMargin)
	})
}

func (e *TableElement) setBorders(ctx RenderContext, hasRows bool) {
	rules := ctx.options.Styles.Table
	border := lipgloss.NormalBorder()
	grid := e.isGrid(ctx)

	if !grid && rules.RowSeparator != nil && rules.ColumnSeparator != nil {
		border = lipgloss.Border{
			Top:    *rules.RowSeparator,
			Bottom: *rules.RowSeparator,
			Left:   *rules.ColumnSeparator,
			Right:  *rules.ColumnSeparator,
			Middle: *rules.CenterSeparator,
		}
	}
	ctx.table.lipgloss.Border(border)
	ctx.table.lipgloss.BorderTop(grid)
	ctx.table.lipgloss.BorderLeft(grid)
	ctx.table.lipgloss.BorderRight(grid)
	ctx.table.lipgloss.BorderBottom(grid)
	ctx.table.lipgloss.BorderRow(grid)
	if grid && !hasRows {
		ctx.table.lipgloss.BorderHeader(false)
	}
	if rules.BorderColor != nil && *rules.BorderColor != "" {
		ctx.table.lipgloss.BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(*rules.BorderColor)))
	}
}

func (e *TableElement) tableMaxTotal(ctx RenderContext, t *table.Table, cols, cellMargin int) int {
	headers := t.GetHeaders()
	data := t.GetData()
	rows := data.Rows()
	maxTotal := 0
	for col := range cols {
		colMax := 0
		if col < len(headers) {
			colMax = max(colMax, lipgloss.Width(headers[col]))
		}
		for row := range rows {
			colMax = max(colMax, lipgloss.Width(data.At(row, col)))
		}
		maxTotal += colMax + e.cellStyle(ctx, col, cellMargin).GetHorizontalFrameSize()
	}
	return maxTotal
}

func (e *TableElement) applySizing(ctx RenderContext, t *table.Table, availableWidth, cols, borderW, cellMargin int, grid, fitContent bool) {
	if (!grid && !fitContent) || availableWidth <= 0 || cols == 0 {
		return
	}
	if fitContent {
		t.FitContent()
	}
	maxTotal := e.tableMaxTotal(ctx, t, cols, cellMargin)
	if maxTotal <= availableWidth && maxTotal+borderW > availableWidth && maxTotal-1 > borderW {
		// Lip Gloss resizer checks maxTotal() <= tableWidth without accounting
		// for horizontal borders, which skips shrinking and clips the right
		// border via MaxWidth. Nudge width below maxTotal to enter shrinking.
		t.Width(maxTotal - 1)
	}
}

// Finish finishes rendering a TableElement.
func (e *TableElement) Finish(_ io.Writer, ctx RenderContext) error {
	defer func() {
		ctx.table.lipgloss = nil
		ctx.table.tableImages = nil
		ctx.table.tableLinks = nil
	}()

	t := ctx.table.lipgloss
	if t == nil {
		return nil
	}

	rules := ctx.options.Styles.Table
	availableWidth := int(ctx.blockStack.Width(ctx))
	cols := max(len(t.GetHeaders()), t.GetData().Columns())
	hasRows := t.GetData().Rows() > 0
	grid := e.isGrid(ctx)
	fitContent := e.isFitContent(ctx)

	borderW := max(cols-1, 0)
	if grid && cols > 0 {
		borderW = cols + 1
	}

	cellMargin := 1
	if grid && availableWidth > 0 && cols > 0 && availableWidth < cols*3+borderW {
		cellMargin = 0
	}

	e.setStyles(ctx, cellMargin)
	e.setBorders(ctx, hasRows)
	e.applySizing(ctx, t, availableWidth, cols, borderW, cellMargin, grid, fitContent)

	ow := ctx.blockStack.Current().Block
	if _, err := ow.WriteString(t.String()); err != nil {
		return fmt.Errorf("glamour: error writing to buffer: %w", err)
	}

	_, _ = renderText(ow, ctx.blockStack.With(rules.StylePrimitive), rules.Suffix)
	_, _ = renderText(ow, ctx.blockStack.Current().Style.StylePrimitive, rules.BlockSuffix)

	e.printTableLinks(ctx)

	ctx.table.lipgloss = nil
	return nil
}

// Finish finishes rendering a TableRowElement.
func (e *TableRowElement) Finish(_ io.Writer, ctx RenderContext) error {
	if ctx.table.lipgloss == nil {
		return nil
	}

	ctx.table.lipgloss.Row(ctx.table.row...)
	ctx.table.row = []string{}
	return nil
}

// Finish finishes rendering a TableHeadElement.
func (e *TableHeadElement) Finish(_ io.Writer, ctx RenderContext) error {
	if ctx.table.lipgloss == nil {
		return nil
	}

	ctx.table.lipgloss.Headers(ctx.table.header...)
	ctx.table.header = []string{}
	return nil
}

// Render renders a TableCellElement.
func (e *TableCellElement) Render(_ io.Writer, ctx RenderContext) error {
	var b bytes.Buffer
	style := ctx.options.Styles.Table.StylePrimitive
	if e.Head {
		style = cascadeStylePrimitive(style, ctx.options.Styles.Table.Header, true)
	}
	for _, child := range e.Children {
		_, gated := child.(inlineLinkStyleOverrider)
		if r, ok := child.(StyleOverriderElementRenderer); ok && (!gated || ctx.options.HyperlinkMode == HyperlinkModeInline) {
			if err := r.StyleOverrideRender(&b, ctx, style); err != nil {
				return fmt.Errorf("glamour: error rendering with style: %w", err)
			}
		} else {
			var bb bytes.Buffer
			if err := child.Render(&bb, ctx); err != nil {
				return fmt.Errorf("glamour: error rendering: %w", err)
			}
			el := &BaseElement{
				Token: bb.String(),
				Style: style,
			}
			if err := el.Render(&b, ctx); err != nil {
				return err
			}
		}
	}

	if e.Head {
		ctx.table.header = append(ctx.table.header, b.String())
	} else {
		ctx.table.row = append(ctx.table.row, b.String())
	}

	return nil
}
