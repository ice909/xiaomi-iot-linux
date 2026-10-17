package desktop

import "github.com/egoist/mygo/ui"

// Keep native control behavior (focus, hover, press and disabled) while
// sharing the same restrained palette and spacing across all four pages.
func appTheme(base *ui.Theme) *ui.Theme {
	t := *base
	t.Radius, t.Spacing, t.FontSize = 4, 4, 14
	if t.Dark {
		t.Background, t.Surface = ui.Hex("#18181B"), ui.Hex("#202023")
		t.SurfaceHover, t.SurfacePressed = ui.Hex("#303034"), ui.Hex("#3F3F46")
		t.Text, t.TextMuted, t.Border = ui.Hex("#FAFAFA"), ui.Hex("#A1A1AA"), ui.Hex("#38383E")
		t.Accent, t.AccentHover, t.AccentPressed = ui.Hex("#FAFAFA"), ui.Hex("#E4E4E7"), ui.Hex("#D4D4D8")
		t.AccentText, t.Selection = ui.Hex("#18181B"), ui.Hex("#303034")
		t.Success, t.Warning, t.Danger = ui.Hex("#6EE7B7"), ui.Hex("#FCD34D"), ui.Hex("#FCA5A5")
		t.Focus = ui.Hex("#A1A1AA")
	} else {
		t.Background, t.Surface = ui.Hex("#FFFFFF"), ui.Hex("#FAFAFA")
		t.SurfaceHover, t.SurfacePressed = ui.Hex("#F0F0F2"), ui.Hex("#E4E4E7")
		t.Text, t.TextMuted, t.Border = ui.Hex("#18181B"), ui.Hex("#62626C"), ui.Hex("#E4E4E7")
		t.Accent, t.AccentHover, t.AccentPressed = ui.Hex("#27272A"), ui.Hex("#3F3F46"), ui.Hex("#09090B")
		t.AccentText, t.Selection = ui.Hex("#FFFFFF"), ui.Hex("#EDEDEF")
		t.Success, t.Warning, t.Danger = ui.Hex("#047857"), ui.Hex("#946200"), ui.Hex("#B91C1C")
		t.Focus = ui.Hex("#71717A")
	}
	return &t
}

func section(c *ui.Context, title string, fn func()) {
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		ui.Text(c, title).FontSize(15).Bold()
		fn()
	})
}

func muted(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(13).Wrap().TextColor(c.Theme().TextMuted)
}

func kv(c *ui.Context, k, v string) {
	if v == "" {
		v = "—"
	}
	ui.Row(c).FillWidth().Gap(12).MinHeight(24).Children(func() {
		ui.Text(c, k).Width(88).Shrink(0).TextColor(c.Theme().TextMuted)
		ui.Text(c, v).Grow(1).Wrap().Selectable()
	})
}

func action(c *ui.Context, label string, primary bool) ui.Element {
	if primary {
		return ui.PrimaryButton(c.Key(label), label).Height(32)
	}
	return ui.Button(c.Key(label), label).Height(32)
}

func field(c *ui.Context, label string, fn func()) {
	ui.Row(c).FillWidth().Gap(12).Children(func() {
		ui.Text(c, label).Width(88).Shrink(0)
		ui.Column(c).Grow(1).Children(fn)
	})
}
