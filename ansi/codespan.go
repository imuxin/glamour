package ansi

import "io"

// A CodeSpanElement is used to render codespan.
type CodeSpanElement struct {
	Text  string
	Style StylePrimitive
}

// Render renders a CodeSpanElement.
func (e *CodeSpanElement) Render(w io.Writer, _ RenderContext) error {
	_, _ = renderText(w, e.Style, e.Style.Prefix+e.Text+e.Style.Suffix)
	return nil
}

// StyleOverrideRender renders a CodeSpanElement while inheriting outer visual
// attributes (such as link underline or emphasis) without losing inline code
// colors or running Markdown backslash unescaping on literal code text.
func (e *CodeSpanElement) StyleOverrideRender(w io.Writer, _ RenderContext, style StylePrimitive) error {
	merged := cascadeStylePrimitive(style, e.Style, false)
	_, _ = renderText(w, merged, merged.Prefix+e.Text+merged.Suffix)
	return nil
}

func (*CodeSpanElement) isInlineLinkStyleOverrider() {}
