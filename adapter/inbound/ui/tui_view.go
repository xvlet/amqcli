package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func (m *AppModel) View() string {
	var headerOk string
	brokerInfoStr := ""
	sysUsageStr := ""
	// Dynamic colors based on profile
	isHighColor := (lipgloss.ColorProfile() == termenv.ANSI256 || lipgloss.ColorProfile() == termenv.TrueColor)

	cConnected := "10"
	cDisconnected := "1"
	cYellow := "3"
	cSecondary := "7"
	cCyan := "14"
	cDim := "8"

	if isHighColor {
		cConnected = "46"
		cDisconnected = "9"
		cYellow = "226"
		cSecondary = "244"
		cCyan = "51"
		cDim = "240"
	}

	dimPipe := lipgloss.NewStyle().Foreground(lipgloss.Color(cDim)).Render(" | ")

	if m.brokerInfo != "" {
		brokerInfoStr = fmt.Sprintf(" [%s]", m.brokerInfo)

		if m.viewStats {
			cyanStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cCyan)).Render

			storeStr := fmt.Sprintf("Store %d%%", m.brokerStats.StorePercentUsage)
			if m.brokerStats.StoreLimit > 0 {
				var exactStoreUsed int64
				for _, q := range m.queues {
					exactStoreUsed += q.StoreMessageSize
				}
				actualPercent := float64(exactStoreUsed) / float64(m.brokerStats.StoreLimit) * 100.0
				storeStr = fmt.Sprintf("Store %.2f%% (%s / %s)", actualPercent, formatBytes(exactStoreUsed), formatBytes(m.brokerStats.StoreLimit))
			}

			sysUsageStr = fmt.Sprintf("   %s%s%s%s%s%s%s",
				cyanStyle(fmt.Sprintf("System Usage: CPU %.1f%%", m.brokerStats.CPUUsage)),
				dimPipe,
				cyanStyle(fmt.Sprintf("Memory %d%%", m.brokerStats.MemoryPercentUsage)),
				dimPipe,
				cyanStyle(storeStr),
				dimPipe,
				cyanStyle(fmt.Sprintf("Temp %d%%", m.brokerStats.TempPercentUsage)))
		}
	}

	if m.err != nil {
		headerOk = lipgloss.NewStyle().MarginLeft(2).Bold(true).Foreground(lipgloss.Color(cDisconnected)).Render(fmt.Sprintf("%s Disconnected", m.host))
	} else {
		connPart := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cConnected)).Render(fmt.Sprintf("%s Connected%s", m.host, brokerInfoStr))
		headerOk = lipgloss.NewStyle().MarginLeft(2).Render(connPart + sysUsageStr)
	}

	// Determine universal box scaling widths and footer alignment
	contentW := m.width - 6
	if contentW < 20 {
		contentW = 20
	}

	// Create cohesive Box Footer with Left & Right text perfectly spanning box
	y := lipgloss.NewStyle().Foreground(lipgloss.Color(cYellow)).Render
	g := lipgloss.NewStyle().Foreground(lipgloss.Color(cSecondary)).Render

	timeStr := m.lastUpdated.Format("2006-01-02 15:04:05")
	rightText := g(fmt.Sprintf("Last Updated: %s", timeStr))
	leftText := fmt.Sprintf("%s%s", y("q"), g(" : Quit"))

	footerW := contentW + 2 // Box inner content (contentW) + 2 horizontal borders
	spaceW := footerW - lipgloss.Width(leftText)
	if spaceW < 0 {
		spaceW = 0
	}
	footerOk := lipgloss.NewStyle().MarginLeft(2).Render(lipgloss.JoinHorizontal(lipgloss.Top, leftText, lipgloss.NewStyle().Width(spaceW).Align(lipgloss.Right).Render(rightText)))

	if m.currentState == stateList {
		var subHeader string
		if m.err != nil {
			subHeader = lipgloss.NewStyle().MarginLeft(2).Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("Error: %v", m.err))
		} else {
			subHeader = lipgloss.NewStyle().MarginLeft(2).Foreground(lipgloss.Color(cSecondary)).Render(fmt.Sprintf("Command: [%s]reate%s[%s]end to%s[%s]urge%s[%s]elete%s<%s> Browse%s<%s>nfo%sCo<%s>nections",
				y("C"), dimPipe, y("S"), dimPipe, y("P"), dimPipe, y("D"), dimPipe, y("Enter"), dimPipe, y("I"), dimPipe, y("n")))
		}

		// Use global contentW dimension
		tableBox := lipgloss.NewStyle().MarginLeft(2).Border(asciiBorder).BorderForeground(lipgloss.Color("8")).Padding(0, 1).Width(contentW).Render(m.queueTable.View())

		return lipgloss.JoinVertical(lipgloss.Left, headerOk, subHeader, tableBox, footerOk)
	}

	if m.currentState == stateMessageList {
		var sub string
		if m.err != nil {
			sub = lipgloss.NewStyle().MarginLeft(2).Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("Error: %v", m.err))
		} else {
			pending := int64(0)
			for _, q := range m.queues {
				if q.Name == m.selectedQueue {
					pending = q.Pending
					break
				}
			}
			searchHint := fmt.Sprintf("[%s] Search", y("F3|Ctrl+F"))
			if m.isFiltered && m.filterKeyword != "" {
				searchHint += lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Render(fmt.Sprintf(" (%s)", m.filterKeyword))
			}
			sub = lipgloss.NewStyle().MarginLeft(2).Render(fmt.Sprintf("Browsing: %s (%d/%d) | Command: <%s> Back | <%s> Detail | <%s> Select | %s | [%s] Delete | [%s] Delete By Time", m.selectedQueue, len(m.messages), pending, y("Esc"), y("Enter"), y("Space"), searchHint, y("d"), y("p")))
		}

		// Use global contentW dimension
		tableBox := lipgloss.NewStyle().MarginLeft(2).Border(asciiBorder).BorderForeground(lipgloss.Color("8")).Padding(0, 1).Width(contentW).Render(m.msgTable.View())

		return lipgloss.JoinVertical(lipgloss.Left, headerOk, sub, tableBox, footerOk)
	}

	if m.currentState == stateMessageDetail && m.selectedMessage != nil {
		var sub string
		if m.err != nil {
			sub = lipgloss.NewStyle().MarginLeft(2).Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("Error: %v", m.err))
		} else {
			sub = lipgloss.NewStyle().MarginLeft(2).Render(fmt.Sprintf("Browsing: %s | Command: <%s> Back | [%s]elete | [%s]etry | [%s]ove", m.selectedQueue, y("Esc"), y("D"), y("R"), y("M")))
		}

		// Fixed-width bracketed label for uniform alignment, rendered in gray
		// ' Correlation ID ' = 16 runes → const w = 16 so all labels align
		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
		lbl := func(s string) string {
			const w = 16
			padded := s
			for len([]rune(padded)) < w {
				padded += " "
			}
			return labelStyle.Render("[" + padded + "]")
		}

		var b strings.Builder
		fmt.Fprintf(&b, "%s queue://%s\n", lbl(" Destination "), m.selectedQueue)
		fmt.Fprintf(&b, "%s %s\n", lbl(" Message ID "), m.selectedMessage.MessageID)
		fmt.Fprintf(&b, "%s %s\n", lbl(" Correlation ID "), m.selectedMessage.CorrelationID)
		fmt.Fprintf(&b, "%s %s\n", lbl(" Timestamp "), m.selectedMessage.Timestamp.String())
		fmt.Fprintf(&b, "\n%s\n", lbl(" Properties "))

		keys := make([]string, 0, len(m.selectedMessage.Properties))
		for k := range m.selectedMessage.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			v := m.selectedMessage.Properties[k]
			fmt.Fprintf(&b, "%s: %s\n", k, v)
		}
		fmt.Fprintf(&b, "\n%s\n", lbl(" Payload "))
		fmt.Fprintln(&b, m.selectedMessage.Body)

		// Use global contentW dimension for the box boundary
		innerWidth := contentW - 4 // Remove padding offsets for string wrapper
		if innerWidth < 20 {
			innerWidth = 20
		}

		wrappedText := wrapText(b.String(), innerWidth)
		m.viewport.SetContent(wrappedText)

		detailBox := lipgloss.NewStyle().MarginLeft(2).Padding(1, 2).Border(asciiBorder).BorderForeground(lipgloss.Color("8")).Width(contentW).Render(m.viewport.View())

		return lipgloss.JoinVertical(lipgloss.Left, headerOk, sub, detailBox, footerOk)
	}

	if m.currentState == stateQueueInfo {
		var sub string
		if m.err != nil {
			sub = lipgloss.NewStyle().MarginLeft(2).Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("Error: %v", m.err))
		} else {
			sub = lipgloss.NewStyle().MarginLeft(2).Render(fmt.Sprintf("Queue Info: %s | Command: <%s> Back", m.selectedQueue, y("Esc")))
		}

		if m.selectedQueueDetail == nil {
			var b strings.Builder
			b.WriteString("\n  Loading queue details...")
			infoBox := lipgloss.NewStyle().MarginLeft(2).Padding(1, 2).Border(asciiBorder).BorderForeground(lipgloss.Color("8")).Width(contentW).Render(b.String())
			return lipgloss.JoinVertical(lipgloss.Left, headerOk, sub, infoBox, footerOk)
		}

		qd := m.selectedQueueDetail
		// Label helper for alignment
		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
		lbl := func(s string) string {
			const w = 20
			padded := s
			for len([]rune(padded)) < w {
				padded += " "
			}
			return labelStyle.Render("[" + padded + "]")
		}

		// Indentation and Style definitions
		indent := "   "
		var b strings.Builder
		fmt.Fprintf(&b, "\n%s%s\n", indent, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")).Render(" [ Queue Statistics ] "))

		// Stat rows
		fmt.Fprintf(&b, "%s  %s %-20s    %s %d\n", indent, lbl(" Name "), qd.Name, lbl(" Queue Size "), qd.QueueSize)
		fmt.Fprintf(&b, "%s  %s %-20d    %s %d\n", indent, lbl(" Consumer Count "), qd.ConsumerCount, lbl(" In-Flight "), qd.InFlightCount)
		fmt.Fprintf(&b, "%s  %s %-20d    %s %d\n", indent, lbl(" Producer Count "), qd.ProducerCount, lbl(" Expired "), qd.ExpiredCount)
		fmt.Fprintf(&b, "%s  %s %-20s    %s %d%%\n", indent, lbl(" Memory Usage "), formatBytes(qd.MemoryUsageBytes), lbl(" Memory Percent "), qd.MemoryPercentUsage)
		fmt.Fprintf(&b, "%s  %s %-20s    %s %s\n", indent, lbl(" Store Size "), formatBytes(qd.StoreMessageSize), lbl(" Total Enqueued "), formatWithCommas(qd.EnqueueCount))
		fmt.Fprintf(&b, "%s  %s %-20s    %s %.2f ms\n\n", indent, lbl(" Total Dequeued "), formatWithCommas(qd.DequeueCount), lbl(" Avg Blocked Time "), qd.AverageBlockedTime)

		fmt.Fprintf(&b, "%s%s\n", indent, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")).Render(" [ Active Consumers ] "))

		statsStr := b.String()

		var tableStr string
		if len(qd.Consumers) == 0 {
			tableStr = fmt.Sprintf("\n%s  No active consumers.\n", indent)
		} else {
			rawTableStr := m.consumersTable.View()
			lines := strings.Split(rawTableStr, "\n")
			if len(lines) > 0 {
				// Top long separator line
				longLine := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(strings.Repeat("-", contentW-4))
				newLines := append([]string{longLine}, lines...)
				tableStr = strings.Join(newLines, "\n")
			} else {
				tableStr = rawTableStr
			}
		}

		combined := lipgloss.JoinVertical(lipgloss.Left, statsStr, tableStr)
		infoBox := lipgloss.NewStyle().MarginLeft(2).Padding(0, 1).Border(asciiBorder).BorderForeground(lipgloss.Color("8")).Width(contentW).Render(combined)

		return lipgloss.JoinVertical(lipgloss.Left, headerOk, sub, infoBox, footerOk)
	}

	if m.currentState == stateConnections {
		var sub string
		if m.err != nil {
			sub = lipgloss.NewStyle().MarginLeft(2).Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("Error: %v", m.err))
		} else {
			sub = lipgloss.NewStyle().MarginLeft(2).Render(fmt.Sprintf("Connections : %d (total) | Command: <%s> Back", len(m.connections), y("Esc")))
		}

		tableBox := lipgloss.NewStyle().MarginLeft(2).Border(asciiBorder).BorderForeground(lipgloss.Color("8")).Padding(0, 1).Width(contentW).Render(m.connectionsTable.View())

		return lipgloss.JoinVertical(lipgloss.Left, headerOk, sub, tableBox, footerOk)
	}

	// Confirm Delete Popup
	if m.currentState == stateConfirmDelete || m.currentState == stateConfirmMultiDelete {
		var title, body string
		if m.currentState == stateConfirmMultiDelete {
			title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("Delete Multiple Messages")
			selectedCount := 0
			for _, sel := range m.selectedMessages {
				if sel {
					selectedCount++
				}
			}
			body = fmt.Sprintf("Are you sure you want to delete %d selected message(s)?", selectedCount)
		} else {
			title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("Delete Queue")
			body = fmt.Sprintf("Are you sure you want to delete:\n\n  %s",
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Render(m.confirmTarget))
		}

		okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Padding(0, 1)
		cancelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Padding(0, 1)
		focusedBtnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")).Bold(true).Padding(0, 1)

		if m.confirmFocus == 0 {
			okStyle = focusedBtnStyle
		} else {
			cancelStyle = focusedBtnStyle
		}

		buttons := lipgloss.JoinHorizontal(lipgloss.Center, okStyle.Render("[  OK  ]"), "      ", cancelStyle.Render("[ Cancel ]"))

		content := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", buttons)
		popup := lipgloss.NewStyle().
			Border(asciiBorder).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 3).
			Width(50).
			Render(content)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
	}

	// Search Popup
	if m.currentState == stateSearch {
		popup := m.searchForm.View()
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
	}

	// Time Delete Popup
	if m.currentState == stateTimeDeletePopup {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.viewTimeDeletePopup())
	}

	// Other Form Popups (Create Queue, Send, Move)
	formToRender := m.popupForm
	popup := popupOverlayStyle.Render(formToRender.View())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
}
