package agentscale

import "bytes"

// maxStreamBuffer bounds retained garbage while waiting for a frame. A real
// frame is only 18 bytes; retaining more than one frame plus a margin is never
// useful for resynchronisation.
const maxStreamBuffer = FrameLength * 8

// Stream reassembles the D12 continuous byte stream. One Read may contain a
// partial frame, several frames, or both; never assume read boundaries align
// with frames. CKS is a binary byte and may equal STX, CR or any separator, so
// framing uses the fixed length and structural checks rather than delimiters.
type Stream struct {
	buf       []byte
	discarded uint64
}

// Feed appends bytes and emits every complete valid frame. Invalid candidates
// are reported and one byte is dropped before resynchronising; that keeps a
// later real STX discoverable even when the bad candidate began in garbage.
func (s *Stream) Feed(data []byte, emit func(Frame), invalid func(error)) {
	s.buf = append(s.buf, data...)
	for {
		i := bytes.IndexByte(s.buf, stx)
		if i < 0 {
			s.discarded += uint64(len(s.buf))
			s.buf = s.buf[:0]
			return
		}
		if i > 0 {
			s.discarded += uint64(i)
			s.buf = s.buf[i:]
		}
		if len(s.buf) < FrameLength {
			return
		}
		f, err := ParseFrame(s.buf[:FrameLength])
		if err == nil {
			emit(f)
			s.buf = s.buf[FrameLength:]
			continue
		}
		if invalid != nil {
			invalid(err)
		}
		s.buf = s.buf[1:]
	}
}

// Trim bounds the retained buffer after a long run of incomplete input. The
// final FrameLength-1 bytes are preserved because they may contain the start
// of the next frame.
func (s *Stream) Trim() {
	if len(s.buf) <= maxStreamBuffer {
		return
	}
	keep := FrameLength - 1
	s.discarded += uint64(len(s.buf) - keep)
	s.buf = append(s.buf[:0], s.buf[len(s.buf)-keep:]...)
}

func (s *Stream) Discarded() uint64 { return s.discarded }
func (s *Stream) Buffered() int     { return len(s.buf) }
