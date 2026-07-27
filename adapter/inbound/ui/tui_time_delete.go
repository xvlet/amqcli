package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *AppModel) updateTimeDeletePopup(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.currentState = stateMessageList
			return m, nil
		case "enter":
			switch m.timeDeleteFocus {
			case 1:
				timeStr := m.timeDeleteVal + m.timeDeleteUnit
				err := m.uc.DeleteMessagesByTime(m.selectedQueue, timeStr)
				if err != nil {
					m.currentState = stateMessageList
					return m, nil
				}
				m.currentState = stateMessageList
				return m, m.fetchMessages()
			case 2:
				m.currentState = stateMessageList
				return m, nil
			default:
				m.timeDeleteFocus = 1
			}
		case "tab", "right":
			switch m.timeDeleteFocus {
			case 0:
				m.timeDeleteFocus = 1
			case 1:
				m.timeDeleteFocus = 2
			default:
				m.timeDeleteFocus = 0
			}
		case "shift+tab", "left":
			switch m.timeDeleteFocus {
			case 0:
				m.timeDeleteFocus = 2
			case 1:
				m.timeDeleteFocus = 0
			default:
				m.timeDeleteFocus = 1
			}
		case "up":
			m.timeDeleteFocus = 0
		case "down":
			if m.timeDeleteFocus == 0 {
				m.timeDeleteFocus = 1
			}
		case "m", "M":
			if m.timeDeleteFocus == 0 {
				m.timeDeleteUnit = "m"
			}
		case "h", "H":
			if m.timeDeleteFocus == 0 {
				m.timeDeleteUnit = "h"
			}
		case "d", "D":
			if m.timeDeleteFocus == 0 {
				m.timeDeleteUnit = "d"
			}
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			if m.timeDeleteFocus == 0 && len(m.timeDeleteVal) < 5 { // limit length
				if m.timeDeleteVal == "0" {
					m.timeDeleteVal = keyMsg.String()
				} else {
					m.timeDeleteVal += keyMsg.String()
				}
			}
		case "backspace", "delete":
			if m.timeDeleteFocus == 0 && len(m.timeDeleteVal) > 0 {
				m.timeDeleteVal = m.timeDeleteVal[:len(m.timeDeleteVal)-1]
			}
		}
	}
	return m, nil
}

func (m *AppModel) viewTimeDeletePopup() string {
	border := lipgloss.NewStyle().
		Border(appBorder).
		BorderForeground(lipgloss.Color("#8aadf4")).
		Padding(1, 2)

	title := lipgloss.NewStyle().Foreground(lipgloss.Color("#c6a0f6")).Bold(true).Render("Delete By Time")
	desc := lipgloss.NewStyle().Foreground(lipgloss.Color("#cad3f5")).Render("Delete messages older than:")

	valColor := lipgloss.Color("#181926")
	if m.timeDeleteFocus != 0 {
		valColor = lipgloss.Color("#8087a2")
	}
	valStr := lipgloss.NewStyle().Foreground(valColor).Render(m.timeDeleteVal)
	if m.timeDeleteVal == "" {
		valStr = "_"
	}

	unitStr := "days"
	mStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8087a2"))
	hStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8087a2"))
	dStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8087a2"))

	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#181926")).Bold(true)

	switch m.timeDeleteUnit {
	case "m":
		unitStr = "minutes"
		mStyle = activeStyle
	case "h":
		unitStr = "hours"
		hStyle = activeStyle
	default:
		dStyle = activeStyle
	}

	units := fmt.Sprintf("%s / %s / %s",
		mStyle.Render("(m)inutes"),
		hStyle.Render("(h)ours"),
		dStyle.Render("(d)ays"),
	)

	bracketLeft := lipgloss.NewStyle().Foreground(lipgloss.Color("#8087a2")).Render("[ ")
	bracketRight := lipgloss.NewStyle().Foreground(lipgloss.Color("#8087a2")).Render(" ]")
	if m.timeDeleteFocus == 0 {
		bracketLeft = lipgloss.NewStyle().Foreground(lipgloss.Color("#181926")).Render("[ ")
		bracketRight = lipgloss.NewStyle().Foreground(lipgloss.Color("#181926")).Render(" ]")
	}

	inputLine := fmt.Sprintf("%s%s%s %s", bracketLeft, valStr, bracketRight, units)

	selectionInfo := lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render(fmt.Sprintf("Current selection: %s %s", m.timeDeleteVal, unitStr))
	if m.timeDeleteVal == "" {
		selectionInfo = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render("Current selection: (invalid)")
	}

	btnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a5adcb")).Padding(0, 1)
	focusedBtnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#181926")).Background(lipgloss.Color("#8aadf4")).Bold(true).Padding(0, 1)

	delStyle := btnStyle
	canStyle := btnStyle

	switch m.timeDeleteFocus {
	case 1:
		delStyle = focusedBtnStyle
	case 2:
		canStyle = focusedBtnStyle
	}

	footer := lipgloss.JoinHorizontal(lipgloss.Center, delStyle.Render("[  OK  ]"), "      ", canStyle.Render("[ Cancel ]"))

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"---------------------------------------",
		"",
		lipgloss.NewStyle().Align(lipgloss.Left).Render(desc),
		"",
		lipgloss.NewStyle().Align(lipgloss.Left).Render(inputLine),
		"",
		lipgloss.NewStyle().Align(lipgloss.Left).Render(selectionInfo),
		"",
		footer,
	)

	return border.Render(content)
}
