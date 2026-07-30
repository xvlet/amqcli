package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/xvlet/amqcli/domain"
	"github.com/xvlet/amqcli/usecase"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var (
	// titleStyle will be set dynamically in NewAppModel to handle profile-specific colors
	titleStyle        lipgloss.Style
	popupOverlayStyle = lipgloss.NewStyle().Padding(2, 4)
	appBorder         lipgloss.Border
)

func init() {
	lang := strings.ToUpper(os.Getenv("LANG"))
	lcAll := strings.ToUpper(os.Getenv("LC_ALL"))
	term := strings.ToUpper(os.Getenv("TERM"))

	// Determine if the terminal supports UTF-8
	supportsUTF8 := strings.Contains(lang, "UTF-8") || strings.Contains(lcAll, "UTF-8") || strings.Contains(term, "XTERM") || strings.Contains(term, "256COLOR")

	if supportsUTF8 {
		appBorder = lipgloss.RoundedBorder()
	} else {
		appBorder = lipgloss.Border{
			Top:         "-",
			Bottom:      "-",
			Left:        "|",
			Right:       "|",
			TopLeft:     "+",
			TopRight:    "+",
			BottomLeft:  "+",
			BottomRight: "+",
		}
	}
}

type state int

const (
	stateList state = iota
	stateCreateQueue
	stateSendTo
	stateMessageList
	stateMessageDetail
	stateMoveMessage
	stateConfirmDelete
	stateSearch
	stateQueueInfo
	stateConnections
	stateConfirmMultiDelete
	stateTimeDeletePopup
)

type AppModel struct {
	uc              *usecase.ActiveMQUseCase
	refreshInterval time.Duration
	viewStats       bool

	queues          []domain.Queue
	messages        []domain.Message
	selectedMessage *domain.Message

	queueTable table.Model
	msgTable   table.Model

	width         int
	height        int
	host          string
	lastUpdated   time.Time
	err           error
	brokerInfo    string
	brokerStats   domain.BrokerStats
	currentState  state
	popupForm     FormModel
	selectedQueue string
	confirmTarget string // queue name pending deletion confirm
	searchForm    SearchFormModel
	isFiltered    bool
	filterType    string
	filterKeyword string
	isLoadingMore bool
	viewport      viewport.Model
	viewportReady bool
	msgFirstLoad  bool // true only on first entry into a queue's message list

	selectedQueueDetail *domain.QueueDetail
	connections         []domain.Connection
	connectionsTable    table.Model
	consumersTable      table.Model

	selectedMessages map[string]bool
	timeDeleteVal    string
	timeDeleteUnit   string
	timeDeleteFocus  int
	confirmFocus     int
}

func (m *AppModel) initViewport() {
	if !m.viewportReady {
		m.viewport = viewport.New(m.width-4, m.height-14) // Broad viewport width
		m.viewportReady = true
	}
}

type tickMsg time.Time

func NewAppModel(uc *usecase.ActiveMQUseCase, interval time.Duration, host string) *AppModel {
	// Original profile detection for conditional styling
	detectedProfile := lipgloss.ColorProfile()

	// Force at least ANSI profile if TERM is set but detection resulted in Ascii
	if detectedProfile == termenv.Ascii && os.Getenv("TERM") != "" {
		lipgloss.SetColorProfile(termenv.ANSI)
	}

	// Initialize dynamic global styles based on profile
	isHighColor := (detectedProfile == termenv.ANSI256 || detectedProfile == termenv.TrueColor)
	if isHighColor {
		titleStyle = lipgloss.NewStyle().MarginLeft(2).Bold(true).Foreground(lipgloss.Color("#c6a0f6"))
	} else {
		titleStyle = lipgloss.NewStyle().MarginLeft(2).Bold(true).Foreground(lipgloss.Color("#c6a0f6"))
	}

	// 1. Queue Table
	qCols := []table.Column{
		{Title: "Name", Width: 26},
		{Title: fmt.Sprintf("%10s", "Pending"), Width: 10},
		{Title: fmt.Sprintf("%10s", "Consumers"), Width: 10},
		{Title: fmt.Sprintf("%10s", "Enqueued"), Width: 10},
		{Title: fmt.Sprintf("%10s", "Dequeued"), Width: 10},
	}
	qTable := table.New(table.WithColumns(qCols), table.WithFocused(true))

	// 2. Message Browse Table
	mCols := []table.Column{
		{Title: "SEQ", Width: 7},
		{Title: "Message ID", Width: 46},
		{Title: "Correlation ID", Width: 36},
		{Title: "Persistence", Width: 12},
		{Title: "Priority", Width: 8},
		{Title: "Redelivered", Width: 12},
		{Title: "Timestamp", Width: 30},
		{Title: "Action", Width: 15},
	}
	mTable := table.New(table.WithColumns(mCols), table.WithFocused(true))

	// 3. Connections Table
	cCols := []table.Column{
		{Title: "Name", Width: 40},
		{Title: "Remote Address", Width: 30},
		{Title: "Active", Width: 10},
		{Title: "Slow", Width: 10},
	}
	cTable := table.New(table.WithColumns(cCols), table.WithFocused(true))

	// 4. Consumers Table
	conCols := []table.Column{
		{Title: "PID", Width: 10},
		{Title: "Remote Address", Width: 25},
		{Title: "Client ID", Width: 40},
		{Title: "Dequeues", Width: 10},
		{Title: "Uptime", Width: 15},
	}
	conTable := table.New(table.WithColumns(conCols), table.WithFocused(true))

	s := table.DefaultStyles()
	s.Header = s.Header.BorderStyle(appBorder).BorderForeground(lipgloss.Color("#5b6078")).BorderBottom(true).Bold(false)

	// Determine theme based on detected profile
	isHighColor = (detectedProfile == termenv.ANSI256 || detectedProfile == termenv.TrueColor)

	if isHighColor {
		// Original 256-color theme
		s.Selected = s.Selected.Foreground(lipgloss.Color("#181926")).Background(lipgloss.Color("#8aadf4")).Bold(false)
	} else {
		// Limited terminal: Use high-contrast Black on White theme (as requested)
		s.Selected = s.Selected.Foreground(lipgloss.Color("0")).Background(lipgloss.Color("15")).Bold(false)
	}

	qTable.SetStyles(s)
	mTable.SetStyles(s)
	cTable.SetStyles(s)
	conTable.SetStyles(s)

	return &AppModel{
		uc:               uc,
		refreshInterval:  interval,
		host:             host,
		viewStats:        false,
		lastUpdated:      time.Now(),
		queueTable:       qTable,
		msgTable:         mTable,
		connectionsTable: cTable,
		consumersTable:   conTable,
		currentState:     stateList,
		selectedMessages: make(map[string]bool),
	}
}

func (m *AppModel) Init() tea.Cmd {
	initCmds := []tea.Cmd{m.tickRefresh(), m.fetchQueues(), m.fetchBrokerInfo()}
	if m.viewStats {
		initCmds = append(initCmds, m.fetchBrokerStats())
	}
	return tea.Batch(initCmds...)
}

type (
	brokerInfoMsg  string
	brokerStatsMsg domain.BrokerStats
)
