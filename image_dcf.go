package aliceafa

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"io"

	bst "github.com/mixcode/binarystruct"
	"golang.org/x/text/encoding/japanese"
)

var (
	sjisDecoder = japanese.ShiftJIS.NewDecoder()
)

// DCF is QNF file with independent alpha masks.
// returned baseImageName contains the base image filename that should be overlayed on.
func LoadDCF(rs io.ReadSeeker) (img image.Image, baseImageName string, err error) {

	readSz := int64(0)

	type ChunkHeader struct {
		Signature string `binary:"[4]byte"`
		Len       int    `binary:"uint32"` // length of the following data body
	}

	// read DCF file header chuk
	var dcfHeader struct {
		ChunkHeader
		Unknown1         int    `binary:"uint32"` // usually 01. Version?
		Width, Height    int    `binary:"uint32"` // image dimension
		Unknown2         int    `binary:"uint32"` // usually 0x20
		BaseImageNameLen int    `binary:"uint32"`
		BaseImageName    []byte `binary:"[BaseImageNameLen]byte"` // name of the base image
	}
	sz, err := bst.Read(rs, bst.LittleEndian, &dcfHeader)
	if err != nil {
		return
	}
	if sz != dcfHeader.Len+8 {
		// overrun
		err = ErrInvalidFormat
		return
	}
	readSz += int64(sz)
	if dcfHeader.Signature != "dcf " {
		err = ErrInvalidFormat
		return
	}

	// decode base image name
	// the base name is ShiftJIS code bytes rotate-righted by (length%7 + 1)
	rot := len(dcfHeader.BaseImageName)%7 + 1
	for i, b := range dcfHeader.BaseImageName {
		// rotate left to recover ShiftJIS codes
		dcfHeader.BaseImageName[i] = (b << rot) | (b >> (8 - rot))
	}
	baseNameBytes, err := sjisDecoder.Bytes(dcfHeader.BaseImageName)
	if err != nil {
		return
	}
	baseImageName = string(baseNameBytes)

	// read alpha mask block chunk
	var alphaChunk struct {
		ChunkHeader
		UncompressedSize int    `binary:"uint32"`
		Zip              []byte `binary:"[Len - 4]byte"`
	}
	sz, err = bst.Read(rs, bst.LittleEndian, &alphaChunk)
	if err != nil {
		return
	}
	readSz += int64(sz)
	if alphaChunk.Signature != "dfdl" {
		err = ErrInvalidFormat
		return
	}
	alphaMask, err := uncompressZlib(alphaChunk.Zip)
	if err != nil {
		return
	}
	// first 4 byte is the number of alpha mask bytes
	maskCount := 0
	for i := 0; i < 4; i++ {
		maskCount = maskCount | (int(alphaMask[i]) << (8 * i))
	}
	alphaMask = alphaMask[4:]
	if maskCount != len(alphaMask) {
		err = ErrInvalidFormat
		return
	}

	// read embedded QNF image
	var imageChunk ChunkHeader
	sz, err = bst.Read(rs, bst.LittleEndian, &imageChunk)
	if err != nil {
		return
	}
	readSz += int64(sz)
	if imageChunk.Signature != "dcgd" {
		err = ErrInvalidFormat
		return
	}
	//qnfImg, sz64, err := LoadQNT(fi)
	//readSz += sz64
	qnfImg, err := LoadQNT(rs)
	if err != nil {
		return
	}
	// TODO: use stream counter for actual read size
	readSz += int64(imageChunk.Len)

	// merge the image and the alpha mask
	const (
		// each alpha mask byte represents a 16x16 pixel block
		blockX, blockY = 16, 16
	)
	//xCount := (dcfHeader.Width + blockX - 1) / blockX
	//yCount := (dcfHeader.Height + blockY - 1) / blockY
	xCount := dcfHeader.Width / blockX // Note: the mask may be smaller than the image
	yCount := dcfHeader.Height / blockY
	if xCount*yCount != maskCount {
		// log.Printf("dim[%d x %d]", dcfHeader.Width, dcfHeader.Height)
		// log.Printf("qnt[%d x %d]", qnfImg.Bounds().Dx(), qnfImg.Bounds().Dy())
		// log.Printf("dimb[%d x %d](%d)", dcfHeader.Width/blockX, dcfHeader.Height/blockY, (dcfHeader.Width/blockX)*(dcfHeader.Height/blockY))
		err = fmt.Errorf("invalid alpha block size: expected %d, actual %d", xCount*yCount, maskCount)
		return
	}

	// assume that QNF image is in NRGBA format.
	// NRGBA is Non-alpha-premultiplied RGB, that means 0<=RGB<=255
	// (plane RGBA is Alpha-premultiplied, that means 0<=RGB<=A)
	rgbImg, ok := qnfImg.(*image.NRGBA)
	if !ok {
		err = fmt.Errorf("image is not in NRGBA format")
		return
	}
	k := 0
	for i := 0; i < yCount; i++ { // iterate over each 16x16 block, then remove alpha if the block's mask value is 0
		for j := 0; j < xCount; j++ {
			maskValue := alphaMask[k] // maskValue is 0 or 1
			k++
			if maskValue != 0 && maskValue != 1 {
				err = fmt.Errorf("unknown alpha mask value")
				return
			}
			if maskValue == 0 { // zero: do not mask
				continue
			}
			px, py := j*blockX, i*blockY
			dx, dy := px+blockX, py+blockY
			if dx > dcfHeader.Width {
				dx = dcfHeader.Width
			}
			if dy > dcfHeader.Height {
				dy = dcfHeader.Height
			}
			for y := py; y < dy; y++ {
				o := y*rgbImg.Stride + px*4 + 3 // +3 for alpha byte
				for x := px; x < dx; x++ {
					rgbImg.Pix[o] = 0
					o += 4
				}
			}
		}
	}
	img = rgbImg
	return
}

func uncompressZlib(input []byte) (unzipped []byte, err error) {
	b := bytes.NewBuffer(input)
	zl, err := zlib.NewReader(b)
	if err != nil {
		return
	}
	return io.ReadAll(zl)
}
