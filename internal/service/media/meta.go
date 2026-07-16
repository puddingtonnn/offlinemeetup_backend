package media

import (
	"image"
	// Register the standard image decoders so image.DecodeConfig can read
	// dimensions from their headers.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"

	mp4 "github.com/abema/go-mp4"
	// Register the WebP decoder for image.DecodeConfig.
	_ "golang.org/x/image/webp"
)

// Meta is best-effort media metadata layered on top of the Detect result. Mime
// and Ext may be refined (an .m4a voice note sniffs as video/mp4 but is corrected
// to audio/mp4); DurationMS/Width/Height are nil when the format carries no such
// info or the bytes cannot be parsed.
type Meta struct {
	Mime       string
	Ext        string
	DurationMS *int64
	Width      *int
	Height     *int
}

// isobmff is the set of Detect MIME types backed by the ISO Base Media File
// Format container (mp4/m4a/mov/3gp), which we parse for duration + dimensions.
var isobmff = map[string]bool{
	"video/mp4":       true,
	"video/quicktime": true,
	"video/3gpp":      true,
	"audio/mp4":       true,
}

// rasterImage is the set of Detect MIME types measurable via the standard
// image.DecodeConfig decoders (header-only, no full decode).
var rasterImage = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// ExtractMeta enriches the Detect result (mime, ext) with media metadata read
// from r, which must be positioned at the start. It is best-effort: on any parse
// failure it returns the unchanged mime/ext with nil metadata, so a malformed or
// unsupported file never blocks the upload. The caller must rewind r afterwards.
func ExtractMeta(mime, ext string, r io.ReadSeeker) Meta {
	m := Meta{Mime: mime, Ext: ext}
	switch {
	case isobmff[mime]:
		fillISOBMFF(&m, r)
	case rasterImage[mime]:
		fillImage(&m, r)
	}
	return m
}

// fillISOBMFF walks moov for mvhd (duration), hdlr (track kind) and tkhd
// (visual dimensions) in a single pass. It reads via seeks, so a trailing moov
// (common in phone captures) is handled without buffering the whole file.
func fillISOBMFF(m *Meta, r io.ReadSeeker) {
	boxes, err := mp4.ExtractBoxesWithPayload(r, nil, []mp4.BoxPath{
		{mp4.BoxTypeMoov(), mp4.BoxTypeMvhd()},
		{mp4.BoxTypeMoov(), mp4.BoxTypeTrak(), mp4.BoxTypeMdia(), mp4.BoxTypeHdlr()},
		{mp4.BoxTypeMoov(), mp4.BoxTypeTrak(), mp4.BoxTypeTkhd()},
	})
	if err != nil {
		return
	}

	var hasVideo, hasAudio bool
	for _, b := range boxes {
		switch p := b.Payload.(type) {
		case *mp4.Mvhd:
			if p.Timescale != 0 {
				ms := int64(p.GetDuration()) * 1000 / int64(p.Timescale)
				m.DurationMS = &ms
			}
		case *mp4.Hdlr:
			switch string(p.HandlerType[:]) {
			case "vide":
				hasVideo = true
			case "soun":
				hasAudio = true
			}
		case *mp4.Tkhd:
			// tkhd carries visual dimensions as 16.16 fixed-point; audio tracks
			// report 0. Codec-agnostic, so this also covers HEVC video.
			w, h := int(p.GetWidthInt()), int(p.GetHeightInt())
			if w > 0 && h > 0 && m.Width == nil {
				m.Width, m.Height = &w, &h
				hasVideo = true
			}
		}
	}

	// A file that sniffs as video/mp4 but carries only an audio track is a voice
	// note (.m4a) — correct the MIME + extension so the client renders it as audio.
	if m.Mime == "video/mp4" && hasAudio && !hasVideo {
		m.Mime = "audio/mp4"
		if e, ok := allowed["audio/mp4"]; ok {
			m.Ext = e
		}
	}
}

// fillImage reads only the image header to learn its pixel dimensions.
func fillImage(m *Meta, r io.ReadSeeker) {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return
	}
	w, h := cfg.Width, cfg.Height
	m.Width, m.Height = &w, &h
}
