package jobs

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestFlacPictureBlock(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 10), G: uint8(y * 10), B: 0, A: 255})
		}
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	pngData := pngBuf.Bytes()

	b64, err := flacPictureBlock(pngData)
	if err != nil {
		t.Fatalf("flacPictureBlock: %v", err)
	}

	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}

	off := 0
	readU32 := func() uint32 {
		v := binary.BigEndian.Uint32(raw[off : off+4])
		off += 4
		return v
	}

	picType := readU32()
	if picType != 3 {
		t.Errorf("picture type = %d, want 3", picType)
	}

	mimeLen := readU32()
	mime := string(raw[off : off+int(mimeLen)])
	off += int(mimeLen)
	if mime != "image/png" {
		t.Errorf("mime = %q, want image/png", mime)
	}

	descLen := readU32()
	off += int(descLen)
	if descLen != 0 {
		t.Errorf("description length = %d, want 0", descLen)
	}

	width := readU32()
	height := readU32()
	if width != 2 {
		t.Errorf("width = %d, want 2", width)
	}
	if height != 3 {
		t.Errorf("height = %d, want 3", height)
	}

	depth := readU32()
	if depth != 24 {
		t.Errorf("depth = %d, want 24", depth)
	}

	colors := readU32()
	if colors != 0 {
		t.Errorf("colors = %d, want 0", colors)
	}

	dataLen := readU32()
	if int(dataLen) != len(pngData) {
		t.Errorf("data length = %d, want %d", dataLen, len(pngData))
	}
	trailing := raw[off:]
	if len(trailing) != len(pngData) {
		t.Errorf("trailing data length = %d, want %d", len(trailing), len(pngData))
	}
	if !bytes.Equal(trailing, pngData) {
		t.Errorf("trailing data does not match original png bytes")
	}
}
