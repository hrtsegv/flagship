package flagship

import (
	"bufio"
	"strings"
)

// sseFrame is one parsed Server-Sent Events frame: its event type and
// data. A comment-only frame (a heartbeat, or any line starting with ":")
// comes back with both fields empty, which callers treat as a no-op.
//
// The id field is intentionally not parsed: the SDK doesn't need to track
// it between frames, only send something back as Last-Event-ID on the
// next connection -- and the client's own currently-applied version is
// exactly that, and more precise than the last raw id read off the wire
// when a version-gap recovery happened mid-stream (see connectAndStream).
type sseFrame struct {
	event string
	data  string
}

// readSSEFrame reads one Server-Sent Events frame: a run of "field: value"
// lines terminated by a blank line.
func readSSEFrame(r *bufio.Reader) (sseFrame, error) {
	var frame sseFrame
	var dataLines []string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return sseFrame{}, err
		}
		line = strings.TrimRight(line, "\r\n")

		switch {
		case line == "":
			frame.data = strings.Join(dataLines, "\n")
			return frame, nil
		case strings.HasPrefix(line, ":"):
			// Comment or heartbeat; ignore.
		case strings.HasPrefix(line, "event:"):
			frame.event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
}
