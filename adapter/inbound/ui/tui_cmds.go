package ui

import (
	"fmt"
	"github.com/xvlet/amqcli/domain"
	"os"
	"sort"
	"strings"
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

type snapshotSavedMsg struct {
	err      error
	filename string
}

func (m *AppModel) takeFullSnapshot() tea.Cmd {
	return func() tea.Msg {
		filename := fmt.Sprintf("amqcli_full_snapshot_%d.txt", time.Now().Unix())

		jvmStats, _ := m.uc.GetJVMStats()
		brokerStats, _ := m.uc.GetBrokerStats()
		brokerInfo, _ := m.uc.GetBrokerInfo()
		topics, _ := m.uc.GetTopics()
		queues, _ := m.uc.GetQueues()
		connections, _ := m.uc.GetConnections()
		allConsumers, _ := m.uc.GetAllConsumers()

		sort.Slice(queues, func(i, j int) bool { return strings.ToLower(queues[i].Name) < strings.ToLower(queues[j].Name) })
		sort.Slice(topics, func(i, j int) bool { return strings.ToLower(topics[i].Name) < strings.ToLower(topics[j].Name) })
		sort.Slice(connections, func(i, j int) bool {
			return strings.ToLower(connections[i].Name) < strings.ToLower(connections[j].Name)
		})
		sort.Slice(allConsumers, func(i, j int) bool {
			return strings.ToLower(allConsumers[i].ClientID) < strings.ToLower(allConsumers[j].ClientID)
		})

		// #nosec G304 -- filename is auto-generated locally
		f, err := os.Create(filename)
		if err != nil {
			return snapshotSavedMsg{err: err, filename: filename}
		}
		defer func() { _ = f.Close() }()

		doubleLine := strings.Repeat("═", 115)
		singleLine := strings.Repeat("─", 115)
		boxLine := strings.Repeat("─", 113)

		_, _ = fmt.Fprintf(f, "╭%s╮\n", boxLine)
		title := "A M Q C L I   D I A G N O S T I C   S N A P S H O T"
		_, _ = fmt.Fprintf(f, "│%31s%s%31s│\n", "", title, "")
		_, _ = fmt.Fprintf(f, "├%s┤\n", boxLine)
		_, _ = fmt.Fprintf(f, "│ Date & Time     : %-93s │\n", time.Now().Format("2006-01-02 15:04:05 KST"))
		_, _ = fmt.Fprintf(f, "│ amqcli Version  : %-93s │\n", "v0.1.0")
		_, _ = fmt.Fprintf(f, "│ Target Broker   : %-93s │\n", m.host)
		_, _ = fmt.Fprintf(f, "│ AMQ Version     : %-93s │\n", strings.TrimSpace(brokerInfo))
		_, _ = fmt.Fprintf(f, "│ Broker Uptime   : %-93s │\n", brokerStats.Uptime)
		_, _ = fmt.Fprintf(f, "╰%s╯\n\n", boxLine)

		_, _ = fmt.Fprintf(f, "[ 1. System & JVM Health ]\n")
		_, _ = fmt.Fprintln(f, singleLine)

		heapPercent := 0.0
		if jvmStats.HeapMemoryMax > 0 {
			heapPercent = (float64(jvmStats.HeapMemoryUsed) / float64(jvmStats.HeapMemoryMax)) * 100.0
		}

		_, _ = fmt.Fprintf(f, "Heap Memory     : %s %s / %s bytes (%.1f%%)\n",
			drawProgressBar(heapPercent, 20),
			formatWithCommas(jvmStats.HeapMemoryUsed),
			formatWithCommas(jvmStats.HeapMemoryMax),
			heapPercent)
		_, _ = fmt.Fprintf(f, "Non-Heap Memory : %s bytes\n", formatWithCommas(jvmStats.NonHeapMemoryUsed))
		_, _ = fmt.Fprintf(f, "Active Threads  : %s (Peak: %s)\n\n", formatWithCommas(int64(jvmStats.ThreadCount)), formatWithCommas(int64(jvmStats.PeakThreadCount)))

		_, _ = fmt.Fprintf(f, "[ 2. Broker Usage & Global Metrics ]\n")
		_, _ = fmt.Fprintln(f, singleLine)
		_, _ = fmt.Fprintf(f, "Store Limit     : %s %d%%\n", drawProgressBar(float64(brokerStats.StorePercentUsage), 20), brokerStats.StorePercentUsage)
		_, _ = fmt.Fprintf(f, "Memory Limit    : %s %d%%\n", drawProgressBar(float64(brokerStats.MemoryPercentUsage), 20), brokerStats.MemoryPercentUsage)
		_, _ = fmt.Fprintf(f, "Temp Limit      : %s %d%%\n", drawProgressBar(float64(brokerStats.TempPercentUsage), 20), brokerStats.TempPercentUsage)
		_, _ = fmt.Fprintf(f, "Total Enqueues  : %s\n", formatWithCommas(brokerStats.TotalEnqueueCount))
		_, _ = fmt.Fprintf(f, "Total Dequeues  : %s\n", formatWithCommas(brokerStats.TotalDequeueCount))
		_, _ = fmt.Fprintf(f, "Total Consumers : %s\n", formatWithCommas(brokerStats.TotalConsumerCount))
		_, _ = fmt.Fprintf(f, "Total Producers : %s\n\n", formatWithCommas(brokerStats.TotalProducerCount))

		_, _ = fmt.Fprintf(f, "[ 3. Queues Statistics ]\n")
		_, _ = fmt.Fprintln(f, doubleLine)
		_, _ = fmt.Fprintf(f, " %-44s %9s %10s %12s %12s %10s %10s\n", "Name", "Pending", "Consumers", "Enqueued", "Dequeued", "InFlight", "Expired")
		_, _ = fmt.Fprintln(f, singleLine)
		for _, q := range queues {
			_, _ = fmt.Fprintf(f, " %-44s %9s %10s %12s %12s %10s %10s\n",
				truncateStr(q.Name, 44),
				formatWithCommas(q.Pending),
				formatWithCommas(q.Consumers),
				formatWithCommas(q.Enqueued),
				formatWithCommas(q.Dequeued),
				formatWithCommas(q.InFlightCount),
				formatWithCommas(q.ExpiredCount))
		}
		_, _ = fmt.Fprintln(f, singleLine)
		_, _ = fmt.Fprintln(f, "")

		_, _ = fmt.Fprintf(f, "[ 4. Topics Statistics ]\n")
		_, _ = fmt.Fprintln(f, doubleLine)
		_, _ = fmt.Fprintf(f, " %-68s %10s %14s %14s\n", "Name", "Consumers", "Enqueued", "Dequeued")
		_, _ = fmt.Fprintln(f, singleLine)
		for _, t := range topics {
			_, _ = fmt.Fprintf(f, " %-68s %10s %14s %14s\n",
				truncateStr(t.Name, 68),
				formatWithCommas(t.ConsumerCount),
				formatWithCommas(t.EnqueueCount),
				formatWithCommas(t.DequeueCount))
		}
		_, _ = fmt.Fprintln(f, singleLine)
		_, _ = fmt.Fprintln(f, "")

		_, _ = fmt.Fprintf(f, "[ 5. Consumers (All Queues) ]\n")
		_, _ = fmt.Fprintln(f, doubleLine)
		_, _ = fmt.Fprintf(f, " %-28s %-20s %-16s %6s %8s %10s %8s %8s\n", "Client ID", "Destination", "Remote Address", "PID", "Uptime", "Dispatched", "Pending", "Prefetch")
		_, _ = fmt.Fprintln(f, singleLine)
		for _, c := range allConsumers {
			_, _ = fmt.Fprintf(f, " %-28s %-20s %-16s %6s %8s %10s %8s %8s\n",
				truncateStr(c.ClientID, 28),
				truncateStr(c.DestinationName, 20),
				truncateStr(stripProtocol(c.RemoteAddress), 16),
				truncateStr(c.PID, 6),
				truncateStr(c.Uptime, 8),
				formatWithCommas(c.DispatchedQueueSize),
				formatWithCommas(c.PendingQueueSize),
				formatWithCommas(int64(c.PrefetchSize)))
		}
		_, _ = fmt.Fprintln(f, singleLine)
		_, _ = fmt.Fprintln(f, "")

		_, _ = fmt.Fprintf(f, "[ 6. Active Connections ]\n")
		_, _ = fmt.Fprintln(f, doubleLine)
		_, _ = fmt.Fprintf(f, " %-58s %-30s %-9s %-9s\n", "Name", "Remote Address", "Active", "Slow")
		_, _ = fmt.Fprintln(f, singleLine)
		for _, c := range connections {
			_, _ = fmt.Fprintf(f, " %-58s %-30s %-9t %-9t\n",
				truncateStr(c.Name, 58),
				truncateStr(c.RemoteAddress, 30),
				c.Active,
				c.Slow)
		}
		_, _ = fmt.Fprintln(f, singleLine)
		_, _ = fmt.Fprintln(f, "")

		return snapshotSavedMsg{err: nil, filename: filename}
	}
}

func (m *AppModel) takeQueueSnapshot() tea.Cmd {
	return func() tea.Msg {
		timestamp := time.Now().Format("20060102_150405")
		safeQueueName := strings.ReplaceAll(m.selectedQueue, "/", "_")
		safeQueueName = strings.ReplaceAll(safeQueueName, " ", "_")
		filename := fmt.Sprintf("amqcli_snapshot_%s_%s.txt", safeQueueName, timestamp)

		jvmStats, _ := m.uc.GetJVMStats()
		brokerStats, _ := m.uc.GetBrokerStats()
		brokerInfo, _ := m.uc.GetBrokerInfo()

		qd := m.selectedQueueDetail
		if qd == nil {
			return snapshotSavedMsg{err: fmt.Errorf("queue details not loaded")}
		}

		// #nosec G304 -- filename is auto-generated locally
		f, err := os.Create(filename)
		if err != nil {
			return snapshotSavedMsg{err: err, filename: filename}
		}
		defer func() { _ = f.Close() }()

		doubleLine := strings.Repeat("═", 115)
		singleLine := strings.Repeat("─", 115)
		boxLine := strings.Repeat("─", 113)

		_, _ = fmt.Fprintf(f, "╭%s╮\n", boxLine)
		title := fmt.Sprintf("Q U E U E   S N A P S H O T : %s", truncateStr(m.selectedQueue, 30))
		padLeft := (113 - len(title)) / 2
		padRight := 113 - len(title) - padLeft
		_, _ = fmt.Fprintf(f, "│%s%s%s│\n", strings.Repeat(" ", padLeft), title, strings.Repeat(" ", padRight))
		_, _ = fmt.Fprintf(f, "├%s┤\n", boxLine)
		_, _ = fmt.Fprintf(f, "│ Date & Time     : %-93s │\n", time.Now().Format("2006-01-02 15:04:05 KST"))
		_, _ = fmt.Fprintf(f, "│ amqcli Version  : %-93s │\n", "v0.1.0")
		_, _ = fmt.Fprintf(f, "│ Target Broker   : %-93s │\n", m.host)
		_, _ = fmt.Fprintf(f, "│ AMQ Version     : %-93s │\n", strings.TrimSpace(brokerInfo))
		_, _ = fmt.Fprintf(f, "│ Broker Uptime   : %-93s │\n", brokerStats.Uptime)
		_, _ = fmt.Fprintf(f, "╰%s╯\n\n", boxLine)

		_, _ = fmt.Fprintf(f, "[ 1. System & JVM Health ]\n")
		_, _ = fmt.Fprintln(f, singleLine)

		heapPercent := 0.0
		if jvmStats.HeapMemoryMax > 0 {
			heapPercent = (float64(jvmStats.HeapMemoryUsed) / float64(jvmStats.HeapMemoryMax)) * 100.0
		}

		_, _ = fmt.Fprintf(f, "Heap Memory     : %s %s / %s bytes (%.1f%%)\n",
			drawProgressBar(heapPercent, 20),
			formatWithCommas(jvmStats.HeapMemoryUsed),
			formatWithCommas(jvmStats.HeapMemoryMax),
			heapPercent)
		_, _ = fmt.Fprintf(f, "Non-Heap Memory : %s bytes\n", formatWithCommas(jvmStats.NonHeapMemoryUsed))
		_, _ = fmt.Fprintf(f, "Active Threads  : %s (Peak: %s)\n\n", formatWithCommas(int64(jvmStats.ThreadCount)), formatWithCommas(int64(jvmStats.PeakThreadCount)))

		_, _ = fmt.Fprintf(f, "[ 2. Target Queue Metrics ]\n")
		_, _ = fmt.Fprintln(f, doubleLine)
		_, _ = fmt.Fprintf(f, " %-44s %9s %10s %12s %12s %10s %10s\n", "Name", "Pending", "Consumers", "Enqueued", "Dequeued", "InFlight", "Expired")
		_, _ = fmt.Fprintln(f, singleLine)
		_, _ = fmt.Fprintf(f, " %-44s %9s %10s %12s %12s %10s %10s\n",
			truncateStr(qd.Name, 44),
			formatWithCommas(qd.QueueSize),
			formatWithCommas(qd.ConsumerCount),
			formatWithCommas(qd.EnqueueCount),
			formatWithCommas(qd.DequeueCount),
			formatWithCommas(qd.InFlightCount),
			formatWithCommas(qd.ExpiredCount))
		_, _ = fmt.Fprintln(f, singleLine)

		if qd.MemoryPercentUsage > 0 {
			_, _ = fmt.Fprintf(f, " Memory Usage    : %s %d%%\n", drawProgressBar(float64(qd.MemoryPercentUsage), 20), qd.MemoryPercentUsage)
		}
		_, _ = fmt.Fprintln(f, "")

		_, _ = fmt.Fprintf(f, "[ 3. Connected Consumers ]\n")
		_, _ = fmt.Fprintln(f, doubleLine)
		_, _ = fmt.Fprintf(f, " %-48s %-25s %6s %10s %9s %9s\n", "Client ID", "Remote Address", "PID", "Uptime", "Dispatched", "Pending")
		_, _ = fmt.Fprintln(f, singleLine)

		sort.Slice(qd.Consumers, func(i, j int) bool {
			return strings.ToLower(qd.Consumers[i].ClientID) < strings.ToLower(qd.Consumers[j].ClientID)
		})

		for _, c := range qd.Consumers {
			_, _ = fmt.Fprintf(f, " %-48s %-25s %6s %10s %9s %9s\n",
				truncateStr(c.ClientID, 48),
				truncateStr(stripProtocol(c.RemoteAddress), 25),
				truncateStr(c.PID, 6),
				truncateStr(c.Uptime, 10),
				formatWithCommas(c.DispatchedQueueSize),
				formatWithCommas(c.PendingQueueSize))
		}
		if len(qd.Consumers) == 0 {
			_, _ = fmt.Fprintf(f, " (No active consumers connected to this queue)\n")
		}
		_, _ = fmt.Fprintln(f, singleLine)
		_, _ = fmt.Fprintln(f, "")

		_, _ = fmt.Fprintf(f, "[ 4. Messages Preview (Top 20) ]\n")
		_, _ = fmt.Fprintln(f, doubleLine)
		_, _ = fmt.Fprintf(f, " %-40s %-30s %-38s\n", "Message ID", "Correlation ID", "Timestamp")
		_, _ = fmt.Fprintln(f, singleLine)

		// Fetch fresh messages for this queue — m.messages may belong to a different queue
		queueMsgs, _ := m.uc.BrowseQueueWithPagination(m.selectedQueue, 20, "")
		limit := len(queueMsgs)
		if limit > 20 {
			limit = 20
		}

		for i := 0; i < limit; i++ {
			msg := queueMsgs[i]
			_, _ = fmt.Fprintf(f, " %-40s %-30s %-38s\n",
				truncateStr(msg.MessageID, 40),
				truncateStr(msg.CorrelationID, 30),
				truncateStr(msg.Timestamp.Format("2006-01-02 15:04:05"), 38))
		}
		if len(queueMsgs) == 0 {
			_, _ = fmt.Fprintf(f, " (Queue is empty)\n")
		} else if len(queueMsgs) > 20 {
			_, _ = fmt.Fprintf(f, " ... (showing 20 of %d messages)\n", len(queueMsgs))
		}
		_, _ = fmt.Fprintln(f, singleLine)
		_, _ = fmt.Fprintln(f, "")

		return snapshotSavedMsg{err: nil, filename: filename}
	}
}
