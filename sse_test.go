package flagship

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func TestReadSSEFrame_ParsesEventAndData(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("id: 3\nevent: configuration.updated\ndata: {\"version\":3}\n\n"))

	frame, err := readSSEFrame(r)
	if err != nil {
		t.Fatalf("readSSEFrame() unexpected error: %v", err)
	}
	if frame.event != "configuration.updated" {
		t.Errorf("event = %q, want %q", frame.event, "configuration.updated")
	}
	if frame.data != `{"version":3}` {
		t.Errorf("data = %q, want %q", frame.data, `{"version":3}`)
	}
}

func TestReadSSEFrame_CommentOnlyFrameIsEmpty(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(": heartbeat\n\n"))

	frame, err := readSSEFrame(r)
	if err != nil {
		t.Fatalf("readSSEFrame() unexpected error: %v", err)
	}
	if frame.event != "" || frame.data != "" {
		t.Errorf("frame = %+v, want both fields empty for a comment-only frame", frame)
	}
}

func TestReadSSEFrame_MultipleFramesReadSequentially(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(
		"id: 1\nevent: configuration.updated\ndata: {\"version\":1}\n\n" +
			"id: 2\nevent: configuration.updated\ndata: {\"version\":2}\n\n",
	))

	first, err := readSSEFrame(r)
	if err != nil {
		t.Fatal(err)
	}
	if first.data != `{"version":1}` {
		t.Errorf("first frame data = %q, want %q", first.data, `{"version":1}`)
	}

	second, err := readSSEFrame(r)
	if err != nil {
		t.Fatal(err)
	}
	if second.data != `{"version":2}` {
		t.Errorf("second frame data = %q, want %q", second.data, `{"version":2}`)
	}
}

func TestReadSSEFrame_MultiLineDataIsJoined(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("event: configuration.updated\ndata: line one\ndata: line two\n\n"))

	frame, err := readSSEFrame(r)
	if err != nil {
		t.Fatal(err)
	}
	if frame.data != "line one\nline two" {
		t.Errorf("data = %q, want joined multi-line data", frame.data)
	}
}

func TestReadSSEFrame_UnknownFieldsAreIgnored(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("retry: 5000\nevent: configuration.updated\ndata: {}\n\n"))

	frame, err := readSSEFrame(r)
	if err != nil {
		t.Fatal(err)
	}
	if frame.event != "configuration.updated" || frame.data != "{}" {
		t.Errorf("frame = %+v, unexpected result when an unrecognized field is present", frame)
	}
}

func TestReadSSEFrame_EOFBeforeBlankLineIsAnError(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("event: configuration.updated\ndata: {}"))

	_, err := readSSEFrame(r)
	if err != io.EOF {
		t.Fatalf("readSSEFrame() error = %v, want io.EOF for a stream that ends mid-frame", err)
	}
}
