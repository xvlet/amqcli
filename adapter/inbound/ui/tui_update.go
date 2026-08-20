package ui

import (
	"fmt"
	"github.com/xvlet/amqcli/domain"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		if m.err != nil && (strings.Contains(m.err.Error(), "read-only") || strings.Contains(m.err.Error(), "snapshot") || strings.Contains(m.err.Error(), "aborting") || strings.Contains(m.err.Error(), "queue name did not match")) {
			m.err = nil
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalculateTableWidths()
		m.queueTable.SetHeight(msg.Height - 12)
		m.msgTable.SetHeight(msg.Height - 12)
		m.connectionsTable.SetHeight(msg.Height - 12)

		conHeight := msg.Height - 18
		if conHeight < 3 {
			conHeight = 3
		}
		m.consumersTable.SetHeight(conHeight)

		if !m.viewportReady {
			m.initViewport()
		} else {
			m.viewport.Width = msg.Width - 4
			m.viewport.Height = msg.Height - 14
		}
		return m, nil
	case error:
		m.err = msg
		return m, m.tickRefresh() // Keep tick alive even after error so recovery is automatic
	case tickMsg:
		if m.err != nil && (strings.Contains(m.err.Error(), "read-only") || strings.Contains(m.err.Error(), "snapshot") || strings.Contains(m.err.Error(), "aborting") || strings.Contains(m.err.Error(), "queue name did not match")) {
			m.err = nil
		}
		switch m.currentState {
		case stateList:
			cmds := []tea.Cmd{m.fetchQueues(), m.tickRefresh(), m.fetchBrokerStats(), m.fetchConnections()}
			return m, tea.Batch(cmds...)
		case stateMessageList:
			return m, tea.Batch(m.fetchMessages(), m.tickRefresh(), m.fetchBrokerStats(), m.fetchConnections())
		case stateQueueInfo:
			return m, tea.Batch(m.fetchQueueDetail(m.selectedQueue), m.tickRefresh(), m.fetchBrokerStats(), m.fetchConnections())
		case stateConnections:
			return m, tea.Batch(m.fetchConnections(), m.tickRefresh(), m.fetchBrokerStats())
		default:
			return m, tea.Batch(m.tickRefresh(), m.fetchBrokerStats(), m.fetchConnections())
		}
	case snapshotSavedMsg:
		if msg.err != nil {
			m.err = fmt.Errorf("failed to save snapshot: %v", msg.err)
		} else {
			m.err = fmt.Errorf("snapshot saved to %s", msg.filename)
		}
		return m, nil
	case brokerInfoMsg:
		if string(msg) != "" {
			m.brokerInfo = string(msg)
		}
		return m, nil
	case brokerStatsMsg:
		m.brokerStats = domain.BrokerStats(msg)
		return m, nil
	case []domain.Queue:
		m.err = nil
		m.lastUpdated = time.Now()
		m.queues = msg
		rows := m.buildQueueRows(msg)
		m.queueTable.SetRows(rows)
		return m, nil
	case *domain.QueueDetail:
		m.err = nil
		m.lastUpdated = time.Now()
		m.selectedQueueDetail = msg
		if msg != nil {
			// Sort by Client ID then Remote Address for a stable and consistent display order
			sort.Slice(msg.Consumers, func(i, j int) bool {
				cI := strings.ToLower(msg.Consumers[i].ClientID)
				cJ := strings.ToLower(msg.Consumers[j].ClientID)
				if cI != cJ {
					return cI < cJ
				}
				return msg.Consumers[i].RemoteAddress < msg.Consumers[j].RemoteAddress
			})

			rows := make([]table.Row, len(msg.Consumers))
			for i, c := range msg.Consumers {
				rows[i] = table.Row{
					c.PID,
					c.RemoteAddress,
					c.ClientID,
					strconv.FormatInt(c.Dequeues, 10),
					c.Uptime,
				}
			}
			m.consumersTable.SetRows(rows)
		}
		return m, nil
	case []domain.Connection:
		m.err = nil
		m.lastUpdated = time.Now()
		m.connections = msg

		// Sort by Name (case-insensitive)
		sort.Slice(m.connections, func(i, j int) bool {
			return strings.ToLower(m.connections[i].Name) < strings.ToLower(m.connections[j].Name)
		})

		rows := make([]table.Row, len(m.connections))
		for i, c := range m.connections {
			active := "false"
			if c.Active {
				active = "true"
			}
			slow := "false"
			if c.Slow {
				slow = "true"
			}
			rows[i] = table.Row{c.Name, c.RemoteAddress, active, slow}
		}
		m.connectionsTable.SetRows(rows)
		return m, nil
	case []domain.Message:
		m.err = nil
		if m.currentState == stateMessageList || m.currentState == stateMessageDetail {
			m.lastUpdated = time.Now()
		}
		m.messages = msg
		var displayMsgs []domain.Message
		if m.isFiltered {
			displayMsgs = []domain.Message{}
			for _, v := range msg {
				if len(strings.TrimSpace(v.MessageID)) < 5 {
					continue
				}
				target := v.CorrelationID
				if m.filterType == SearchTypeMessageID {
					target = v.MessageID
				}
				if strings.Contains(strings.ToLower(target), strings.ToLower(m.filterKeyword)) {
					displayMsgs = append(displayMsgs, v)
				}
			}
			m.messages = msg // Store all, then filter for display
		} else {
			newMsgs := []domain.Message{}
			for _, v := range msg {
				if len(strings.TrimSpace(v.MessageID)) >= 5 {
					newMsgs = append(newMsgs, v)
				}
			}
			displayMsgs = newMsgs
			m.messages = newMsgs
		}

		rows := make([]table.Row, len(displayMsgs))

		// calculate width for right alignment based on max number
		maxLen := len(strconv.Itoa(len(displayMsgs)))
		if maxLen < 3 {
			maxLen = 3
		}

		seqW := maxLen + 2
		cols := m.msgTable.Columns()
		if len(cols) > 0 && cols[0].Width < seqW {
			cols[0].Width = seqW
			m.msgTable.SetColumns(cols)
		}

		for i, mg := range displayMsgs {
			rd := "false"
			if mg.Redelivered {
				rd = "true"
			}
			seqNum := fmt.Sprintf("%*d", maxLen, i+1)

			if m.selectedMessages[mg.MessageID] {
				seqStr := "v " + seqNum
				rows[i] = table.Row{seqStr, mg.MessageID, mg.CorrelationID, mg.Persistence, strconv.Itoa(mg.Priority), rd, mg.Timestamp.Format(time.RFC3339), "[d] Delete"}
			} else {
				seqStr := "  " + seqNum
				rows[i] = table.Row{seqStr, mg.MessageID, mg.CorrelationID, mg.Persistence, strconv.Itoa(mg.Priority), rd, mg.Timestamp.Format(time.RFC3339), "[d] Delete"}
			}
		}
		m.msgTable.SetRows(rows)
		// Only go to top on first entry into the queue; preserve position during refresh
		if m.msgFirstLoad {
			m.msgTable.GotoTop()
			m.msgFirstLoad = false
		}
		return m, nil
	case moreMessagesMsg:
		m.isLoadingMore = false
		if len(msg) == 0 {
			return m, nil
		}

		// Merge and skip duplicates/empty
		existingIds := make(map[string]bool)
		for _, v := range m.messages {
			if v.MessageID != "" {
				existingIds[v.MessageID] = true
			}
		}

		added := 0
		for _, v := range msg {
			if v.MessageID == "" {
				continue
			}
			if !existingIds[v.MessageID] {
				m.messages = append(m.messages, v)
				added++
			}
		}

		if added > 0 {
			// Trigger a re-render
			return m.Update(m.messages)
		}
		return m, nil
	}

	switch m.currentState {
	case stateList:
		return m.updateList(msg)
	case stateMessageList:
		return m.updateMessageList(msg)
	case stateMessageDetail:
		return m.updateMessageDetail(msg)
	case stateConfirmMultiDelete:
		return m.updateConfirmDelete(msg)
	case stateCreateQueue, stateSendTo, stateMoveMessage, stateConfirmDelete, stateConfirmPurge:
		return m.updatePopup(msg)
	case stateTimeDeletePopup:
		return m.updateTimeDeletePopup(msg)
	case stateSearch:
		return m.updateSearch(msg)
	case stateQueueInfo:
		return m.updateQueueInfo(msg)
	case stateConnections:
		return m.updateConnections(msg)
	}

	return m, nil
}

func (m *AppModel) updateConfirmDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "left", "right", "tab", "shift+tab":
			if m.confirmFocus == 0 {
				m.confirmFocus = 1
			} else {
				m.confirmFocus = 0
			}
		case "enter":
			if m.confirmFocus == 0 { // OK
				if m.currentState == stateConfirmMultiDelete {
					m.currentState = stateMessageList
					var targets []string
					for id, sel := range m.selectedMessages {
						if sel {
							targets = append(targets, id)
						}
					}
					m.selectedMessages = make(map[string]bool)

					return m, func() tea.Msg {
						for _, id := range targets {
							_ = m.uc.DeleteMessage(m.selectedQueue, id)
						}
						return m.fetchMessages()()
					}
				}

				qName := m.confirmTarget
				m.confirmTarget = ""
				m.currentState = stateList
				return m, func() tea.Msg {
					err := m.uc.DeleteQueue(qName)
					if err != nil {
						return err
					}
					return m.fetchQueues()()
				}
			} else { // Cancel
				if m.currentState == stateConfirmMultiDelete {
					m.currentState = stateMessageList
					return m, nil
				}
				m.confirmTarget = ""
				m.currentState = stateList
				return m, nil
			}
		case "esc", "n", "N", "q", "ctrl+c":
			if m.currentState == stateConfirmMultiDelete {
				m.currentState = stateMessageList
				return m, nil
			}
			m.confirmTarget = ""
			m.currentState = stateList
			return m, nil
		case "y", "Y":
			// Fallback shortcut for quick confirm
			m.confirmFocus = 0
			return m.updateConfirmDelete(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
		}
	}
	return m, nil
}

func (m *AppModel) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			if row := m.queueTable.SelectedRow(); row != nil {
				m.selectedQueue = row[0]
				m.currentState = stateMessageList
				m.msgFirstLoad = true // mark first load so cursor resets to top
				m.messages = []domain.Message{}
				m.msgTable.SetRows([]table.Row{})
				return m, m.fetchMessages()
			}
		case "c", "C", "alt+c":
			if m.readOnly {
				m.err = fmt.Errorf("read-only mode is active")
				return m, nil
			}
			m.currentState = stateCreateQueue
			m.popupForm = NewFormModel("Create Queue", []string{"Queue Name"})
			return m, m.popupForm.Init()
		case "s", "S", "alt+s":
			if m.readOnly {
				m.err = fmt.Errorf("read-only mode is active")
				return m, nil
			}
			if row := m.queueTable.SelectedRow(); row != nil {
				m.selectedQueue = row[0]
				m.currentState = stateSendTo
				m.popupForm = NewFormModel("Send Message to "+m.selectedQueue, []string{"CorrelationID", "TTL (seconds)", "Message Body"})
				return m, m.popupForm.Init()
			}
		case "p", "P", "alt+p":
			if m.readOnly {
				m.err = fmt.Errorf("read-only mode is active")
				return m, nil
			}
			if row := m.queueTable.SelectedRow(); row != nil {
				m.selectedQueue = row[0]
				m.currentState = stateConfirmPurge
				m.popupForm = NewFormModel(fmt.Sprintf("Purge Queue: %s\nType queue name to confirm", m.selectedQueue), []string{"Queue Name"})
				return m, m.popupForm.Init()
			}
		case "d", "D", "alt+d":
			if m.readOnly {
				m.err = fmt.Errorf("read-only mode is active")
				return m, nil
			}
			if row := m.queueTable.SelectedRow(); row != nil {
				m.confirmTarget = row[0]
				m.currentState = stateConfirmDelete // We can redefine stateConfirmDelete to use form
				m.popupForm = NewFormModel(fmt.Sprintf("Delete Queue: %s\nType queue name to confirm", m.confirmTarget), []string{"Queue Name"})
				return m, m.popupForm.Init()
			}
		case "i", "I":
			if row := m.queueTable.SelectedRow(); row != nil {
				m.selectedQueue = row[0]
				m.currentState = stateQueueInfo
				m.selectedQueueDetail = nil   // clear old data
				m.consumersTable.SetCursor(0) // reset cursor to first row
				return m, m.fetchQueueDetail(m.selectedQueue)
			}
		case "u", "U":
			m.viewStats = !m.viewStats
			m.updateQueueTableColumns()
			m.queueTable.SetRows(m.buildQueueRows(m.queues)) // update rows immediately to prevent column length panic
			if m.viewStats {
				return m, tea.Batch(m.fetchBrokerStats(), m.fetchQueues())
			}
			return m, m.fetchQueues()
		case "o", "O":
			m.err = fmt.Errorf("generating full diagnostic snapshot")
			return m, m.takeFullSnapshot()
		case "n", "N":
			m.currentState = stateConnections
			m.connections = nil
			return m, m.fetchConnections()
		}
	}

	var cmd tea.Cmd
	m.queueTable, cmd = m.queueTable.Update(msg)
	return m, cmd
}

func (m *AppModel) updateMessageList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			if m.isFiltered {
				m.isFiltered = false
				m.filterKeyword = ""
				// Immediately update table with current messages, then fetch latest data in background
				_, immediateCmd := m.Update(m.messages)
				return m, tea.Batch(immediateCmd, m.fetchMessages())
			}
			m.currentState = stateList
			m.selectedQueue = ""
			return m, m.fetchQueues()
		case "f3", "ctrl+f":
			m.currentState = stateSearch
			m.searchForm = NewSearchFormModel()
			return m, m.searchForm.Init()
		case " ":
			cursor := m.msgTable.Cursor()
			if cursor >= 0 && cursor < len(m.messages) {
				msgID := m.messages[cursor].MessageID
				m.selectedMessages[msgID] = !m.selectedMessages[msgID]
				m.msgTable.MoveDown(1)
				return m.Update(m.messages)
			}
		case "enter":
			if row := m.msgTable.SelectedRow(); row != nil {
				cursor := m.msgTable.Cursor()
				if cursor >= 0 && cursor < len(m.messages) {
					copiedMsg := m.messages[cursor]
					fullBody, err := m.uc.GetFullMessageBody(m.selectedQueue, copiedMsg.MessageID)
					if err == nil && fullBody != "" {
						copiedMsg.Body = fullBody
					}

					m.selectedMessage = &copiedMsg
					m.currentState = stateMessageDetail
					if m.viewportReady {
						m.viewport.GotoTop()
					}
					return m, nil
				}
			}
		case "d":
			if m.readOnly {
				m.err = fmt.Errorf("read-only mode is active")
				return m, nil
			}
			selectedCount := 0
			for _, sel := range m.selectedMessages {
				if sel {
					selectedCount++
				}
			}
			if selectedCount > 0 {
				m.confirmFocus = 1 // Default to Cancel for safety
				m.currentState = stateConfirmMultiDelete
				return m, nil
			}

			if row := m.msgTable.SelectedRow(); row != nil {
				cursor := m.msgTable.Cursor()
				if cursor >= 0 && cursor < len(m.messages) {
					msgID := m.messages[cursor].MessageID
					return m, func() tea.Msg {
						err := m.uc.DeleteMessage(m.selectedQueue, msgID)
						if err != nil {
							return err
						}
						return m.fetchMessages()()
					}
				}
			}
		case "p", "P", "alt+p":
			if m.readOnly {
				m.err = fmt.Errorf("read-only mode is active")
				return m, nil
			}
			m.timeDeleteVal = "3"
			m.timeDeleteUnit = "d"
			m.timeDeleteFocus = 0
			m.currentState = stateTimeDeletePopup
			return m, nil
		case "D", "alt+d":
			if m.readOnly {
				m.err = fmt.Errorf("read-only mode is active")
				return m, nil
			}
			m.currentState = stateConfirmPurge
			m.popupForm = NewFormModel(fmt.Sprintf("Purge Queue: %s\nType queue name to confirm", m.selectedQueue), []string{"Queue Name"})
			return m, m.popupForm.Init()
		}
	}

	var cmd tea.Cmd
	m.msgTable, cmd = m.msgTable.Update(msg)

	// Check for infinite scroll
	if m.currentState == stateMessageList && !m.isLoadingMore && len(m.messages) > 0 {
		curr := m.msgTable.Cursor()
		total := len(m.msgTable.Rows())
		// Trigger when near the bottom (within last 5 items) or at the bottom
		if curr >= total-5 && total >= 100 {
			pending := int64(0)
			for _, q := range m.queues {
				if q.Name == m.selectedQueue {
					pending = q.Pending
					break
				}
			}

			if int64(len(m.messages)) < pending {
				m.isLoadingMore = true
				return m, tea.Batch(cmd, m.fetchMoreMessages())
			}
		}
	}

	return m, cmd
}

func (m *AppModel) updateMessageDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			m.currentState = stateMessageList
			m.selectedMessage = nil
			return m, nil
		case "d", "D", "alt+d": // Delete
			if m.readOnly {
				m.err = fmt.Errorf("read-only mode is active")
				return m, nil
			}
			if m.selectedMessage != nil {
				return m, func() tea.Msg {
					err := m.uc.DeleteMessage(m.selectedQueue, m.selectedMessage.MessageID)
					if err != nil {
						return err
					}
					m.currentState = stateMessageList
					m.selectedMessage = nil
					return m.fetchMessages()()
				}
			}
		case "r", "R", "alt+r": // Retry (Usually valid if it's DLQ)
			if m.readOnly {
				m.err = fmt.Errorf("read-only mode is active")
				return m, nil
			}
			if m.selectedMessage != nil {
				return m, func() tea.Msg {
					err := m.uc.RetryMessage(m.selectedQueue, m.selectedMessage.MessageID)
					if err != nil {
						return err
					}
					m.currentState = stateMessageList
					m.selectedMessage = nil
					return m.fetchMessages()()
				}
			}
		case "m", "M", "alt+m": // Move
			if m.readOnly {
				m.err = fmt.Errorf("read-only mode is active")
				return m, nil
			}
			if m.selectedMessage != nil {
				m.currentState = stateMoveMessage
				m.popupForm = NewFormModel("Move Message", []string{"Destination Queue Name"})
				return m, m.popupForm.Init()
			}
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *AppModel) updatePopup(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var event string
	m.popupForm, cmd, event = m.popupForm.Update(msg)

	prevState := stateList
	if m.currentState == stateMoveMessage {
		prevState = stateMessageDetail
	}

	switch event {
	case SubmitOptCancel:
		m.currentState = prevState
		return m, nil
	case SubmitOptOk:
		vals := m.popupForm.GetValues()
		var actionCmd tea.Cmd

		if m.currentState == stateCreateQueue && len(vals) > 0 {
			actionCmd = func() tea.Msg {
				err := m.uc.CreateQueue(vals[0])
				if err != nil {
					return err
				}
				return m.fetchQueues()()
			}
		} else if m.currentState == stateSendTo && len(vals) == 3 {
			ttl := 30 * time.Second
			if vals[1] != "" {
				if t, err := time.ParseDuration(vals[1] + "s"); err == nil {
					ttl = t
				}
			}
			actionCmd = func() tea.Msg {
				err := m.uc.SendToQueue(m.selectedQueue, vals[0], ttl, vals[2])
				if err != nil {
					return err
				}
				return nil
			}
		} else if m.currentState == stateMoveMessage && len(vals) > 0 {
			dest := vals[0]
			msgID := m.selectedMessage.MessageID
			actionCmd = func() tea.Msg {
				err := m.uc.MoveMessage(m.selectedQueue, msgID, dest)
				if err != nil {
					return err
				}
				m.currentState = stateMessageList
				return m.fetchMessages()()
			}
			m.currentState = stateMessageList // skip detail since it's moved
			return m, actionCmd
		} else if m.currentState == stateConfirmDelete && len(vals) > 0 {
			if vals[0] != m.confirmTarget {
				m.err = fmt.Errorf("queue name did not match, aborting delete")
				m.currentState = stateList
				return m, nil
			}
			qName := m.confirmTarget
			m.confirmTarget = ""
			actionCmd = func() tea.Msg {
				err := m.uc.DeleteQueue(qName)
				if err != nil {
					return err
				}
				return m.fetchQueues()()
			}
			m.currentState = stateList
			return m, actionCmd
		} else if m.currentState == stateConfirmPurge && len(vals) > 0 {
			if vals[0] != m.selectedQueue {
				m.err = fmt.Errorf("queue name did not match, aborting purge")
				if prevState == stateList {
					m.currentState = stateList
				} else {
					m.currentState = stateMessageList
				}
				return m, nil
			}
			qName := m.selectedQueue
			actionCmd = func() tea.Msg {
				err := m.uc.PurgeQueue(qName)
				if err != nil {
					return err
				}
				if prevState == stateList {
					return m.fetchQueues()()
				}
				return m.fetchMessages()()
			}
			if prevState == stateList {
				m.currentState = stateList
			} else {
				m.currentState = stateMessageList
			}
			return m, actionCmd
		}

		m.currentState = prevState
		return m, actionCmd
	}

	return m, cmd
}

func (m *AppModel) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	var event string
	m.searchForm, _, event = m.searchForm.Update(msg)

	switch event {
	case SubmitOptCancel:
		m.currentState = stateMessageList
		return m, nil
	case SubmitOptOk:
		t, k := m.searchForm.GetValues()
		m.filterType = t
		m.filterKeyword = k
		m.isFiltered = true
		m.currentState = stateMessageList
		m.msgTable.GotoTop()

		// Apply filter to current messages
		return m.Update(m.messages)
	}

	return m, nil
}

func (m *AppModel) updateQueueInfo(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			m.currentState = stateList
			m.selectedQueueDetail = nil
			return m, nil
		case "o", "O":
			m.err = fmt.Errorf("generating queue diagnostic snapshot")
			return m, m.takeQueueSnapshot()
		}
	}
	var cmd tea.Cmd
	m.consumersTable, cmd = m.consumersTable.Update(msg)
	return m, cmd
}

func (m *AppModel) updateConnections(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			m.currentState = stateList
			m.connections = nil
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.connectionsTable, cmd = m.connectionsTable.Update(msg)
	return m, cmd
}

func (m *AppModel) updateQueueTableColumns() {
	// Name width is preserved from existing configuration to avoid shrinking unexpectedly
	qNameW := 26
	if len(m.queueTable.Columns()) > 0 {
		qNameW = m.queueTable.Columns()[0].Width
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
	m.queueTable.SetRows([]table.Row{}) // Temporarily clear rows to prevent panic during SetColumns
	m.queueTable.SetColumns(qCols)
}

func (m *AppModel) buildQueueRows(queues []domain.Queue) []table.Row {
	rows := make([]table.Row, len(queues))
	termEnv := strings.ToLower(os.Getenv("TERM"))
	langEnv := strings.ToLower(os.Getenv("LANG"))
	isASCII := (lipgloss.ColorProfile() == termenv.Ascii) ||
		strings.Contains(termEnv, "vt") ||
		strings.Contains(termEnv, "aix") ||
		(langEnv != "" && !strings.Contains(langEnv, "utf-8") && !strings.Contains(langEnv, "utf8"))

	for i, q := range queues {
		row := table.Row{
			q.Name,
			fmt.Sprintf("%12d", q.Pending),
			fmt.Sprintf("%12d", q.Consumers),
			fmt.Sprintf("%12d", q.Enqueued),
			fmt.Sprintf("%12d", q.Dequeued),
		}
		if m.viewStats {
			diskStr := "  -"
			if q.StoreMessageSize > 0 {
				diskStr = fmt.Sprintf("[ %s ]", formatBytes(q.StoreMessageSize))
			}
			actualMemPercent := float64(q.MemoryPercentUsage)
			if q.MemoryLimit > 0 {
				actualMemPercent = float64(q.MemoryUsageBytes) / float64(q.MemoryLimit) * 100.0
			}
			row = append(row, makeUsageBar(actualMemPercent, q.MemoryUsageBytes, isASCII), diskStr)
		}
		rows[i] = row
	}
	return rows
}
