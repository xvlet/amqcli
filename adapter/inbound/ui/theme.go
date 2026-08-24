package ui

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	Primary    lipgloss.Color
	Secondary  lipgloss.Color
	Success    lipgloss.Color
	Warning    lipgloss.Color
	Error      lipgloss.Color
	Info       lipgloss.Color
	Border     lipgloss.Color
	Text       lipgloss.Color
	MutedText  lipgloss.Color
	Highlight  lipgloss.Color
	Background lipgloss.Color
	ActiveItem lipgloss.Color
}

var AppTheme = Theme{
	Primary:    lipgloss.Color("#c6a0f6"), // Pink/Purple (Headers)
	Secondary:  lipgloss.Color("#8aadf4"), // Blue (Focus)
	Success:    lipgloss.Color("#a6da95"), // Green (Connected, Success)
	Warning:    lipgloss.Color("208"),     // Orange/Yellow (Warnings)
	Error:      lipgloss.Color("#ed8796"), // Red (Disconnected, Error)
	Info:       lipgloss.Color("#91d7e3"), // Cyan (Info, Memory, Temps)
	Border:     lipgloss.Color("#5b6078"), // Muted Blue/Gray (Borders)
	Text:       lipgloss.Color("#cad3f5"), // White/Light Gray (Normal text)
	MutedText:  lipgloss.Color("#8087a2"), // Darker Gray (Hints, Unfocused)
	Highlight:  lipgloss.Color("14"),      // Bright Cyan (Search highlights)
	Background: lipgloss.Color("#181926"), // Dark Base (Buttons, etc.)
	ActiveItem: lipgloss.Color("#eed49f"), // Yellow (Active selection)
}
