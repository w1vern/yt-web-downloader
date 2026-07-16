package jobs

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image/png"
)

// flacPictureBlock builds a FLAC METADATA_BLOCK_PICTURE structure (as used
// by the vorbis-comment convention yt-dlp/mutagen and ffmpeg understand for
// ogg/opus cover art) from raw PNG bytes, and returns it base64-encoded
// (standard encoding), ready to pass as
// "-metadata METADATA_BLOCK_PICTURE=<result>" to ffmpeg.
//
// Structure (all integers big-endian uint32):
//
//	picture type (3 = front cover)
//	mime type length + mime type string ("image/png")
//	description length + description string ("")
//	width, height (decoded from the PNG header)
//	color depth (24)
//	number of colors used (0 = not a palette image)
//	picture data length + picture data
func flacPictureBlock(pngData []byte) (string, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(pngData))
	if err != nil {
		return "", fmt.Errorf("decode png config: %w", err)
	}

	const mime = "image/png"
	const desc = ""

	var buf bytes.Buffer
	writeU32 := func(v uint32) {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], v)
		buf.Write(b[:])
	}

	writeU32(3) // picture type: front cover
	writeU32(uint32(len(mime)))
	buf.WriteString(mime)
	writeU32(uint32(len(desc)))
	buf.WriteString(desc)
	writeU32(uint32(cfg.Width))
	writeU32(uint32(cfg.Height))
	writeU32(24) // color depth
	writeU32(0)  // number of colors (non-palette)
	writeU32(uint32(len(pngData)))
	buf.Write(pngData)

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
