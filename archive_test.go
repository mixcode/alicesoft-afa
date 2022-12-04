package aliceald

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestAld(t *testing.T) {

	fname := "./_testdata/testald.ald"

	fi, err := os.Open(fname)
	if err != nil {
		t.Fatal(err)
	}
	defer fi.Close()
	ald, err := LoadALD(fi)
	if err != nil {
		t.Fatal(err)
	}

	//if ald.Path != fname {
	//	t.Fatalf("invalid file name")
	//}
	sz := ald.Size()
	if sz != 0x340 {
		t.Fatalf("invalid file count")
	}

	/*
		for i := 0; i < sz; i++ {
			f := ald.Entry[i]
			log.Printf("(%d)[%s]: %x~%x (+%x)", i, f.Name, f.Offset, f.Offset+f.Size-1, f.Size)
		}

		files := []int{830, 831}
		for _, n := range files {
			err = saveFile(ald, fi, n)
			if err != nil {
				t.Fatal(err)
			}
		}
	*/
}

func TestAfa(t *testing.T) {
	fname := "./_testdata/testafa.afa"

	fi, err := os.Open(fname)
	if err != nil {
		t.Fatal(err)
	}
	defer fi.Close()
	afa, err := LoadAFA(fi)
	if err != nil {
		t.Fatal(err)
	}

	sz := afa.Size()
	if sz != 4297 {
		t.Fatalf("invalid file count")
	}

	/*
		for i := 0; i < sz; i++ {
			f := afa.Entry[i]
			log.Printf("(%d)[%s]: %x~%x (+%x)", i, f.Name, f.Offset, f.Offset+f.Size-1, f.Size)
		}

		//files := []int{1425, 1426}
		files := []int{1351, 1352}
		for _, n := range files {
			err = saveFile(afa, fi, n)
			if err != nil {
				t.Fatal(err)
			}
		}
	*/

}

func saveFile(arch *AliceArch, fi io.ReadSeeker, index int) (err error) {
	d, err := arch.Read(fi, index)
	if err != nil {
		return
	}
	return os.WriteFile(filepath.Join("_testdata/", arch.Entry[index].Name), d, 0644)
}
