package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/mattn/go-runewidth"
)

func (m *AppModel) recalculateTableWidths() {
	if m.width <= 0 {
		return
	}

	// Calculate inner width for safety checks (Margin(2)+Border(2)+Padding(2)=6 offset)
	contentWidth := m.width - 10

	// 1. Queue Table Resizing
	// Use original fixed widths, only shrink Name if terminal is too narrow
	qNameW := 26
	if contentWidth < 105 {
		qNameW = contentWidth - 70
		if qNameW < 10 {
			qNameW = 10
		}
	}
	qCols := []table.Column{
		{Title: "Name", Width: qNameW},
		{Title: fmt.Sprintf("%12s", "Pending"), Width: 12},
		{Title: fmt.Sprintf("%12s", "Consumers"), Width: 12},
		{Title: fmt.Sprintf("%12s", "Enqueued"), Width: 12},
		{Title: fmt.Sprintf("%12s", "Dequeued"), Width: 12},
	}
	if m.viewStats {
		qCols = append(qCols,
			table.Column{Title: "Memory", Width: 30},
			table.Column{Title: "Disk", Width: 15},
		)
	}
	m.queueTable.SetColumns(qCols)

	// 2. Message Table Resizing
	// Use original fixed widths where possible, shrink ID/Correlation proportionally if needed
	mIdW := 46
	mCorrW := 36
	if contentWidth < 164 {
		slack := contentWidth - 82
		if slack < 20 {
			slack = 20
		}
		mIdW = int(float64(slack) * 0.56)
		mCorrW = slack - mIdW
	}

	m.msgTable.SetColumns([]table.Column{
		{Title: "SEQ", Width: 5},
		{Title: "Message ID", Width: mIdW},
		{Title: "Correlation ID", Width: mCorrW},
		{Title: "Persistence", Width: 12},
		{Title: "Priority", Width: 8},
		{Title: "Redelivered", Width: 12},
		{Title: "Timestamp", Width: 30},
		{Title: "Action", Width: 15},
	})

	// 3. Connections Table Resizing
	cNameW := 40
	cAddrW := 30
	if contentWidth < 90 {
		cAddrW = contentWidth - 60
		if cAddrW < 10 {
			cAddrW = 10
		}
		cNameW = contentWidth - cAddrW - 20
		if cNameW < 10 {
			cNameW = 10
		}
	}
	m.connectionsTable.SetColumns([]table.Column{
		{Title: "Name", Width: cNameW},
		{Title: "Remote Address", Width: cAddrW},
		{Title: "Active", Width: 10},
		{Title: "Slow", Width: 10},
	})

	// 4. Consumers Table Resizing
	conPidW := 10
	conAddrW := 25
	conClientW := 40
	conDeqW := 10
	conUptimeW := 15

	if contentWidth < 100 {
		// narrower terminal, shrink ClientID proportionally
		conClientW = contentWidth - 60
		if conClientW < 10 {
			conClientW = 10
		}
		conAddrW = contentWidth - conClientW - 35
		if conAddrW < 10 {
			conAddrW = 10
		}
	}

	m.consumersTable.SetColumns([]table.Column{
		{Title: "PID", Width: conPidW},
		{Title: "Remote Address", Width: conAddrW},
		{Title: "Client ID", Width: conClientW},
		{Title: "Dequeues", Width: conDeqW},
		{Title: "Uptime", Width: conUptimeW},
	})
}

// wrapText manually wraps long strings by breaking words safely using runes if they exceed the width
func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	var result strings.Builder
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}
		if len(line) == 0 {
			continue
		}

		currentWidth := 0
		var currentLine strings.Builder

		for _, r := range line {
			rw := runewidth.RuneWidth(r)
			if currentWidth+rw > width {
				result.WriteString(currentLine.String())
				result.WriteString("\n")
				currentLine.Reset()
				currentWidth = 0
			}
			currentLine.WriteRune(r)
			currentWidth += rw
		}
		if currentLine.Len() > 0 {
			result.WriteString(currentLine.String())
		}
	}
	return result.String()
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatWithCommas(n int64) string {
	if n < 0 {
		return "-" + formatWithCommas(-n)
	}
	in := strconv.FormatInt(n, 10)
	numOfDigits := len(in)
	if numOfDigits <= 3 {
		return in
	}
	var b strings.Builder
	for i, c := range in {
		if i > 0 && (numOfDigits-i)%3 == 0 {
			b.WriteRune(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func makeUsageBar(percent float64, bytes int64, isASCII bool) string {
	if percent <= 0 && bytes == 0 {
		return "  -"
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := int(percent) / 10
	empty := 10 - filled

	var bar string
	if isASCII {
		bar = strings.Repeat("#", filled) + strings.Repeat("-", empty)
	} else {
		bar = strings.Repeat("█", filled) + strings.Repeat("░", empty)
	}

	var sizeStr string
	if bytes > 1024*1024*1024 {
		sizeStr = fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
	} else if bytes > 1024*1024 {
		sizeStr = fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	} else if bytes > 1024 {
		sizeStr = fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	} else {
		sizeStr = fmt.Sprintf("%d B", bytes)
	}

	if percent > 0 && percent < 1 {
		return fmt.Sprintf("[%s] %4.2f%% (%s)", bar, percent, sizeStr)
	}
	return fmt.Sprintf("[%s] %3.0f%% (%s)", bar, percent, sizeStr)
}
