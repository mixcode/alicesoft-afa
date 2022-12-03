package aliceald

import (
	"testing"
)

func TestAld(t *testing.T) {
	fname := "./_testdata/DGT.ALD"

	ald, err := LoadALD(fname)
	if err != nil {
		t.Fatal(err)
	}

	if ald.Path != fname {
		t.Fatalf("invalid file name")
	}
	sz := ald.Size()
	if sz != 0x340 {
		t.Fatalf("invalid file count")
	}
	/*
		for i := 0; i < sz; i++ {
			f := ald.File[i]
			log.Printf("(%d)[%s]: %x~%x (+%x)", i, f.Name, f.Offset, f.Offset+f.Size-1, f.Size)
		}
	*/
}
