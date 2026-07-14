// Package media resolves the MIME type of an uploaded file from its real bytes
// and gates it to an allowed media whitelist (photo / video / audio). The stored
// Content-Type and object-key extension are always derived here from the sniffed
// bytes, never from the client filename — an attacker must not be able to inject
// an active-content extension (.html/.svg/.js) into a public object key or store
// non-media bytes disguised as media.
package media

import (
	"bytes"
	"net/http"
	"strings"
)

// allowed maps a canonical media MIME type to its stored file extension.
var allowed = map[string]string{
	// images
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
	"image/heic": ".heic",
	"image/heif": ".heif",
	// video
	"video/mp4":       ".mp4",
	"video/quicktime": ".mov",
	"video/webm":      ".webm",
	"video/3gpp":      ".3gp",
	"video/x-msvideo": ".avi",
	// audio
	"audio/mpeg": ".mp3",
	"audio/mp4":  ".m4a",
	"audio/aac":  ".aac",
	"audio/ogg":  ".ogg",
	"audio/wav":  ".wav",
	"audio/flac": ".flac",
}

// sniffAlias maps names http.DetectContentType uses to our canonical names.
var sniffAlias = map[string]string{
	"audio/wave":      "audio/wav",
	"application/ogg": "audio/ogg",
	"video/avi":       "video/x-msvideo",
}

// Detect resolves the media MIME type + stored extension from a file's first
// bytes. ok is false for anything not in the allowed media set (documents, svg,
// executables, unknown containers).
func Detect(head []byte) (mime, ext string, ok bool) {
	// 1) Go's built-in sniffer: reliable for images and some a/v.
	if m := normalize(http.DetectContentType(head)); m != "" {
		if e, found := allowed[m]; found {
			return m, e, true
		}
	}
	// 2) Supplementary magic-byte checks for media Go's sniffer misses
	//    (.mov / .m4a / .heic / .mkv / .flac / raw aac).
	if m := detectMagic(head); m != "" {
		if e, found := allowed[m]; found {
			return m, e, true
		}
	}
	return "", "", false
}

// normalize strips any "; charset=..." suffix and maps sniffer names to canonical.
func normalize(m string) string {
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	if alias, ok := sniffAlias[m]; ok {
		return alias
	}
	return m
}

// detectMagic covers media containers the standard library does not sniff.
func detectMagic(b []byte) string {
	// ISO Base Media File Format: [4-byte box size]"ftyp"[major brand].
	if len(b) >= 12 && bytes.Equal(b[4:8], []byte("ftyp")) {
		return byBrand(string(b[8:12]))
	}
	// Matroska / WebM: EBML header. Both are canonicalized to video/webm.
	if len(b) >= 4 && bytes.Equal(b[:4], []byte{0x1A, 0x45, 0xDF, 0xA3}) {
		return "video/webm"
	}
	// Ogg container (vorbis/opus).
	if len(b) >= 4 && bytes.Equal(b[:4], []byte("OggS")) {
		return "audio/ogg"
	}
	// FLAC.
	if len(b) >= 4 && bytes.Equal(b[:4], []byte("fLaC")) {
		return "audio/flac"
	}
	// RIFF containers.
	if len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) {
		switch string(b[8:12]) {
		case "WAVE":
			return "audio/wav"
		case "AVI ":
			return "video/x-msvideo"
		}
	}
	// MP3 with an ID3 tag.
	if len(b) >= 3 && bytes.Equal(b[:3], []byte("ID3")) {
		return "audio/mpeg"
	}
	// Raw AAC (ADTS syncword 0xFFFx, layer 00) — check before the broader MP3 sync.
	if len(b) >= 2 && b[0] == 0xFF && (b[1]&0xF6) == 0xF0 {
		return "audio/aac"
	}
	// MPEG audio frame sync (11 bits set).
	if len(b) >= 2 && b[0] == 0xFF && (b[1]&0xE0) == 0xE0 {
		return "audio/mpeg"
	}
	return ""
}

// byBrand maps an ISOBMFF major brand to a canonical MIME type. Unknown brands
// fall back to video/mp4 (the file is still ISOBMFF media).
func byBrand(brand string) string {
	switch brand {
	case "qt  ":
		return "video/quicktime"
	case "M4A ", "M4B ":
		return "audio/mp4"
	case "heic", "heix", "hevc", "heim", "heis":
		return "image/heic"
	case "mif1", "msf1", "heif":
		return "image/heif"
	case "3gp4", "3gp5", "3gp6", "3g2a":
		return "video/3gpp"
	default:
		return "video/mp4"
	}
}
