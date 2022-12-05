package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/encoding/japanese"

	aliceafa "github.com/mixcode/alicesoft-afa"
	bst "github.com/mixcode/binarystruct"
)

// flags
var (
	listOnly  = false
	imageOnly = false
	rawImage  = false
	plainDCF  = false
	quiet     = false
	overwrite = false
	outDir    = ""
)

var (
	sjisDecoder = japanese.ShiftJIS.NewDecoder()
)

func isImageExt(ext string) bool {
	return ext == ".dcf" || ext == ".qnt"
}

func baseAndLowerExt(filename string) (base, ext string) {
	ext = strings.ToLower(filepath.Ext(filename))
	base = filename[:len(filename)-len(ext)]
	return
}

func loadDCFBaseName(fi io.Reader) string {
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
		return ""
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
		return ""
	}
	return string(baseNameBytes)
}

// show filenames
func listFiles(rs io.ReadSeeker, arch *aliceafa.AliceArch) (err error) {
	for _, e := range arch.Entry {
		_, ext := baseAndLowerExt(e.Name)
		if imageOnly && !isImageExt(ext) {
			continue
		}
		if ext == ".dcf" {
			// for DCF, also show the name of the base file
			baseName := ""
			_, err = rs.Seek(e.Offset, io.SeekStart)
			if err == nil {
				baseName = loadDCFBaseName(rs)
			}
			if baseName != "" {
				fmt.Printf("%s (%s)\n", e.Name, baseName)
			}
			continue
		}
		fmt.Printf("%s\n", e.Name)
	}
	return
}

func isFileExist(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

func saveFile(rs io.ReadSeeker, arch *aliceafa.AliceArch, index int, nameMap map[string]int) (err error) {
	e := arch.Entry[index]
	_, ext := baseAndLowerExt(e.Name)
	isImage := isImageExt(ext)
	if imageOnly && !isImage {
		// don't save non-image file
		return
	}

	outPath := filepath.Join(outDir, e.Name)
	if !overwrite && isFileExist(outPath) {
		err = fmt.Errorf("file %s exists", outPath)
		return
	}
	_, err = rs.Seek(e.Offset, io.SeekStart)
	if err != nil {
		return
	}

	if rawImage || !isImage {
		// save file as-is
		var fo *os.File
		fo, err = os.Create(outPath)
		if err != nil {
			return
		}
		defer fo.Close()
		_, err = io.CopyN(fo, rs, e.Size)
		if !quiet {
			fmt.Println(outPath)
		}
		return
	}

	var img image.Image
	switch ext {
	case ".qnt":
		img, err = aliceafa.LoadQNT(rs)
		if err != nil {
			return
		}
	case ".dcf":
		baseName := ""
		img, baseName, err = aliceafa.LoadDCF(rs)
		if err != nil {
			return
		}
		if !plainDCF && baseName != "" {
			baseBase, _ := baseAndLowerExt(baseName)
			baseIdx, ok := nameMap[baseBase]
			if ok {
				// load the base file
				baseEntry := arch.Entry[baseIdx]
				_, err = rs.Seek(baseEntry.Offset, io.SeekStart)
				if err != nil {
					return
				}
				baseImg, er := aliceafa.LoadQNT(rs)
				if er != nil {
					_, err = rs.Seek(baseEntry.Offset, io.SeekStart)
					if err != nil {
						return
					}
					baseImg, _, _ = aliceafa.LoadDCF(rs)
				}
				if baseImg != nil {
					img, err = mergeImage(baseImg, img)
					if err != nil {
						return
					}
				}
			}
		}
	}

	if img != nil {
		// write PNG
		outPath += ".png"
		var fo *os.File
		fo, err = os.Create(outPath)
		if err != nil {
			return
		}
		defer fo.Close()
		err = png.Encode(fo, img)
		if err != nil {
			return
		}
		if !quiet {
			fmt.Println(outPath)
		}
	}

	return
}

func mergeImage(baseImg, img image.Image) (image.Image, error) {
	bi, ok := baseImg.(draw.Image)
	if !ok {
		return nil, fmt.Errorf("not a drawable iamge")
	}
	draw.Draw(bi, img.Bounds(), img, image.Point{}, draw.Over)
	return baseImg, nil
}

func run() (err error) {
	args := flag.Args()
	if len(args) == 0 {
		return fmt.Errorf("archive filename not given (use -help for help)")
	}

	archiveFile, args := args[0], args[1:]

	// open archive file
	fi, err := os.Open(archiveFile)
	if err != nil {
		return
	}
	defer fi.Close()
	var arch *aliceafa.AliceArch
	_, afName := filepath.Split(archiveFile)
	afBase, afExt := baseAndLowerExt(afName)
	switch afExt {
	case ".ald":
		arch, err = aliceafa.OpenALD(fi)
		if err != nil {
			return
		}
	case ".afa":
		arch, err = aliceafa.OpenAFA(fi)
		if err != nil {
			return
		}
	default:
		arch, err = aliceafa.OpenAFA(fi)
		if err != nil {
			arch, err = aliceafa.OpenALD(fi)
			if err != nil {
				return
			}
		}
	}

	if listOnly {
		// show file list
		return listFiles(fi, arch)
	}

	// make output directory
	if outDir == "" {
		outDir = filepath.Join(".", afBase)
	}
	err = os.MkdirAll(outDir, 0755)
	if err != nil {
		return
	}

	// make archive name map for DCF merge
	nameMap := make(map[string]int)
	for i, e := range arch.Entry {
		base, _ := baseAndLowerExt(e.Name)
		nameMap[base] = i
	}

	if len(args) > 0 {
		argMap := make(map[string]bool)
		for _, s := range args {
			argMap[s] = true
		}
		for i, e := range arch.Entry {
			if argMap[e.Name] {
				err = saveFile(fi, arch, i, nameMap)
				if err != nil {
					return
				}
			}
		}
	} else {
		for i := range arch.Entry {
			err = saveFile(fi, arch, i, nameMap)
			if err != nil {
				return
			}
		}

	}

	return
}

func main() {
	var err error

	flag.Usage = func() {
		o := flag.CommandLine.Output()
		fmt.Fprintf(o, "%s: extract files from AliceSoft ALD/AFA archive\n", os.Args[0])
		fmt.Fprintf(o, "usage: %s [flags] ArchiveFile [extractFile ...]\n", os.Args[0])
		fmt.Fprintf(o, "flags:\n")
		flag.PrintDefaults()
	}
	flag.BoolVar(&listOnly, "ls", listOnly, "show list of files without extracting")
	flag.BoolVar(&imageOnly, "imageonly", imageOnly, "extract only QNT/DCF image files")
	flag.BoolVar(&rawImage, "raw", rawImage, "do NOT convert QNT/DCF to PNG")
	flag.BoolVar(&plainDCF, "plaindcf", plainDCF, "do NOT join DCF with base image")
	flag.BoolVar(&quiet, "q", quiet, "suppress log output")
	flag.BoolVar(&overwrite, "f", overwrite, "force overwrite existing files")
	flag.StringVar(&outDir, "outdir", outDir, "output directory. default is the name of input file")

	flag.Parse()

	err = run()

	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
