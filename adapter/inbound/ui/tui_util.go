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

	// Box inner usable width: Width(m.width-6) with Border(2) and Padding(2) gives m.width - 10
	innerUsableWidth := m.width - 10
	if innerUsableWidth < 40 {
		innerUsableWidth = 40
	}

	// 1. Queue Table Resizing
	// Max cap at 46 so UUID queue names (36 chars) fit completely without creating huge empty gaps in fullscreen
	if !m.viewStats {
		// 5 columns: Name + 4 fixed metrics (12 chars each = 48) + 5*2 cell padding (10) = 58 fixed overhead
		qNameW := innerUsableWidth - 58 - 2
		if qNameW > 46 {
			qNameW = 46
		}
		if qNameW < 10 {
			qNameW = 10
		}
		m.queueTable.SetColumns([]table.Column{
			{Title: "Name", Width: qNameW},
			{Title: fmt.Sprintf("%12s", "Pending"), Width: 12},
			{Title: fmt.Sprintf("%12s", "Consumers"), Width: 12},
			{Title: fmt.Sprintf("%12s", "Enqueued"), Width: 12},
			{Title: fmt.Sprintf("%12s", "Dequeued"), Width: 12},
		})
	} else {
		// 7 columns: Name + 4 metrics (48) + Memory (26) + Disk (12) = 86 + 7*2 padding (14) = 100 fixed overhead
		qNameW := innerUsableWidth - 100 - 2
		if qNameW > 46 {
			qNameW = 46
		}
		if qNameW < 10 {
			qNameW = 10
		}
		m.queueTable.SetColumns([]table.Column{
			{Title: "Name", Width: qNameW},
			{Title: fmt.Sprintf("%12s", "Pending"), Width: 12},
			{Title: fmt.Sprintf("%12s", "Consumers"), Width: 12},
			{Title: fmt.Sprintf("%12s", "Enqueued"), Width: 12},
			{Title: fmt.Sprintf("%12s", "Dequeued"), Width: 12},
			{Title: "Memory", Width: 26},
			{Title: "Disk", Width: 12},
		})
	}

	// 2. Message Table Resizing (8 columns)
	// 8*2 padding (16) + SEQ(5) + Persistence(12) + Priority(8) + Redelivered(12) + Timestamp(24) + Action(10) = 87 fixed overhead
	msgSlack := innerUsableWidth - 87 - 2
	if msgSlack < 20 {
		msgSlack = 20
	}
	mIdW := int(float64(msgSlack) * 0.55)
	mCorrW := msgSlack - mIdW
	if mIdW > 46 {
		mIdW = 46
	}
	if mCorrW > 36 {
		mCorrW = 36
	}
	if mIdW < 10 {
		mIdW = 10
	}
	if mCorrW < 10 {
		mCorrW = 10
	}

	m.msgTable.SetColumns([]table.Column{
		{Title: "SEQ", Width: 5},
		{Title: "Message ID", Width: mIdW},
		{Title: "Correlation ID", Width: mCorrW},
		{Title: "Persistence", Width: 12},
		{Title: "Priority", Width: 8},
		{Title: "Redelivered", Width: 12},
		{Title: "Timestamp", Width: 24},
		{Title: "Action", Width: 10},
	})

	// 3. Connections Table Resizing (4 columns)
	// 4*2 padding (8) + Active(10) + Slow(10) = 28 fixed overhead
	connSlack := innerUsableWidth - 28 - 2
	if connSlack < 20 {
		connSlack = 20
	}
	cNameW := int(float64(connSlack) * 0.55)
	cAddrW := connSlack - cNameW
	if cNameW > 46 {
		cNameW = 46
	}
	if cAddrW > 32 {
		cAddrW = 32
	}
	if cNameW < 10 {
		cNameW = 10
	}
	if cAddrW < 10 {
		cAddrW = 10
	}

	m.connectionsTable.SetColumns([]table.Column{
		{Title: "Name", Width: cNameW},
		{Title: "Remote Address", Width: cAddrW},
		{Title: "Active", Width: 10},
		{Title: "Slow", Width: 10},
	})

	// 4. Consumers Table Resizing (5 columns)
	// 5*2 padding (10) + PID(10) + Remote Address(22) + Dequeues(10) + Uptime(12) = 64 fixed overhead
	conClientW := innerUsableWidth - 64 - 2
	if conClientW > 46 {
		conClientW = 46
	}
	if conClientW < 10 {
		conClientW = 10
	}

	m.consumersTable.SetColumns([]table.Column{
		{Title: "PID", Width: 10},
		{Title: "Remote Address", Width: 22},
		{Title: "Client ID", Width: conClientW},
		{Title: "Dequeues", Width: 10},
		{Title: "Uptime", Width: 12},
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

func drawProgressBar(percent float64, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := int((percent / 100.0) * float64(width))
	empty := width - filled

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	return fmt.Sprintf("[%s]", bar)
}

func truncateStr(s string, max int) string {
	if len(s) > max {
		return s[:max-2] + ".."
	}
	return s
}

func stripProtocol(s string) string {
	if strings.HasPrefix(s, "tcp://") {
		return s[6:]
	}
	if strings.HasPrefix(s, "ssl://") {
		return s[6:]
	}
	return s
}
