package aliceafa

import (
	"compress/zlib"
	"errors"
	"fmt"
	"image"
	"io"

	bst "github.com/mixcode/binarystruct"
)

var (
	ErrInvalidFormat = errors.New("invalid data format")
)

// Load QNT image.
// The QNT images assumed to be 8-bit RGBA image.
// Returning img is actually an *image.NRGBA type.
func LoadQNT(fi io.ReadSeeker) (img image.Image, err error) {

	readSz := int64(0)
	headerSize := int64(48)

	// read signature
	var qntSig struct {
		Signature []byte `binary:"[4]byte"`
		Version   int    `binary:"uint32"`
	}
	sz, err := bst.Read(fi, bst.LittleEndian, &qntSig)
	if err != nil {
		return
	}
	if string(qntSig.Signature[:3]) != "QNT" || qntSig.Signature[3] != 0 {
		return nil, ErrInvalidFormat
	}
	readSz += int64(sz)

	// the total header size vary on the version
	if qntSig.Version != 0 {
		// if the version is not zero, then read the header size
		var s uint32
		sz, err = bst.Read(fi, bst.LittleEndian, &s)
		if err != nil {
			return
		}
		readSz += int64(sz)
		headerSize = int64(s)
	}

	// read the image info
	var qntImageInfo struct {
		//+0x00
		X, Y          int `binary:"uint32"`
		Width, Height int `binary:"uint32"`
		//+0x10
		ColorDepth                 int `binary:"uint32"`
		Reserved                   int `binary:"uint32"`
		RGBDataSize, AlphaDataSize int `binary:"uint32"`
		//+0x20
	}
	sz, err = bst.Read(fi, bst.LittleEndian, &qntImageInfo)
	if err != nil {
		return
	}
	readSz += int64(sz)
	if qntImageInfo.ColorDepth != 24 {
		err = fmt.Errorf("unsupported bit depth; must be 24 but has %d", qntImageInfo.ColorDepth)
		return
	}

	// read extra headers if exists
	var extraHeader []byte
	if readSz < headerSize {
		extraHeader = make([]byte, headerSize-readSz)
		sz, err = io.ReadFull(fi, extraHeader)
		if err != nil {
			return
		}
		readSz += int64(sz)
	}

	// image width and height
	width, height := qntImageInfo.Width, qntImageInfo.Height
	if width == 0 || height == 0 {
		// no image
		return nil, nil
	}

	// determine width and height of raw data
	// raw data width and colum are aligned to multiple of 2
	rawWidth, rawHeight := width, height
	if width%2 != 0 {
		rawWidth++
	}
	if height%2 != 0 {
		rawHeight++
	}
	planeSize := rawWidth * rawHeight

	// QNT plane bytes are diff-encoded
	decodeDiff := func(buf []byte, w, h int) {
		k := 1
		for i := 1; i < w; i++ {
			buf[k] = buf[k-1] - buf[k]
			k++
		}
		prevLine := 0
		for j := 1; j < h; j++ {
			buf[k] = buf[prevLine] - buf[k]
			k, prevLine = k+1, prevLine+1
			for j := 1; j < w; j++ {
				// (left_pixel + upper_pixel)/2 - alphaValue
				a := byte((int(buf[prevLine]) + int(buf[k-1])) >> 1)
				buf[k] = a - buf[k]
				k, prevLine = k+1, prevLine+1
			}
		}
	}

	// allocate plane buffer
	plane := make([][]byte, 4) // buffer for R, G, B, A
	for i := 0; i < 4; i++ {
		plane[i] = make([]byte, rawWidth*rawHeight)
	}

	// load RGB planes
	if qntImageInfo.RGBDataSize > 0 {

		lastPos, _ := fi.Seek(0, io.SeekCurrent)

		// reorder pixels
		reorderPixel := func(dest, raw []byte, w, h int) {
			// QNT pixels are grouped in same colors, blue first.
			// ex) BBBB... GGGG... RRRR...
			// Each RGB plane is sequence of 2x2 pixels values ordered in [ LU, LD, RU, RD ] order
			//       x0 x1 x2 x3 ...
			//    y0 P1 P3 P5 P7
			//    y1 P2 P4 P6 P8
			var i, j, k int
			for j = 0; j < h; j += 2 { // for each two lines
				var p = j * w
				for i = 0; i < w; i += 2 { // for each two pixels
					dest[p] = raw[k]       // LU
					dest[p+w] = raw[k+1]   // LD
					dest[p+1] = raw[k+2]   // RU
					dest[p+w+1] = raw[k+3] // RD
					p, k = p+2, k+4
				}
			}
		}

		// prepare ZLIB decoder
		rgbReader, e := zlib.NewReader(io.LimitReader(fi, int64(qntImageInfo.RGBDataSize)))
		if e != nil {
			err = e
			return
		}

		// read BGR planes
		buf := make([]byte, planeSize)
		for i := 2; i >= 0; i-- { // QNF image is ordered as BGR
			_, err = io.ReadFull(rgbReader, buf) // read plane for one color
			if err != nil {
				return
			}
			reorderPixel(plane[i], buf, rawWidth, rawHeight)
			decodeDiff(plane[i], rawWidth, rawHeight)
		}

		// skip leftover bytes
		currentPos, _ := fi.Seek(0, io.SeekCurrent)
		rgbSize := currentPos - lastPos
		if rgbSize != int64(qntImageInfo.RGBDataSize) {
			skipSize := int64(qntImageInfo.RGBDataSize) - rgbSize
			io.CopyN(io.Discard, fi, skipSize)
		}
		readSz += int64(qntImageInfo.RGBDataSize)
	}

	if qntImageInfo.AlphaDataSize > 0 {
		// load alpha plane
		alphaReader, e := zlib.NewReader(io.LimitReader(fi, int64(qntImageInfo.AlphaDataSize)))
		if e != nil {
			err = e
			return
		}
		_, err = io.ReadFull(alphaReader, plane[3])
		if err != nil {
			return
		}
		decodeDiff(plane[3], rawWidth, rawHeight)
		readSz += int64(qntImageInfo.AlphaDataSize)
	} else {
		// Set all pixel's alpha to 255
		for i := 0; i < planeSize; i++ {
			plane[3][i] = 0xff
		}
	}

	// build image from planes
	// NRGBA is Non-alpha-premultiplied RGB, that means 0<=RGB<=255
	// (plane RGBA is Alpha-premultiplied, that means 0<=RGB<=A)
	wx, wy := qntImageInfo.X, qntImageInfo.Y
	rgba := image.NewNRGBA(image.Rect(wx, wy, wx+width, wy+height))
	p := 0
	for i := 0; i < height; i++ {
		k := i * rawWidth
		for j := 0; j < width; j++ {
			rgba.Pix[p+0] = plane[0][k] // R
			rgba.Pix[p+1] = plane[1][k] // G
			rgba.Pix[p+2] = plane[2][k] // B
			rgba.Pix[p+3] = plane[3][k] // A

			// TEST: print alpha plane
			//rgba.Pix[p+0] = plane[3][k] // A
			//rgba.Pix[p+1] = plane[3][k] // A
			//rgba.Pix[p+2] = plane[3][k] // A
			//rgba.Pix[p+3] = 255

			p, k = p+4, k+1
		}
	}

	img = rgba
	return
}
