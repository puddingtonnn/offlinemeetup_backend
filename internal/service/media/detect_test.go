package media

import "testing"

func TestDetect(t *testing.T) {
	cases := []struct {
		name     string
		head     []byte
		wantMime string
		wantExt  string
		wantOK   bool
	}{
		{"png", []byte("\x89PNG\r\n\x1a\n" + "payload"), "image/png", ".png", true},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 'J', 'F', 'I', 'F'}, "image/jpeg", ".jpg", true},
		{"gif", []byte("GIF89a" + "payload"), "image/gif", ".gif", true},
		{"mp4 isom", append([]byte{0, 0, 0, 0x20}, []byte("ftypisom....")...), "video/mp4", ".mp4", true},
		{"mov", append([]byte{0, 0, 0, 0x14}, []byte("ftypqt  ....")...), "video/quicktime", ".mov", true},
		{"m4a", append([]byte{0, 0, 0, 0x20}, []byte("ftypM4A ....")...), "audio/mp4", ".m4a", true},
		{"heic", append([]byte{0, 0, 0, 0x18}, []byte("ftypheic....")...), "image/heic", ".heic", true},
		{"3gp", append([]byte{0, 0, 0, 0x18}, []byte("ftyp3gp4....")...), "video/3gpp", ".3gp", true},
		{"webm/ebml", []byte{0x1A, 0x45, 0xDF, 0xA3, 0, 0, 0, 0}, "video/webm", ".webm", true},
		{"ogg", []byte("OggS" + "payload...."), "audio/ogg", ".ogg", true},
		{"flac", []byte("fLaC" + "payload...."), "audio/flac", ".flac", true},
		{"wav", []byte("RIFF" + "size" + "WAVE" + "...."), "audio/wav", ".wav", true},
		{"mp3 id3", []byte("ID3" + "\x03\x00\x00\x00\x00\x00\x00"), "audio/mpeg", ".mp3", true},
		{"pdf rejected", []byte("%PDF-1.7\n%...."), "", "", false},
		{"svg rejected", []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"), "", "", false},
		{"exe rejected", []byte("MZ\x90\x00\x03\x00\x00\x00...."), "", "", false},
		{"empty rejected", []byte{}, "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mime, ext, ok := Detect(tc.head)
			if ok != tc.wantOK || mime != tc.wantMime || ext != tc.wantExt {
				t.Fatalf("Detect() = (%q, %q, %v), want (%q, %q, %v)",
					mime, ext, ok, tc.wantMime, tc.wantExt, tc.wantOK)
			}
		})
	}
}
