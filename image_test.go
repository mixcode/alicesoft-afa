package aliceafa

import (
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bst "github.com/mixcode/binarystruct"
)

func TestQNT(t *testing.T) {
	filename := "_testdata/testqnt.qnt"

	fi, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer fi.Close()

	img, err := LoadQNT(fi)
	if err != nil {
		t.Fatal(err)
	}
	_ = img

	/*
		err = savePNG(asExt(filename, "png"), img)
		if err != nil {
			t.Fatal(err)
		}
	*/
}

func TestDCF(t *testing.T) {
	filename := "_testdata/testdcf.dcf"

	fi, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer fi.Close()

	img, baseName, err := LoadDCF(fi)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = img, baseName
	//log.Printf("basename: %s", baseName)

	err = savePNG(asExt(filename, "png"), img)
	if err != nil {
		t.Fatal(err)
	}

}

func savePNG(filename string, img image.Image) (err error) {
	f, err := os.Create(filename)
	if err != nil {
		return
	}
	defer f.Close()
	return png.Encode(f, img)
}

func asExt(filename, ext string) string {
	e := filepath.Ext(filename)
	body := filename[:len(filename)-len(e)]
	if ext[0] != '.' {
		ext = "." + ext
	}
	return body + ext
}

func DONOTUSE_TestDCFNames(t *testing.T) {
	filename := "_testdata/testafa.afa"

	fi, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer fi.Close()

	afa, err := OpenAFA(fi)
	if err != nil {
		t.Fatal(err)
	}

	loadDCFName := func(fi io.Reader) []byte {
		// read DCF file header chuk
		var dcfHeader struct {
			Signature        string `binary:"[4]byte"`
			Len              int    `binary:"uint32"`
			Unknown1         int    `binary:"uint32"` // usually 01. Version?
			Width, Height    int    `binary:"uint32"`
			Unknown2         int    `binary:"uint32"` // usually 0x20
			BaseImageNameLen int    `binary:"uint32"`
			BaseImageName    []byte `binary:"[BaseImageNameLen]byte"` // name of the base image
		}
		_, err := bst.Read(fi, bst.LittleEndian, &dcfHeader)
		if err != nil {
			return nil
		}
		return dcfHeader.BaseImageName
	}

	for i, f := range afa.Entry {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext != ".dcf" {
			continue
		}
		fi.Seek(f.Offset, io.SeekStart)
		nameBytes := loadDCFName(fi)
		sz := len(nameBytes)

		var decoded []byte
		n := 0
		for n = 1; n <= 8; n++ {
			rot := 1
			for i, b := range nameBytes {
				nameBytes[i] = (b << rot) | (b >> (8 - rot))
			}
			decoded, err = sjisDecoder.Bytes(nameBytes)
			if err == nil && decoded[len(decoded)-4] == '.' {
				break
			}
		}
		if n > 8 {
			fmt.Printf("--[%d](%x|%d) rot: FAIL, [-]\n", i, sz, sz%7)
		} else {
			name := string(decoded)
			fmt.Printf("--[%d](%x|%d) rot: %d, [%s]\n", i, sz, sz%7, n, name)
		}
		hexPrint(nameBytes)
	}

}

func hexPrint(data []byte) {
	var i int
	var b byte
	for i, b = range data {
		if i%16 == 0 {
			fmt.Printf("%06d ", i)
		}
		if i%16 == 8 {
			fmt.Printf(" ")
		}
		fmt.Printf(" %02x", b)
		if i%16 == 15 {
			fmt.Printf("\n")
		}
	}
	if i%16 != 0 {
		fmt.Printf("\n")
	}
}
