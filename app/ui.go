package catsapp

import (
	"github.com/rohanthewiz/grmob/components"
	"github.com/rohanthewiz/grmob/core"
)

// The shared UI vocabulary: the pieces every screen assembles from. grmob
// has no Material layer, so the app bar, the list tile and the dialog are
// built here out of core containers and the components library, once, rather
// than at forty call sites. church_mobile's ui.go is the model and most of
// these are its siblings.

// screenHeader is this app's AppBar: a title row with an optional back
// affordance and trailing actions. The back arrow is drawn only when there is
// something to pop (core.CanPop); on the tab shell there never is.
func screenHeader(ctx *core.Context, title string, actions ...core.View) core.View {
	theme := ctx.Theme()
	items := []core.PropsAndChildren{
		core.BackgroundColor(theme.Colors.Background),
		core.AlignItemsProp(core.AlignItemsCenter),
		core.Gap(8),
		core.PaddingHorizontal(12),
		core.PaddingVertical(10),
	}
	if core.CanPop(ctx) {
		items = append(items, components.Button{
			Label:              "‹",
			Emphasis:           components.EmphasisGhost,
			OnTap:              func() { core.Pop(ctx) },
			AccessibilityLabel: "Back",
			Style: []core.StyleProp{
				core.FontSize(26),
				core.PaddingHorizontal(6),
				core.PaddingVertical(0),
			},
		})
	}
	items = append(items,
		// FlexGrow on the title pins the actions to the trailing edge without
		// a justify rule, which behaves identically on all four renderers.
		core.Box(
			core.FlexGrow(1),
			core.Text(title,
				core.FontSize(20),
				core.FontWeight(core.Bold),
				core.TextColor(theme.Colors.TextPrimary),
			),
		),
	)
	for _, action := range actions {
		items = append(items, action)
	}
	return core.Column(
		core.Gap(0),
		core.PaddingHorizontal(0),
		core.PaddingVertical(0),
		core.Row(items...),
		components.Separator{},
	)
}

// headerAction is a compact glyph button for a header's trailing edge. Every
// one carries an AccessibilityLabel, because a screen reader announcing "↻"
// is announcing nothing.
func headerAction(glyph, label string, onTap func()) core.View {
	return components.Button{
		Label:              glyph,
		Emphasis:           components.EmphasisGhost,
		OnTap:              onTap,
		AccessibilityLabel: label,
		Style: []core.StyleProp{
			core.FontSize(18),
			core.PaddingHorizontal(10),
			core.PaddingVertical(4),
		},
	}
}

// sectionHeader titles a run of rows: the roster's state groups, the More
// screen's blocks.
func sectionHeader(ctx *core.Context, title string) core.View {
	return core.Row(
		core.PaddingHorizontal(16),
		core.PaddingTop(18),
		core.PaddingVertical(4),
		core.Text(title,
			core.FontSize(13),
			core.FontWeight(core.Bold),
			core.TextColor(ctx.Theme().Colors.TextSecondary),
		),
	)
}

// contentRow is the app's ListTile: a leading glyph, a title, a quiet
// subtitle, and an optional trailing view, all tappable.
func contentRow(ctx *core.Context, glyph, title, subtitle string, onTap func(), trailing core.View) core.View {
	theme := ctx.Theme()
	var leading core.View
	if glyph != "" {
		leading = core.Text(glyph,
			core.FontSize(18),
			// A fixed width keeps adjacent titles aligned when the glyphs
			// differ in advance width, which emoji do wildly.
			core.Width("28px"),
			core.TextColor(theme.Colors.Primary),
		)
	}
	return components.ListRow{
		Leading:  leading,
		Title:    title,
		Subtitle: subtitle,
		Trailing: trailing,
		OnTap:    onTap,
	}
}

// mutedText is the caption style: the subtitle under a row, the explanatory
// line under a heading.
func mutedText(ctx *core.Context, s string, extra ...core.StyleProp) core.View {
	props := append([]core.StyleProp{
		core.FontSize(13),
		core.TextColor(ctx.Theme().Colors.TextSecondary),
	}, extra...)
	return core.Text(s, props...)
}

// monoText is a line in the terminal's own idiom: a pane handle, a cwd, a
// fingerprint. grmob has no font family, so this is a size and a colour, not
// a fixed pitch; only TextGrid gets a monospace face.
func monoText(ctx *core.Context, s string) core.View {
	return core.Text(s, core.FontSize(13), core.TextColor(ctx.Theme().Colors.TextPrimary))
}

// emptyNote is the consistent "there is nothing here" placeholder.
func emptyNote(ctx *core.Context, text string) core.View {
	return core.Column(
		// Width 100% is what centres this on the natives: a Column there hugs
		// its widest child, so without it the column sits at the leading
		// edge. The web targets already fill the line.
		core.Width("100%"),
		core.AlignItemsProp(core.AlignItemsCenter),
		core.Padding(24),
		core.Text(text,
			core.Align(core.AlignCenter),
			core.TextColor(ctx.Theme().Colors.TextSecondary),
		),
	)
}

// noticeStrip is the inline banner: full width, quiet, with an optional
// action. The connection banner and the stale-data hint use it.
func noticeStrip(ctx *core.Context, text string, actionLabel string, action func()) core.View {
	theme := ctx.Theme()
	items := []core.PropsAndChildren{
		core.BackgroundColor(theme.Colors.Surface),
		core.AlignItemsProp(core.AlignItemsCenter),
		core.PaddingHorizontal(16),
		core.PaddingVertical(6),
		core.Gap(8),
		core.Box(core.FlexGrow(1), mutedText(ctx, text)),
	}
	if action != nil {
		items = append(items, components.Button{
			Label:    actionLabel,
			Emphasis: components.EmphasisGhost,
			OnTap:    action,
			Style:    []core.StyleProp{core.FontSize(13)},
		})
	}
	return core.Row(items...)
}

// confirmDialog is the app's AlertDialog: a modal with a message and a
// cancel/confirm pair.
//
// core.Modal hides rather than unmounts (its content renders every pass
// regardless of Visible), which is why visible is a plain bool the caller
// holds in state and why a dialog sits at a fixed position in its screen's
// tree rather than being appended conditionally.
func confirmDialog(ctx *core.Context, visible bool, title, body, confirmLabel string, onConfirm, onCancel func()) core.View {
	theme := ctx.Theme()
	return core.Modal(
		core.Visible(visible),
		core.OnDismiss(onCancel),
		core.ModalContent(core.Column(
			core.Gap(12),
			core.Padding(4),
			core.MaxWidth("420px"),
			core.BackgroundColor(theme.Colors.Surface),
			core.Text(title, core.FontSize(18), core.FontWeight(core.Bold),
				core.TextColor(theme.Colors.TextPrimary)),
			core.If(body != "", core.Text(body, core.TextColor(theme.Colors.TextPrimary))),
			core.Row(
				core.Justify(core.JustifyEnd),
				core.Gap(8),
				core.PaddingHorizontal(0),
				components.Button{Label: "Cancel", Emphasis: components.EmphasisGhost, OnTap: onCancel},
				components.Button{Label: confirmLabel, OnTap: onConfirm},
			),
		)),
	)
}

// toast reports a transient result through the platform's own overlay.
// Callable from any goroutine, which is what makes it usable from the
// completion of a command.
func toast(message string) { core.ShowToast(message) }

// stateBadge is the coloured pill that names an agent's state. The colours
// are the theme's status roles, so "blocked" is the same ink everywhere the
// app says it.
func stateBadge(ctx *core.Context, state string, seen bool) core.View {
	label, variant := stateLabel(state, seen)
	return components.Badge{Text: label, Variant: variant}
}

// scrollColumn is a screen body: a header, then everything else in one
// scroll region that fills the leftover height.
func scrollColumn(ctx *core.Context, header core.View, items ...core.PropsAndChildren) core.View {
	body := append([]core.PropsAndChildren{
		core.Gap(0),
		core.PaddingHorizontal(0),
		core.PaddingVertical(0),
	}, items...)
	return core.Column(
		core.Gap(0),
		core.PaddingHorizontal(0),
		core.PaddingVertical(0),
		core.FlexGrow(1),
		header,
		core.Scroll(core.FlexGrow(1), core.Column(body...)),
	)
}
