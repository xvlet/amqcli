package main

import (
	"fmt"
	"regexp"
)

var pidRegex = regexp.MustCompile(`(?:^|[^0-9])([0-9]{4,8})(?:$|[^0-9])`)

func extractPID(clientID, connectionID string) string {
	if match := pidRegex.FindStringSubmatch(clientID); len(match) > 1 {
		return match[1]
	}
	if match := pidRegex.FindStringSubmatch(connectionID); len(match) > 1 {
		return match[1]
	}
	return "-"
}

func main() {
	tests := []struct {
		cid  string
		conn string
		want string
	}{
		{"bridge-client-8390506-12345", "conn-1", "8390506"},
		{"ID:myhost-54321-1629873421-1", "conn-1", "54321"},
		{"no-pid-here", "ID:other-9999-0", "9999"},
		{"just-text", "no-digits", "-"},
	}

	for _, tt := range tests {
		got := extractPID(tt.cid, tt.conn)
		fmt.Printf("CID: %-30s | CONN: %-15s | GOT: %s | WANT: %s\n", tt.cid, tt.conn, got, tt.want)
	}
}
