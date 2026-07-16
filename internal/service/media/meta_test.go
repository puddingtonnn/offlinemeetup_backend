package media

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"

	mp4 "github.com/abema/go-mp4"
)

// memWS is a minimal in-memory io.WriteSeeker used to assemble MP4 fixtures with
// go-mp4's Writer, which backfills box sizes so the fixtures are always well-formed.
type memWS struct {
	buf []byte
	pos int64
}

func (m *memWS) Write(p []byte) (int, error) {
	end := m.pos + int64(len(p))
	if grow := end - int64(len(m.buf)); grow > 0 {
		m.buf = append(m.buf, make([]byte, grow)...)
	}
	copy(m.buf[m.pos:end], p)
	m.pos = end
	return len(p), nil
}

func (m *memWS) Seek(off int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		m.pos = off
	case io.SeekCurrent:
		m.pos += off
	case io.SeekEnd:
		m.pos = int64(len(m.buf)) + off
	}
	return m.pos, nil
}

// trackSpec describes one track to synthesize: its handler kind and, for video,
// integer pixel dimensions (0 for audio).
type trackSpec struct {
	handler       string // "soun" or "vide"
	width, height uint32
}

// buildMP4 assembles a minimal moov (mvhd + one trak per spec) as a real ISOBMFF
// byte stream. duration/timescale feed mvhd; tkhd carries 16.16 fixed-point dims.
func buildMP4(t *testing.T, timescale, duration uint32, tracks []trackSpec) []byte {
	t.Helper()
	ws := &memWS{}
	w := mp4.NewWriter(ws)
	must := func(_ any, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("build mp4 fixture: %v", err)
		}
	}

	must(w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMoov()}))
	must(w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMvhd()}))
	must(mp4.Marshal(w, &mp4.Mvhd{Timescale: timescale, DurationV0: duration}, mp4.Context{}))
	must(w.EndBox())

	for _, tr := range tracks {
		must(w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeTrak()}))

		must(w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeTkhd()}))
		must(mp4.Marshal(w, &mp4.Tkhd{Width: tr.width << 16, Height: tr.height << 16}, mp4.Context{}))
		must(w.EndBox())

		must(w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMdia()}))
		must(w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeHdlr()}))
		var ht [4]byte
		copy(ht[:], tr.handler)
		must(mp4.Marshal(w, &mp4.Hdlr{HandlerType: ht}, mp4.Context{}))
		must(w.EndBox()) // hdlr
		must(w.EndBox()) // mdia

		must(w.EndBox()) // trak
	}

	must(w.EndBox()) // moov
	return ws.buf
}

func TestExtractMeta_MP4AudioOnly(t *testing.T) {
	// A single audio track: sniffs as video/mp4 (shared container) but must be
	// corrected to audio/mp4 with a duration and no dimensions — the voice-note case.
	data := buildMP4(t, 1000, 2000, []trackSpec{{handler: "soun"}})

	m := ExtractMeta("video/mp4", ".mp4", bytes.NewReader(data))

	if m.Mime != "audio/mp4" || m.Ext != ".m4a" {
		t.Fatalf("audio-only mp4 should correct to audio/mp4 .m4a, got %q %q", m.Mime, m.Ext)
	}
	if m.DurationMS == nil || *m.DurationMS != 2000 {
		t.Fatalf("want duration 2000ms, got %v", m.DurationMS)
	}
	if m.Width != nil || m.Height != nil {
		t.Fatalf("audio must carry no dimensions, got %v x %v", m.Width, m.Height)
	}
}

func TestExtractMeta_MP4Video(t *testing.T) {
	// Video + audio: stays video/mp4, dimensions from the video tkhd, duration
	// from mvhd (3000/600 = 5s).
	data := buildMP4(t, 600, 3000, []trackSpec{
		{handler: "vide", width: 1920, height: 1080},
		{handler: "soun"},
	})

	m := ExtractMeta("video/mp4", ".mp4", bytes.NewReader(data))

	if m.Mime != "video/mp4" {
		t.Fatalf("video mp4 must stay video/mp4, got %q", m.Mime)
	}
	if m.Width == nil || *m.Width != 1920 || m.Height == nil || *m.Height != 1080 {
		t.Fatalf("want 1920x1080, got %v x %v", m.Width, m.Height)
	}
	if m.DurationMS == nil || *m.DurationMS != 5000 {
		t.Fatalf("want 5000ms, got %v", m.DurationMS)
	}
}

func TestExtractMeta_Image(t *testing.T) {
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 4, 7))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	m := ExtractMeta("image/png", ".png", bytes.NewReader(buf.Bytes()))

	if m.Width == nil || *m.Width != 4 || m.Height == nil || *m.Height != 7 {
		t.Fatalf("want 4x7, got %v x %v", m.Width, m.Height)
	}
	if m.DurationMS != nil {
		t.Fatalf("image must carry no duration, got %v", m.DurationMS)
	}
}

func TestExtractMeta_Unparseable(t *testing.T) {
	// Bytes accepted by the whitelist as video/mp4 but not valid ISOBMFF: the
	// best-effort pass keeps the baseline mime/ext and returns no metadata.
	m := ExtractMeta("video/mp4", ".mp4", bytes.NewReader([]byte("not a real mp4 file at all")))

	if m.Mime != "video/mp4" || m.Ext != ".mp4" {
		t.Fatalf("baseline mime/ext must survive, got %q %q", m.Mime, m.Ext)
	}
	if m.DurationMS != nil || m.Width != nil || m.Height != nil {
		t.Fatalf("no metadata expected, got %v %v %v", m.DurationMS, m.Width, m.Height)
	}
}
