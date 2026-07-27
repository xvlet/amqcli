package ui

import (
	"amqcli/domain"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *AppModel) fetchBrokerStats() tea.Cmd {
	return func() tea.Msg {
		stats, err := m.uc.GetBrokerStats()
		if err != nil {
			return brokerStatsMsg(domain.BrokerStats{})
		}
		return brokerStatsMsg(stats)
	}
}

func (m *AppModel) fetchBrokerInfo() tea.Cmd {
	return func() tea.Msg {
		info, err := m.uc.GetBrokerInfo()
		if err != nil {
			return brokerInfoMsg("")
		}
		return brokerInfoMsg(info)
	}
}

func (m *AppModel) tickRefresh() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *AppModel) fetchQueues() tea.Cmd {
	return func() tea.Msg {
		q, err := m.uc.GetQueues()
		if err != nil {
			return err
		}
		return q
	}
}

func (m *AppModel) fetchQueueDetail(name string) tea.Cmd {
	return func() tea.Msg {
		qd, err := m.uc.GetQueueDetail(name)
		if err != nil {
			return err
		}
		return qd
	}
}

func (m *AppModel) fetchConnections() tea.Cmd {
	return func() tea.Msg {
		c, err := m.uc.GetConnections()
		if err != nil {
			return err
		}
		return c
	}
}

func (m *AppModel) fetchMessages() tea.Cmd {
	return func() tea.Msg {
		if m.selectedQueue == "" {
			return nil
		}
		limit := 1000
		if len(m.messages) > 900 {
			limit = len(m.messages) + 100
		}
		msgs, err := m.uc.BrowseQueueWithPagination(m.selectedQueue, limit, "")
		if err != nil {
			return err
		}
		return msgs
	}
}

func (m *AppModel) fetchMoreMessages() tea.Cmd {
	return func() tea.Msg {
		if m.selectedQueue == "" || len(m.messages) == 0 {
			return nil
		}

		limit := len(m.messages) + 1000
		selector := ""

		if m.isFiltered && m.filterKeyword != "" {
			target := "JMSCorrelationID"
			if m.filterType == SearchTypeMessageID {
				target = "JMSMessageID"
			}
			selector = fmt.Sprintf("%s LIKE '%%%s%%'", target, m.filterKeyword)
			// For filtered views, we might still need the timestamp selector
			// if we want to bypass the 1000 limit, but let's try increasing cumulative limit first
		}

		msgs, err := m.uc.BrowseQueueWithPagination(m.selectedQueue, limit, selector)
		if err != nil {
			return err
		}
		return moreMessagesMsg(msgs)
	}
}

type moreMessagesMsg []domain.Message
