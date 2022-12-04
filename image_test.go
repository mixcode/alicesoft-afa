package aliceald

import (
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"testing"
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

	log.Printf("bound: %v", img.Bounds())

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
