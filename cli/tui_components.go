package cli

import (
	"fmt"
	"strings"
)

func renderLifecycleRail(theme tuiTheme, phases []string, current int) string {
	if len(phases) == 0 {
		return ""
	}

	segments := make([]string, 0, len(phases)*2-1)
	for i, phase := range phases {
		var label string
		switch {
		case i < current:
			label = theme.railDone.Render("[" + phase + "]")
		case i == current:
			label = theme.railCurrent.Render("[" + phase + "]")
		default:
			label = theme.railPending.Render("[" + phase + "]")
		}
		segments = append(segments, label)
		if i < len(phases)-1 {
			connector := theme.railPending.Render("->")
			if i < current {
				connector = theme.railDone.Render("->")
			}
			segments = append(segments, connector)
		}
	}

	return strings.Join(segments, " ")
}

func renderActionCard(theme tuiTheme, title, why, action string, width int) string {
	if width < 20 {
		width = 20
	}
	body := strings.Builder{}
	body.WriteString(theme.subtitle.Render(title))
	body.WriteString("\n")
	body.WriteString(theme.muted.Render("Why: "))
	body.WriteString(theme.text.Render(why))
	body.WriteString("\n")
	body.WriteString(theme.info.Render("Next: "))
	body.WriteString(theme.highlight.Render(action))
	return theme.panel.Width(width).Render(body.String())
}

func renderSelectableList(theme tuiTheme, title string, items []string, selected int, width, height int) string {
	if width < 20 {
		width = 20
	}
	if height < 6 {
		height = 6
	}
	maxRows := height - 2
	if maxRows < 1 {
		maxRows = 1
	}

	start := 0
	if selected >= maxRows {
		start = selected - maxRows + 1
	}
	end := start + maxRows
	if end > len(items) {
		end = len(items)
	}

	lines := make([]string, 0, maxRows+1)
	lines = append(lines, theme.subtitle.Render(title))
	for i := start; i < end; i++ {
		prefix := "  "
		line := items[i]
		if i == selected {
			prefix = "> "
			line = theme.highlight.Render(truncateRunes(line, width-4))
		} else {
			line = theme.text.Render(truncateRunes(line, width-4))
		}
		lines = append(lines, prefix+line)
	}
	return theme.panel.Width(width).Render(strings.Join(lines, "\n"))
}

func truncateRunes(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	if limit <= 3 {
		return string(r[:limit])
	}
	return fmt.Sprintf("%s...", string(r[:limit-3]))
}

func panelHeights(total int) (int, int) {
	if total < 10 {
		return total / 2, total - total/2
	}
	top := int(float64(total) * 0.35)
	if top < 5 {
		top = 5
	}
	bottom := total - top
	if bottom < 5 {
		bottom = 5
		top = total - bottom
	}
	return top, bottom
}
