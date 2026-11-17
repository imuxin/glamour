package ansi_test

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/glamour/v2"
	glamouransi "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

func boolPtr(v bool) *bool       { return &v }
func stringPtr(v string) *string { return &v }
func uintPtr(v uint) *uint       { return &v }

func newHideLinkTestStyle() glamouransi.StyleConfig {
	st := styles.DarkStyleConfig
	st.Document.Margin = uintPtr(0)
	st.Document.BlockPrefix = ""
	st.Document.BlockSuffix = ""
	st.Link.Color = stringPtr("#7AA2F7")
	st.Link.Underline = boolPtr(true)
	st.LinkText.Color = stringPtr("#7AA2F7")
	st.LinkText.Underline = boolPtr(true)
	st.LinkText.Bold = nil
	st.Code.Prefix = ""
	st.Code.Suffix = ""
	st.Code.Color = stringPtr("#73DACA")
	st.Code.BackgroundColor = stringPtr("#332B45")
	return st
}

func renderWithHideLinkURL(t *testing.T, mdText string, width int) string {
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
			Styles:        newHideLinkTestStyle(),
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

func hasSGRAttr(s, target string) bool {
	for i := 0; i < len(s); {
		rel := strings.Index(s[i:], "\x1b[")
		if rel < 0 {
			return false
		}
		pos := i + rel
		j := pos + 2
		for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == ';' || s[j] == ':') {
			j++
		}
		if j < len(s) && s[j] == 'm' {
			parts := strings.Split(s[pos+2:j], ";")
			for k := 0; k < len(parts); k++ {
				switch parts[k] {
				case "38", "48", "58":
					if k+1 < len(parts) {
						if parts[k+1] == "2" {
							k += 4
						} else if parts[k+1] == "5" {
							k += 2
						}
					}
				default:
					if parts[k] == target {
						return true
					}
				}
			}
		}
		i = j + 1
	}
	return false
}

func TestHideLinkURLProseAndFormatting(t *testing.T) {
	// 1. Ordinary link hides URL in visible text while preserving full OSC 8 URL.
	url := "https://example.com/docs/guide?tab=1&mode=full#intro"
	got := renderWithHideLinkURL(t, "请阅读[开发指南]("+url+")以了解详情。", 80)
	plain := strings.TrimSpace(xansi.Strip(got))
	if plain != "请阅读开发指南以了解详情。" {
		t.Fatalf("unexpected visible text: %q", plain)
	}
	if !strings.Contains(got, ";"+url+"\x07") {
		t.Fatalf("expected OSC 8 hyperlink with full URL %q in %q", url, got)
	}

	// 2. Regular link is not bold when LinkText.Bold == nil, while [**粗体**](url) and **[粗体](url)** are bold.
	regularOut := renderWithHideLinkURL(t, "[普通](https://example.com/1)", 80)
	innerBoldOut := renderWithHideLinkURL(t, "[**内部粗体**](https://example.com/2)", 80)
	outerBoldOut := renderWithHideLinkURL(t, "**[外部粗体](https://example.com/3)**", 80)
	outerItalicOut := renderWithHideLinkURL(t, "*[外部斜体](https://example.com/4)*", 80)

	if hasSGRAttr(regularOut, "1") {
		t.Fatalf("regular link must not emit bold SGR when LinkText.Bold == nil: %q", regularOut)
	}
	if !hasSGRAttr(regularOut, "4") {
		t.Fatalf("regular link must emit underline SGR: %q", regularOut)
	}
	for name, sample := range map[string]string{"innerBold": innerBoldOut, "outerBold": outerBoldOut} {
		if !hasSGRAttr(sample, "1") {
			t.Fatalf("%s must retain bold SGR: %q", name, sample)
		}
		if !hasSGRAttr(sample, "4") {
			t.Fatalf("%s must retain underline SGR: %q", name, sample)
		}
	}
	if !hasSGRAttr(outerItalicOut, "3") || !hasSGRAttr(outerItalicOut, "4") {
		t.Fatalf("outer italic link must retain italic + underline SGR: %q", outerItalicOut)
	}

	// 3. Inline code inside link keeps code styling + underline + literal backslashes + OSC 8.
	codeOut := renderWithHideLinkURL(t, "执行 [`go test \\*`](https://example.com/cmd) 验证", 80)
	codePlain := strings.TrimSpace(xansi.Strip(codeOut))
	if codePlain != `执行 go test \* 验证` {
		t.Fatalf("inline code link visible text corrupted: %q", codePlain)
	}
	if !strings.Contains(codeOut, ";https://example.com/cmd\x07") {
		t.Fatalf("inline code link missing OSC 8 URL: %q", codeOut)
	}
	if !strings.Contains(codeOut, "48;2;51;43;69") {
		t.Fatalf("inline code link must retain code background color: %q", codeOut)
	}
	if !hasSGRAttr(codeOut, "4") {
		t.Fatalf("inline code link must inherit link underline: %q", codeOut)
	}

	// 4. Bare URLs, angle autolinks, emails, images, and empty-text fallback.
	miscMD := strings.Join([]string{
		"裸链 https://example.com/bare",
		"尖括号 <https://example.com/angle>",
		"邮箱 support@example.com",
		"图片 ![架构](https://example.com/arch.png)",
		"空描述 [](https://example.com/empty)",
		"空白描述 [   ](https://example.com/spaces)",
		"纯锚点 [](#section-1)",
	}, "\n\n")
	miscOut := renderWithHideLinkURL(t, miscMD, 120)
	miscPlain := xansi.Strip(miscOut)
	for _, want := range []string{
		"https://example.com/bare",
		"https://example.com/angle",
		"support@example.com",
		"Image: 架构 → https://example.com/arch.png",
		"https://example.com/empty",
		"https://example.com/spaces",
		"#section-1",
	} {
		if !strings.Contains(miscPlain, want) {
			t.Fatalf("expected %q in rendered plain text:\n%s", want, miscPlain)
		}
	}
	if strings.Contains(miscPlain, "mailto:") {
		t.Fatalf("email visible text must not expose mailto: prefix:\n%s", miscPlain)
	}
	if !strings.Contains(miscOut, ";mailto:support@example.com\x07") {
		t.Fatalf("email must carry mailto: OSC 8 hyperlink: %q", miscOut)
	}
}

func TestHideLinkURLTables(t *testing.T) {
	// 1. Table containing only regular links (including bold/code/empty fallback) produces no [1] indices or footer.
	pureTable := strings.Join([]string{
		"| 名称 | 入口 |",
		"| --- | --- |",
		"| 说明 | [官方文档](https://example.com/docs/very/long/path) |",
		"| 重点 | [**核心规范**](https://example.com/spec) |",
		"| 命令 | [`philo`](https://example.com/cli) |",
		"| 兜底 | [](https://example.com/fallback) |",
	}, "\n")
	pureOut := renderWithHideLinkURL(t, pureTable, 100)
	purePlain := xansi.Strip(pureOut)

	for _, want := range []string{"官方文档", "核心规范", "philo", "https://example.com/fallback"} {
		if !strings.Contains(purePlain, want) {
			t.Fatalf("missing %q in table plain output:\n%s", want, purePlain)
		}
	}
	for _, forbidden := range []string{
		"https://example.com/docs/very/long/path",
		"https://example.com/spec",
		"https://example.com/cli",
		"[1]",
	} {
		if strings.Contains(purePlain, forbidden) {
			t.Fatalf("unexpected %q in table plain output:\n%s", forbidden, purePlain)
		}
	}
	for _, wantOSC := range []string{
		";https://example.com/docs/very/long/path\x07",
		";https://example.com/spec\x07",
		";https://example.com/cli\x07",
		";https://example.com/fallback\x07",
	} {
		if !strings.Contains(pureOut, wantOSC) {
			t.Fatalf("missing OSC 8 %q in table output:\n%q", wantOSC, pureOut)
		}
	}

	// 2. Mixed table: regular links render inline without [n], while bare URLs and images keep existing [1] numbering & footer.
	mixedTable := strings.Join([]string{
		"| 普通 | 裸链 | 图片 |",
		"| --- | --- | --- |",
		"| [指南](https://example.com/guide) | https://github.com/org/repo | ![图](https://example.com/img.png) |",
	}, "\n")
	mixedOut := renderWithHideLinkURL(t, mixedTable, 100)
	mixedPlain := xansi.Strip(mixedOut)

	if strings.Contains(mixedPlain, "指南[") || strings.Contains(mixedPlain, "https://example.com/guide") {
		t.Fatalf("regular table link must not be numbered or printed in footer:\n%s", mixedPlain)
	}
	if !strings.Contains(mixedPlain, "[1]: ") || !strings.Contains(mixedPlain, "https://github.com/org/repo") {
		t.Fatalf("bare URL in table must keep existing [1] footnote behavior:\n%s", mixedPlain)
	}
	if !strings.Contains(mixedPlain, "Image: 图[1]") || !strings.Contains(mixedPlain, "https://example.com/img.png") {
		t.Fatalf("image in table must keep existing [1] footnote behavior:\n%s", mixedPlain)
	}

	// 3. Verify top-level glamour.WithHideLinkURL option works end-to-end.
	tr, err := glamour.NewTermRenderer(
		glamour.WithStyles(newHideLinkTestStyle()),
		glamour.WithWordWrap(80),
		glamour.WithHyperlinkMode(glamouransi.HyperlinkModeInline),
	)
	if err != nil {
		t.Fatalf("NewTermRenderer: %v", err)
	}
	topOut, err := tr.Render("[点击跳转](https://example.com/top)")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.TrimSpace(xansi.Strip(topOut)) != "点击跳转" {
		t.Fatalf("unexpected WithHideLinkURL output: %q", xansi.Strip(topOut))
	}
}
