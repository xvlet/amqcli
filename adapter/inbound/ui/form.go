package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	// Use standard bright ANSI colors to prevent dark gray rendering bugs
	// that occur with some pastel hex colors (like #cad3f5) in certain terminal environments.
	focusedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")) // Bright White
	blurredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))  // Normal White
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("14")) // Bright Cyan
	noStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))  // Bright Black (Dark Gray)

	btnStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Padding(0, 1)
	focusedBtnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("14")).Bold(true).Padding(0, 1)

	popupBorder = lipgloss.Color("14") // Bright Cyan

	SubmitOptOk     = "OK"
	SubmitOptCancel = "Cancel"
)

// FormModel represents a generic multiple-input form
type FormModel struct {
	Title       string
	focusIndex  int
	inputs      []textinput.Model
	FieldLabels []string
	Active      bool
}

func NewFormModel(title string, fields []string) FormModel {
	m := FormModel{
		Title:       title,
		inputs:      make([]textinput.Model, len(fields)),
		FieldLabels: fields,
	}

	for i := range m.inputs {
		t := textinput.New()
		t.Cursor.Style = cursorStyle
		t.CharLimit = 120

		if i == 0 {
			t.Focus()
			t.PromptStyle = focusedStyle
			t.TextStyle = focusedStyle
		}
		m.inputs[i] = t
	}

	return m
}

func (m FormModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m FormModel) Update(msg tea.Msg) (FormModel, tea.Cmd, string) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.Active = false
			return m, nil, SubmitOptCancel

		case "tab", "shift+tab", "enter", "up", "down", "left", "right":
			s := msg.String()

			if s == "enter" {
				if m.focusIndex == len(m.inputs) {
					m.Active = false
					return m, nil, SubmitOptOk
				} else if m.focusIndex == len(m.inputs)+1 {
					m.Active = false
					return m, nil, SubmitOptCancel
				} else {
					m.focusIndex = len(m.inputs) // On input field enter -> go to OK
				}
			} else {
				switch s {
				case "up", "shift+tab":
					m.focusIndex--
				case "down", "tab":
					m.focusIndex++
				case "left", "right":
					if m.focusIndex >= len(m.inputs) {
						if m.focusIndex == len(m.inputs) {
							m.focusIndex = len(m.inputs) + 1
						} else {
							m.focusIndex = len(m.inputs)
						}
					}
				}
			}

			if m.focusIndex > len(m.inputs)+1 {
				m.focusIndex = 0
			} else if m.focusIndex < 0 {
				m.focusIndex = len(m.inputs) + 1
			}

			cmds := make([]tea.Cmd, len(m.inputs))
			for i := 0; i <= len(m.inputs)-1; i++ {
				if i == m.focusIndex {
					cmds[i] = m.inputs[i].Focus()
					m.inputs[i].PromptStyle = focusedStyle
					m.inputs[i].TextStyle = focusedStyle
					continue
				}
				m.inputs[i].Blur()
				m.inputs[i].PromptStyle = noStyle
				m.inputs[i].TextStyle = noStyle
			}

			return m, tea.Batch(cmds...), ""
		}
	}

	cmd := m.updateInputs(msg)
	return m, cmd, ""
}

func (m *FormModel) updateInputs(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))
	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}
	return tea.Batch(cmds...)
}

func (m FormModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(m.Title) + "\n\n")

	for i := range m.inputs {
		b.WriteString(m.FieldLabels[i] + "\n")
		b.WriteString(m.inputs[i].View() + "\n\n")
	}

	okStyle := btnStyle
	if m.focusIndex == len(m.inputs) {
		okStyle = focusedBtnStyle
	}
	cancelStyle := btnStyle
	if m.focusIndex == len(m.inputs)+1 {
		cancelStyle = focusedBtnStyle
	}
	buttons := lipgloss.JoinHorizontal(lipgloss.Top, okStyle.Render("[  OK  ]"), "   ", cancelStyle.Render("[ Cancel ]"))
	fmt.Fprintf(&b, "%s\n", buttons)

	return lipgloss.NewStyle().Border(appBorder).BorderForeground(lipgloss.Color("#8aadf4")).Padding(1, 2).Render(b.String())
}

func (m FormModel) GetValues() []string {
	res := make([]string, len(m.inputs))
	for i, in := range m.inputs {
		res[i] = in.Value()
	}
	return res
}
