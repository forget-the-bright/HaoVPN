package transport_test

import (
	"bytes"
	"testing"

	"haovpn/internal/transport"
)

func TestDecoderStickyPackets(t *testing.T) {
	d := transport.NewDecoder()
	f1, err := transport.EncodeFrame(transport.FrameTypeData, []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	f2, err := transport.EncodeFrame(transport.FrameTypeHeartbeat, nil)
	if err != nil {
		t.Fatal(err)
	}
	chunk := append(f1, f2...)
	// Split inside the first frame header (f1 is 10 bytes total: 4 header + 6 body)
	frames, err := d.Feed(chunk[:5])
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 0 {
		t.Fatalf("expected partial, got %d frames", len(frames))
	}
	frames, err = d.Feed(chunk[5:])
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(frames))
	}
	if frames[0].Type != transport.FrameTypeData || !bytes.Equal(frames[0].Payload, []byte("hello")) {
		t.Fatalf("unexpected frame0: %+v", frames[0])
	}
	if frames[1].Type != transport.FrameTypeHeartbeat {
		t.Fatalf("unexpected frame1 type: %d", frames[1].Type)
	}
}

func TestEncodeFrameTooLarge(t *testing.T) {
	_, err := transport.EncodeFrame(transport.FrameTypeData, make([]byte, transport.MaxFrameSize))
	if err == nil {
		t.Fatal("expected error for oversized frame")
	}
}

func TestBufferPool(t *testing.T) {
	b := transport.GetBuffer(1024)
	if len(b) != 1024 {
		t.Fatalf("expected 1024, got %d", len(b))
	}
	transport.PutBuffer(b)
}
