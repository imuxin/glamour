package ansi_test

import (
	"bytes"
	"strings"
	"testing"

	glamouransi "charm.land/glamour/v2/ansi"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

func renderTableGrid(t *testing.T, mdText string, width int, style glamouransi.StyleConfig) string {
	t.Helper()
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Linkify,
			extension.Table,
			extension.Strikethrough,
		),
	)
	md.SetRenderer(renderer.NewRenderer(renderer.WithNodeRenderers(
		util.Prioritized(glamouransi.NewRenderer(glamouransi.Options{
			Styles:        style,
			WordWrap:      width,
			HyperlinkMode: glamouransi.HyperlinkModeInline,
		}), 1000),
	)))
	var out bytes.Buffer
	if err := md.Convert([]byte(mdText), &out); err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	return out.String()
}

func newGridTableTestStyle() glamouransi.StyleConfig {
	st := newHideLinkTestStyle()
	st.Table.Grid = boolPtr(true)
	st.Table.FitContent = boolPtr(true)
	st.Table.BorderColor = stringPtr("#626B8C")
	st.Table.Header.Bold = boolPtr(true)
	st.Table.Header.Color = stringPtr("#F4F4F5")
	return st
}

func tableLines(rendered string) []string {
	var out []string
	for _, line := range strings.Split(rendered, "\n") {
		if strings.TrimSpace(xansi.Strip(line)) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func trimRightPlain(line string) string {
	return strings.TrimRight(xansi.Strip(line), " ")
}

func TestTableGridCompactWidthAndBorders(t *testing.T) {
	md := strings.Join([]string{
		"| 键 | 值 |",
		"| --- | --- |",
		"| a | 1 |",
		"| b | 2 |",
	}, "\n")

	out := renderTableGrid(t, md, 120, newGridTableTestStyle())
	lines := tableLines(out)
	if len(lines) != 7 {
		t.Fatalf("expected 7 lines (top, hdr, sep, row0, sep, row1, bottom), got %d:\n%s", len(lines), xansi.Strip(out))
	}

	plainTop := trimRightPlain(lines[0])
	plainBottom := trimRightPlain(lines[len(lines)-1])
	if !strings.HasPrefix(plainTop, "┌") || !strings.HasSuffix(plainTop, "┐") {
		t.Fatalf("expected closed top border, got %q", plainTop)
	}
	if !strings.HasPrefix(plainBottom, "└") || !strings.HasSuffix(plainBottom, "┘") {
		t.Fatalf("expected closed bottom border, got %q", plainBottom)
	}
	if !strings.Contains(trimRightPlain(lines[4]), "├") || !strings.Contains(trimRightPlain(lines[4]), "┼") || !strings.Contains(trimRightPlain(lines[4]), "┤") {
		t.Fatalf("expected row separator between body rows, got %q", trimRightPlain(lines[4]))
	}
	// Compact natural width: should stay far narrower than viewport width 120.
	if w := xansi.StringWidth(plainTop); w >= 40 {
		t.Fatalf("expected compact table width < 40, got %d (%q)", w, plainTop)
	}
	// Border color #626B8C -> RGB(98, 107, 140)
	if !strings.Contains(lines[0], "38;2;98;107;140") {
		t.Fatalf("expected border color SGR in top border, got %q", lines[0])
	}
}

func TestTableGridHeaderOnlyAvoidsDoubleBottomBorder(t *testing.T) {
	md := strings.Join([]string{
		"| 列一 | 列二 |",
		"| --- | --- |",
	}, "\n")

	out := renderTableGrid(t, md, 80, newGridTableTestStyle())
	lines := tableLines(out)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines for header-only table (top, header, bottom), got %d:\n%s", len(lines), xansi.Strip(out))
	}
	if !strings.HasPrefix(trimRightPlain(lines[0]), "┌") || !strings.HasPrefix(trimRightPlain(lines[2]), "└") {
		t.Fatalf("unexpected header-only table borders:\n%s", xansi.Strip(out))
	}
}

func TestTableGridMultiSpanHeaderPreservesBold(t *testing.T) {
	md := strings.Join([]string{
		"| 普通标题 | 前半 `code` 后半 | [链接标题](https://example.com/h) 尾部 |",
		"| --- | --- | --- |",
		"| a | b | c |",
	}, "\n")

	out := renderTableGrid(t, md, 100, newGridTableTestStyle())
	lines := tableLines(out)
	if len(lines) < 2 {
		t.Fatalf("unexpected output: %q", out)
	}
	headerLine := lines[1]

	// Every visible word in the header line must be emitted with bold SGR active.
	for _, token := range []string{"普通标题", "前半", "code", "后半", "链接标题", "尾部"} {
		idx := strings.Index(headerLine, token)
		if idx < 0 {
			t.Fatalf("missing header token %q in %q", token, headerLine)
		}
		// Inspect active SGR right before this token.
		if !boldActiveAt(headerLine, idx) {
			t.Fatalf("header token %q lost bold SGR in %q", token, headerLine)
		}
	}
}

func boldActiveAt(s string, pos int) bool {
	bold := false
	for i := 0; i < pos; {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == ';' || s[j] == ':') {
				j++
			}
			if j < len(s) && s[j] == 'm' {
				params := s[i+2 : j]
				if params == "" {
					bold = false
				} else {
					parts := strings.Split(params, ";")
					for k := 0; k < len(parts); k++ {
						switch parts[k] {
						case "", "0":
							bold = false
						case "1":
							bold = true
						case "22":
							bold = false
						case "38", "48", "58":
							if k+1 < len(parts) {
								if parts[k+1] == "2" {
									k += 4
								} else if parts[k+1] == "5" {
									k += 2
								}
							}
						}
					}
				}
				i = j + 1
				continue
			}
		}
		i++
	}
	return bold
}

func TestTableGridWidthSweepRightBorderClosedAndLinksContained(t *testing.T) {
	const (
		docURL  = "https://example.com/docs/table-guide"
		specURL = "https://example.com/spec/chinese-wrap"
	)
	md := strings.Join([]string{
		"| 模块 | 说明 | 入口 |",
		"| --- | --- | --- |",
		"| 渲染 | 这是一段用于验证窄屏折行和右边框闭合的中文长句内容 | [在线文档](" + docURL + ") |",
		"| 配置 | 沿用亮暗主题配色与紧凑列宽 | [**规范说明**](" + specURL + ") |",
	}, "\n")

	st := newGridTableTestStyle()

	// Sweep a wide continuous range including ultra-narrow widths and the
	// lipgloss maxTotal..maxTotal+borderW boundary zone.
	for width := 10; width <= 95; width++ {
		out := renderTableGrid(t, md, width, st)
		lines := tableLines(out)
		if len(lines) < 5 {
			t.Fatalf("width=%d produced too few lines (%d):\n%s", width, len(lines), xansi.Strip(out))
		}
		topW := xansi.StringWidth(trimRightPlain(lines[0]))
		if topW > width {
			t.Fatalf("width=%d overflowed viewport width: got %d\n%s", width, topW, xansi.Strip(out))
		}
		for rowIdx, line := range lines {
			plain := trimRightPlain(line)
			if w := xansi.StringWidth(plain); w != topW {
				t.Fatalf("width=%d row=%d ragged width %d != top %d:\n%s", width, rowIdx, w, topW, xansi.Strip(out))
			}
			runes := []rune(plain)
			first, last := runes[0], runes[len(runes)-1]
			switch {
			case rowIdx == 0:
				if first != '┌' || last != '┐' {
					t.Fatalf("width=%d top border not closed: %q", width, plain)
				}
			case rowIdx == len(lines)-1:
				if first != '└' || last != '┘' {
					t.Fatalf("width=%d bottom border not closed: %q", width, plain)
				}
			default:
				validEdge := (first == '│' && last == '│') || (first == '├' && last == '┤')
				if !validEdge {
					t.Fatalf("width=%d row=%d border not closed: %q", width, rowIdx, plain)
				}
			}

			// Verify OSC 8 links never leak onto border glyphs or stay unclosed at EOL.
			if err := checkLineOSC8Boundaries(line); err != nil {
				t.Fatalf("width=%d row=%d OSC 8 violation: %v\nraw=%q", width, rowIdx, err, line)
			}
		}

		// At readable widths, verify wrapped content stays inside the same row
		// cell (exact 4 vertical bars per data line) and link text stays intact.
		if width >= 28 {
			var docText, specText strings.Builder
			for _, line := range lines {
				plain := trimRightPlain(line)
				if strings.HasPrefix(plain, "│") && strings.Count(plain, "│") != 4 {
					t.Fatalf("width=%d expected 4 column dividers on data row: %q", width, plain)
				}
				docText.WriteString(extractOSC8Text(line, docURL))
				specText.WriteString(extractOSC8Text(line, specURL))
			}
			if docText.String() != "在线文档" {
				t.Fatalf("width=%d unexpected doc link text: %q", width, docText.String())
			}
			if specText.String() != "规范说明" {
				t.Fatalf("width=%d unexpected spec link text: %q", width, specText.String())
			}
		}
	}
}

func TestTableGridWrappedCellLinkAndTruncatedHeaderLink(t *testing.T) {
	const (
		hdrURL  = "https://example.com/header-link"
		bodyURL = "https://example.com/wrapped-cell-link"
	)
	md := strings.Join([]string{
		"| 模块 | [超长表头超链接标题说明](" + hdrURL + ") |",
		"| --- | --- |",
		"| 链接 | 前缀 [这是一段在窄表格单元格内必然跨多行折行的中文超链接文本](" + bodyURL + ") 后缀 |",
	}, "\n")

	out := renderTableGrid(t, md, 36, newGridTableTestStyle())
	lines := tableLines(out)

	var bodyLinkLines int
	var gotBody strings.Builder
	for idx, line := range lines {
		if err := checkLineOSC8Boundaries(line); err != nil {
			t.Fatalf("row=%d OSC 8 violation: %v\nraw=%q", idx, err, line)
		}
		if seg := extractOSC8Text(line, bodyURL); seg != "" {
			bodyLinkLines++
			gotBody.WriteString(seg)
		}
	}
	if bodyLinkLines < 2 {
		t.Fatalf("expected wrapped cell link across >= 2 lines, got %d:\n%s", bodyLinkLines, xansi.Strip(out))
	}
	if gotBody.String() != "这是一段在窄表格单元格内必然跨多行折行的中文超链接文本" {
		t.Fatalf("wrapped cell link text mismatch: %q", gotBody.String())
	}
}

func extractOSC8Text(line, wantURL string) string {
	var b strings.Builder
	active := ""
	for i := 0; i < len(line); {
		if line[i] == '\x1b' && i+1 < len(line) {
			switch line[i+1] {
			case '[':
				j := i + 2
				for j < len(line) && (line[j] < 0x40 || line[j] > 0x7e) {
					j++
				}
				if j < len(line) {
					i = j + 1
				} else {
					i = len(line)
				}
				continue
			case ']':
				end := -1
				next := len(line)
				for k := i + 2; k < len(line); k++ {
					if line[k] == '\x07' {
						end = k
						next = k + 1
						break
					}
					if line[k] == '\x1b' && k+1 < len(line) && line[k+1] == '\\' {
						end = k
						next = k + 2
						break
					}
				}
				if end >= 0 {
					payload := line[i+2 : end]
					if strings.HasPrefix(payload, "8;") {
						rest := payload[2:]
						if semi := strings.IndexByte(rest, ';'); semi >= 0 {
							active = rest[semi+1:]
						}
					}
				}
				i = next
				continue
			}
		}
		cluster, _ := xansi.FirstGraphemeCluster(line[i:], xansi.GraphemeWidth)
		if cluster == "" {
			i++
			continue
		}
		if active == wantURL {
			b.WriteString(cluster)
		}
		i += len(cluster)
	}
	return b.String()
}

func checkLineOSC8Boundaries(line string) error {
	active := ""
	for i := 0; i < len(line); {
		if line[i] == '\x1b' && i+1 < len(line) {
			switch line[i+1] {
			case '[':
				j := i + 2
				for j < len(line) && (line[j] < 0x40 || line[j] > 0x7e) {
					j++
				}
				if j < len(line) {
					i = j + 1
				} else {
					i = len(line)
				}
				continue
			case ']':
				end := -1
				next := len(line)
				for k := i + 2; k < len(line); k++ {
					if line[k] == '\x07' {
						end = k
						next = k + 1
						break
					}
					if line[k] == '\x1b' && k+1 < len(line) && line[k+1] == '\\' {
						end = k
						next = k + 2
						break
					}
				}
				if end >= 0 {
					payload := line[i+2 : end]
					if strings.HasPrefix(payload, "8;") {
						rest := payload[2:]
						if semi := strings.IndexByte(rest, ';'); semi >= 0 {
							active = rest[semi+1:]
						}
					}
				}
				i = next
				continue
			}
		}
		cluster, _ := xansi.FirstGraphemeCluster(line[i:], xansi.GraphemeWidth)
		if cluster == "" {
			i++
			continue
		}
		if active != "" {
			switch cluster {
			case "┌", "┐", "└", "┘", "┬", "┴", "├", "┤", "─", "│", "┼":
				return &osc8Err{"border glyph " + cluster + " inherited hyperlink " + active}
			}
		}
		i += len(cluster)
	}
	if active != "" {
		return &osc8Err{"dangling unclosed OSC 8 hyperlink " + active}
	}
	return nil
}

type osc8Err struct{ msg string }

func (e *osc8Err) Error() string { return e.msg }
