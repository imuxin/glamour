package ansi

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"io"
	"net/url"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// A LinkElement is used to render hyperlinks.
type LinkElement struct {
	BaseURL       string
	URL           string
	Children      []ElementRenderer
	SkipText      bool
	SkipHref      bool
	FallbackToURL bool

	hyperlink, resetHyperlink string
	validURL                  bool
}

// Render renders a LinkElement.
func (e *LinkElement) Render(w io.Writer, ctx RenderContext) error {
	return e.renderWithStyles(w, ctx, ctx.options.Styles.LinkText, ctx.options.Styles.Link)
}

// StyleOverrideRender renders a LinkElement while inheriting outer visual styles
// such as surrounding bold/italic emphasis.
func (e *LinkElement) StyleOverrideRender(w io.Writer, ctx RenderContext, style StylePrimitive) error {
	textStyle := cascadeStylePrimitive(style, ctx.options.Styles.LinkText, false)
	hrefStyle := cascadeStylePrimitive(style, ctx.options.Styles.Link, false)
	return e.renderWithStyles(w, ctx, textStyle, hrefStyle)
}

func (*LinkElement) isInlineLinkStyleOverrider() {}

func (e *LinkElement) renderWithStyles(w io.Writer, ctx RenderContext, textStyle, hrefStyle StylePrimitive) error {
	// Make OSC 8 hyperlink token.
	link := resolveRelativeURL(e.BaseURL, e.URL)
	e.hyperlink, e.resetHyperlink, e.validURL = makeHyperlink(link)
	inline := ctx.options.HyperlinkMode == HyperlinkModeInline && e.validURL && !e.SkipText
	if inline {
		underline := true
		textStyle.Underline = &underline
	}

	if !e.SkipText {
		if e.FallbackToURL {
			var textBuf bytes.Buffer
			if err := e.renderTextPart(&textBuf, ctx, textStyle); err != nil {
				return err
			}
			if strings.TrimSpace(ansi.Strip(textBuf.String())) != "" {
				if _, err := io.WriteString(w, textBuf.String()); err != nil {
					return fmt.Errorf("glamour: error writing hyperlink text: %w", err)
				}
			} else if resolved := strings.TrimSpace(link); resolved != "" {
				var fb bytes.Buffer
				el := &BaseElement{
					Token: resolved,
					Style: hrefStyle,
				}
				if err := el.Render(&fb, ctx); err != nil {
					return err
				}
				if _, err := io.WriteString(w, e.hyperlink+fb.String()+e.resetHyperlink); err != nil {
					return fmt.Errorf("glamour: error writing fallback hyperlink: %w", err)
				}
				return nil
			} else if textBuf.Len() > 0 {
				if _, err := io.WriteString(w, textBuf.String()); err != nil {
					return fmt.Errorf("glamour: error writing hyperlink text: %w", err)
				}
			}
		} else if err := e.renderTextPart(w, ctx, textStyle); err != nil {
			return err
		}
	}
	if !inline && !e.SkipHref {
		if err := e.renderHrefPart(w, ctx, hrefStyle, link); err != nil {
			return err
		}
	}
	return nil
}

func (e *LinkElement) renderTextPart(w io.Writer, ctx RenderContext, textStyle StylePrimitive) error {
	for _, child := range e.Children {
		if child == nil {
			continue
		}
		_, gated := child.(inlineLinkStyleOverrider)
		if r, ok := child.(StyleOverriderElementRenderer); ok && (!gated || ctx.options.HyperlinkMode == HyperlinkModeInline) { //nolint:nestif
			var b bytes.Buffer
			if err := r.StyleOverrideRender(&b, ctx, textStyle); err != nil {
				return fmt.Errorf("glamour: error rendering with style: %w", err)
			}

			token := e.hyperlink + b.String() + e.resetHyperlink
			if _, err := io.WriteString(w, token); err != nil {
				return fmt.Errorf("glamour: error writing hyperlink: %w", err)
			}
		} else {
			var b bytes.Buffer
			if err := child.Render(&b, ctx); err != nil {
				return fmt.Errorf("glamour: error rendering: %w", err)
			}
			token := e.hyperlink + b.String() + e.resetHyperlink
			el := &BaseElement{
				Token: token,
				Style: textStyle,
			}
			if err := el.Render(w, ctx); err != nil {
				return fmt.Errorf("glamour: error rendering: %w", err)
			}
		}
	}
	return nil
}

func (e *LinkElement) renderHrefPart(w io.Writer, ctx RenderContext, hrefStyle StylePrimitive, link string) error {
	prefix := ""
	if !e.SkipText {
		prefix = " "
	}

	if e.validURL {
		token := e.hyperlink + link + e.resetHyperlink
		el := &BaseElement{
			Token:  token,
			Prefix: prefix,
			Style:  hrefStyle,
		}
		if err := el.Render(w, ctx); err != nil {
			return err
		}
	}
	return nil
}

// makeHyperlink takes a URL and returns an OSC 8 hyperlink token.
func makeHyperlink(link string) (string, string, bool) {
	// Make OSC 8 hyperlink token.
	var hyperlink, resetHyperlink string

	u, err := url.Parse(link)
	validURL := err == nil && "#"+u.Fragment != link // if the URL only consists of an anchor, ignore it
	if validURL {
		h := fnv.New32a()
		if _, err := io.WriteString(h, link); err != nil {
			return "", "", false
		}
		urlID := fmt.Sprintf("id=%d", h.Sum32())
		hyperlink = ansi.SetHyperlink(link, urlID)
		resetHyperlink = ansi.ResetHyperlink()
	}

	return hyperlink, resetHyperlink, validURL
}
