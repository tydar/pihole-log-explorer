package main

import (
	"strings"
	"time"
)

// Interpreted log line types
// Note: [ required before a ] to ensure tview doesn't interpret strings enclosed in [] as style.
const (
	Blocked   = "gravity blocked"
	Read      = "read"
	AAAA      = "query[AAAA[]"
	A         = "query[A[]"
	Ptr       = "query[PTR[]"
	Cached    = "cached"
	Forwarded = "forwarded"
	Reply     = "reply"
	Unknown   = "unknown"
)

type LogLine struct {
	Timestamp time.Time // Timestamp for line
	LineType  string    // Type of line. Interpreted by UI to determine actions
	Result    string    // Present for cached, reply, blocked
	Domain    string    // Present for cached, reply, blocked, query[*], forwarded
	Requester string    // Present for query[*]
	Upstream  string    // Present for forwarded
	Line      string    // Store full line text for UI purposes
}

func UnmarshalLogLine(line string) LogLine {
	// unmarshalLogLine unmarshals a log line to the struct LogLine
	tokens := strings.Fields(line)

	// parse time
	// since time.Parse needs an exact string for parsing
	// we have to reconstruct the timestamp from the tokens
	timeStr := tokens[0] + " " + tokens[1] + " " + tokens[2]
	timestamp, timeError := time.Parse(time.Stamp, timeStr)
	if timeError != nil {
		panic(timeError)
	}

	// parse out LineType
	var lineType string

	switch tokens[4] {
	case "gravity":
		lineType = Blocked
	case "read":
		lineType = Read
	case "query[AAAA]":
		lineType = AAAA
	case "query[A]":
		lineType = A
	case "query[PTR]":
		lineType = Ptr
	case "cached":
		lineType = Cached
	case "forwarded":
		lineType = Forwarded
	case "reply":
		lineType = Reply
	default:
		lineType = Unknown
	}

	// parse out result for cached, reply, and blocked
	result := ""
	if lineType == Cached || lineType == Reply {
		result = tokens[7]
	} else if lineType == Blocked { // since blocked lines have "gravity blocked", indicies for later values are moved up by one
		result = tokens[8]
	}

	// parse out Domain for cached, reply, blocked, query[*], and forwarded
	domain := ""
	if lineType == Blocked {
		domain = tokens[6]
	} else if lineType == Cached || lineType == Reply || lineType == AAAA ||
		lineType == A || lineType == Ptr || lineType == Forwarded {
		domain = tokens[5]
	}

	// parse out Requester from query[*] lines
	requester := ""
	if lineType == A || lineType == AAAA || lineType == Ptr {
		requester = tokens[7]
	}

	// parse out upstream from forwarded replies
	upstream := ""
	if lineType == Forwarded {
		upstream = tokens[7]
	}

	// ensure all closing square brackets are escaped so tview displays them properly
	sanitizedLine := strings.ReplaceAll(line, "]", "[]")

	return LogLine{
		Timestamp: timestamp,
		LineType:  lineType,
		Result:    result,
		Domain:    domain,
		Requester: requester,
		Upstream:  upstream,
		Line:      sanitizedLine,
	}
}

type FilterFunc func(LogLine) bool

func FilterLogLine(lines []LogLine, f FilterFunc) []LogLine {
	// filterLogLine filters a slice of LogLines using f to determine inclusion
	// f is type func(LogLine) bool
	var filtered []LogLine
	for i := range lines {
		if f(lines[i]) {
			filtered = append(filtered, lines[i])
		}
	}
	return filtered
}

func TextSearchLogLine(s string) FilterFunc {
	// textSearchLogLine is a helper function to generate a FilterFunc
	// that searches for text s anywhere in a LogLine
	return func(ll LogLine) bool {
		return strings.Contains(ll.Line, s)
	}
}
