package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/nxadm/tail"
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

type logLine struct {
	Timestamp time.Time // Timestamp for line
	LineType  string    // Type of line. Interpreted by UI to determine actions
	Result    string    // Present for cached, reply, blocked
	Domain    string    // Present for cached, reply, blocked, query[*], forwarded
	Requester string    // Present for query[*]
	Upstream  string    // Present for forwarded
	Line      string    // Store full line text for UI purposes
}

type filterFunc func(logLine) bool

func filterLogLine(lines []logLine, f filterFunc) []logLine {
	// filterLogLine filters a slice of logLines using f to determine inclusion
	// f is type func(logLine) bool
	var filtered []logLine
	for i := range lines {
		if f(lines[i]) {
			filtered = append(filtered, lines[i])
		}
	}
	return filtered
}

func textSearchLogLine(s string) filterFunc {
	// textSearchLogLine is a helper function to generate a filterFunc
	// that searches for text s anywhere in a logLine
	return func(ll logLine) bool {
		return strings.Contains(ll.Line, s)
	}
}

func setTable(t *tview.Table, logLines []logLine) {
	// setTable sets the value of the main table based on a slice of logLines
	t.Clear()
	rows := len(logLines)
	for r := 1; r <= rows; r++ {
		t.SetCell(r, 0,
			tview.NewTableCell(logLines[rows-r].Line).
				SetTextColor(tcell.ColorWhite).
				SetAlign(tview.AlignLeft))
	}
}

func unmarshalLogLine(line string) logLine {
	// unmarshalLogLine unmarshals a log line to the struct logLine
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

	return logLine{
		Timestamp: timestamp,
		LineType:  lineType,
		Result:    result,
		Domain:    domain,
		Requester: requester,
		Upstream:  upstream,
		Line:      sanitizedLine,
	}
}

func main() {
	app := tview.NewApplication()

	table := tview.NewTable().SetBorders(false) // table element
	table.SetBorder(true).SetTitle("[yellow]PiholeLog")

	// detailPane shows the details of a given entry
	// and allows filter setting
	detailPane := tview.NewList()
	detailPane.SetBorder(true).SetTitle("[yellow]Details")

	// filterIndicator is a text indicator of the current filter state
	filterIndicator := tview.NewTextView()
	filterIndicator.SetTitle("[yellow]Filter Status:")
	filterIndicator.SetText("None").SetBorder(true)

	// filterField is the input box for arbitrary text search
	filterField := tview.NewInputField().SetFieldWidth(30).SetFieldBackgroundColor(tcell.ColorBlack)
	filterField.SetTitle("[yellow]Filter string:").SetBorder(true)

	// set up flexbox layout with larger table than detail pane
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(filterField, 3, 1, false).
		AddItem(filterIndicator, 3, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(detailPane, 0, 1, false).
			AddItem(table, 0, 2, true), 0, 1, true,
		)

	// helpModal is a modal that displays controls help
	helpModal := tview.NewModal()
	helpModal.SetText("Hotkeys:\n" +
		"* f: enter search string\n" +
		"* r: reload the log file\n" +
		"* h: bring up this help pane\n" +
		"* ESC: clear current filter state\n").
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			app.SetRoot(flex, false)
		})

	// begin loading log file
	tf, tailError := tail.TailFile("/var/log/pihole.log", tail.Config{})

	if tailError != nil {
		panic(tailError)
	}

	// with this configuration, Tail will spit out the whole file and then stop
	// after we get the initial file parsed, we can proceed to load the state of the initial table
	// once that is complete, we can enter the main loop and update if I choose to implement that feature
	var logLines []logLine
	for line := range tf.Lines {
		logLine := unmarshalLogLine(line.Text)
		logLines = append(logLines, logLine)
	}

	// the main table for viewing the unedited log lines will be just one column
	rows := len(logLines)
	setTable(table, logLines)

	// set up input handling
	app = app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// controls for the whole app:
		// * f key: set focus to input field for arbitrary string search
		// * r key: reload the log file
		// * h key: help modal
		if event.Key() == tcell.KeyRune {
			if app.GetFocus() != filterField {
				switch event.Rune() {
				case 'f':
					app.SetFocus(filterField)
					return nil
				case 'r':
					tf, tailError = tail.TailFile("/var/log/pihole.log", tail.Config{})
					logLines = make([]logLine, 0) // clear out logLines

					if tailError != nil {
						panic(tailError)
					}

					for line := range tf.Lines {
						logLine := unmarshalLogLine(line.Text)
						logLines = append(logLines, logLine)
					}

					rows = len(logLines)
					setTable(table, logLines)
					return nil
				case 'h':
					app.SetRoot(helpModal, false)
					return nil
				}
			}
		}
		return event // pass any other keys along
	})

	filterField.SetDoneFunc(func(key tcell.Key) {
		searchKey := filterField.GetText()
		filterIndicator.SetText(fmt.Sprintf("Text search: %v", searchKey))
		filtered := filterLogLine(logLines, textSearchLogLine(searchKey))
		setTable(table, filtered)
		app.SetFocus(table)
	})

	// tcell constants and types used for input handling
	// * table.Select sets the selected cell
	// * table.SetFixed sets how many rows and columns are always displayed
	// * table.SetDoneFunc sets the function called when Esc and other keys are pressed
	// * table.SetSelectedFunc sets the function called when a cell is selected
	// * SetSelectable determines whether rows, columns, or cells can be selected
	table.Select(0, 0).SetFixed(1, 1).SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			setTable(table, logLines)
			filterIndicator.SetText("None")
			filterField.SetText("")
		}
		if key == tcell.KeyEnter {
			table.SetSelectable(true, true)
		}
	}).SetSelectedFunc(func(row int, column int) {
		// when a row is selected, we fill in the details pane with the relevant information from the row
		// move the focus to the details pane
		// and set up callbacks for filtering operations
		// this is working at the moment, but I think I need to create some higher level utilities
		// to enable filtering more general (e.g. so the user can just type in something to filter)

		detailPane.Clear()
		selectedLine := logLines[rows-row]

		// ESC key when in the details pane will clear out the applied filter and return focus to the table
		detailPane.SetDoneFunc(func() {
			detailPane.Clear()
			setTable(table, logLines)
			filterIndicator.SetText("None")
			app.SetFocus(table)
		})

		detailPane.AddItem("Timestamp: "+selectedLine.Timestamp.Format(time.Stamp), "", 0, func() {})

		// when an applicable detailPane list item is selected, filter the main table
		detailPane.AddItem("Entry type: "+selectedLine.LineType, "", 0, func() {
			table.Clear()

			// LineType may have a tview-escaped closing square bracket, so we have to undo that
			filterIndicator.SetText(fmt.Sprintf("LineType: %v",
				strings.ReplaceAll(selectedLine.LineType, "[]", "]")))

			filtered := filterLogLine(logLines, func(ll logLine) bool {
				return ll.LineType == selectedLine.LineType
			})

			setTable(table, filtered)
		})

		if selectedLine.Result != "" {
			detailPane.AddItem("Result: "+selectedLine.Result, "", 0, func() {
				table.Clear()

				filterIndicator.SetText(fmt.Sprintf("Result: %v", selectedLine.Result))

				filtered := filterLogLine(logLines, func(ll logLine) bool {
					return ll.Result == selectedLine.Result
				})

				setTable(table, filtered)
			})
		}

		if selectedLine.Domain != "" {
			detailPane.AddItem("Domain: "+selectedLine.Domain, "", 0, func() {
				table.Clear()

				filterIndicator.SetText(fmt.Sprintf("Domain: %v", selectedLine.Domain))

				filtered := filterLogLine(logLines, func(ll logLine) bool {
					return ll.Domain == selectedLine.Domain
				})

				setTable(table, filtered)
			})
		}

		if selectedLine.Requester != "" {
			detailPane.AddItem("Requester: "+selectedLine.Requester, "", 0, func() {
				table.Clear()

				filterIndicator.SetText(fmt.Sprintf("Requester: %v", selectedLine.Requester))

				filtered := filterLogLine(logLines, func(ll logLine) bool {
					return ll.Requester == selectedLine.Requester
				})

				setTable(table, filtered)
			})
		}

		if selectedLine.Upstream != "" {
			detailPane.AddItem("Upstream: "+selectedLine.Upstream, "", 0, func() {
				table.Clear()

				filterIndicator.SetText(fmt.Sprintf("Upstream: %v", selectedLine.Upstream))

				filtered := filterLogLine(logLines, func(ll logLine) bool {
					return ll.Upstream == selectedLine.Upstream
				})

				setTable(table, filtered)
			})
		}
		app.SetFocus(detailPane)
	})

	if err := app.SetRoot(flex, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}
