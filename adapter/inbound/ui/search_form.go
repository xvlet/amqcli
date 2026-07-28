package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	SearchFieldRadio = iota
	SearchFieldKeyword
	SearchFieldOkBtn
	SearchFieldCancelBtn
)

const (
	SearchTypeCorrelationID = "Correlation ID"
	SearchTypeMessageID     = "Message ID"
)

type SearchFormModel struct {
	focusIndex int
	searchType string // "Correlation ID" or "Message ID"
	keyword    textinput.Model
}

func NewSearchFormModel() SearchFormModel {
	k := textinput.New()
	k.Cursor.Style = cursorStyle
	k.Placeholder = "Enter keyword..."
	k.PlaceholderStyle = noStyle
	k.CharLimit = 100
	k.Width = 30

	return SearchFormModel{
		focusIndex: 0,
		searchType: SearchTypeCorrelationID,
		keyword:    k,
	}
}

func (m SearchFormModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m SearchFormModel) Update(msg tea.Msg) (SearchFormModel, tea.Cmd, string) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, nil, SubmitOptCancel
		case "tab", "down":
			m.focusIndex = (m.focusIndex + 1) % 4
		case "shift+tab", "up":
			m.focusIndex--
			if m.focusIndex < 0 {
				m.focusIndex = 3
			}
		case "left", "right":
			switch m.focusIndex {
			case SearchFieldRadio:
				if m.searchType == SearchTypeCorrelationID {
					m.searchType = SearchTypeMessageID
				} else {
					m.searchType = SearchTypeCorrelationID
				}
			case SearchFieldOkBtn:
				m.focusIndex = SearchFieldCancelBtn
			case SearchFieldCancelBtn:
				m.focusIndex = SearchFieldOkBtn
			}
		case "enter":
			switch m.focusIndex {
			case SearchFieldKeyword:
				m.focusIndex = SearchFieldOkBtn
			case SearchFieldOkBtn:
				return m, nil, SubmitOptOk
			case SearchFieldCancelBtn:
				return m, nil, SubmitOptCancel
			}
		}
	}

	// Update focus
	if m.focusIndex == SearchFieldKeyword {
		m.keyword.Focus()
	} else {
		m.keyword.Blur()
	}

	var cmd tea.Cmd
	m.keyword, cmd = m.keyword.Update(msg)
	return m, cmd, ""
}

func (m SearchFormModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Search Messages") + "\n\n")

	// Radio Buttons
	b.WriteString("Search By:\n")
	corrLabel := " ( ) " + SearchTypeCorrelationID
	if m.searchType == SearchTypeCorrelationID {
		corrLabel = " (X) " + SearchTypeCorrelationID
	}
	msgLabel := " ( ) " + SearchTypeMessageID
	if m.searchType == SearchTypeMessageID {
		msgLabel = " (X) " + SearchTypeMessageID
	}

	radioContent := ""
	if m.focusIndex == SearchFieldRadio {
		radioContent = focusedStyle.Render(corrLabel) + "   " + focusedStyle.Render(msgLabel)
	} else {
		if m.searchType == SearchTypeCorrelationID {
			radioContent = focusedStyle.Render(corrLabel) + "   " + blurredStyle.Render(msgLabel)
		} else {
			radioContent = blurredStyle.Render(corrLabel) + "   " + focusedStyle.Render(msgLabel)
		}
	}
	b.WriteString(radioContent + "\n\n")

	// Keyword
	b.WriteString("Keyword:\n")
	if m.focusIndex == SearchFieldKeyword {
		m.keyword.PromptStyle = focusedStyle
		m.keyword.TextStyle = focusedStyle
	} else {
		m.keyword.PromptStyle = noStyle
		m.keyword.TextStyle = noStyle
	}
	b.WriteString(m.keyword.View() + "\n\n")

	// OK / Cancel Buttons
	okStyle := btnStyle
	if m.focusIndex == SearchFieldOkBtn {
		okStyle = focusedBtnStyle
	}

	cancelStyle := btnStyle
	if m.focusIndex == SearchFieldCancelBtn {
		cancelStyle = focusedBtnStyle
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Top,
		okStyle.Render("[  OK  ]"),
		"   ",
		cancelStyle.Render("[ Cancel ]"),
	)
	b.WriteString(buttons + "\n")

	return lipgloss.NewStyle().
		Border(appBorder).
		BorderForeground(popupBorder).
		Padding(1, 2).
		Width(50).
		Render(b.String())
}

func (m SearchFormModel) GetValues() (string, string) {
	return m.searchType, m.keyword.Value()
}
