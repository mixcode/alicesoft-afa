package aliceafa

import (
	"compress/zlib"
	"io"

	"golang.org/x/text/encoding/japanese"

	bst "github.com/mixcode/binarystruct"
)

// Load file info of Alicesoft AFA archive.
// An AFA archive may has ".afa" extension.
func OpenAFA(rs io.ReadSeeker) (afa *AliceArch, err error) {

	// Prepare shift-jis text decoder
	var mst = new(bst.Marshaller)
	mst.AddTextEncoding("sjis", japanese.ShiftJIS)

	// AFA file is a chunked data format
	type ChunkHeader struct {
		Signature string `binary:"[4]byte"` // 4-char signature
		Len       int    `binary:"uint32"`  // tag length, including Signature and Len
	}

	// AFA file global header
	var afaHeader struct {
		// +0x00
		ChunkHeader
		// +0x08
		AliceSignature string `binary:"[8]byte"` // "AlicArch"

		// +0x10
		Version    int   `binary:"uint32"` // 1 or 2
		Unknown    int   `binary:"uint32"` // always 1
		DataOffset int64 `binary:"uint32"` // absolute file offset to "DATA" tag
	}
	_, err = mst.Read(rs, bst.LittleEndian, &afaHeader)
	if err != nil {
		return
	}
	if afaHeader.Signature != "AFAH" || afaHeader.AliceSignature != "AlicArch" {
		return nil, ErrInvalidArchive
	}
	if afaHeader.Len != 0x1c || (afaHeader.Version != 1 && afaHeader.Version != 2) {
		return nil, ErrUnknownVersion
	}

	// "INFO" tag: the file directory
	var infoTag struct {
		// +0x00
		ChunkHeader
		// +0x80
		DecompressedSize int `binary:"uint32"`
		EntryCount       int `binary:"uint32"`
	}
	_, err = mst.Read(rs, bst.LittleEndian, &infoTag)
	if err != nil {
		return
	}
	if infoTag.Signature != "INFO" {
		return nil, ErrInvalidArchive
	}
	// read ZLIB compressed tag body
	infoCompressedSize := int64(infoTag.Len) - 0x10
	zReader, err := zlib.NewReader(io.LimitReader(rs, infoCompressedSize))
	if err != nil {
		return
	}
	fileEntry := make([]FileEntry, infoTag.EntryCount)
	switch afaHeader.Version {
	case 1:
		// parse v1 directory info
		type infoEntryV1 struct {
			FilenameLen        int    `binary:"uint32"`
			FilenamePaddedLen  int    `binary:"uint32"`
			Filename           string `binary:"string(FilenameLen),encoding=sjis"`
			FilenamePad        []byte `binary:"[FilenamePaddedLen - FilenameLen]byte"`
			Unknown1, Unknown2 uint32
			V1Unknown3         uint32 // This entry only exists in AFA v1
			Offset, Size       int64  `binary:"uint32"`
		}
		entries := make([]infoEntryV1, infoTag.EntryCount)
		sz := 0
		sz, err = mst.Read(zReader, bst.LittleEndian, &entries)
		if err != nil {
			return
		}
		if sz != int(infoTag.DecompressedSize) {
			return nil, ErrInvalidArchive
		}
		for i, e := range entries {
			fileEntry[i].Name = e.Filename
			fileEntry[i].Offset = afaHeader.DataOffset + e.Offset
			fileEntry[i].Size = e.Size
		}

	case 2:
		// parse v2 directory info
		type infoEntryV2 struct {
			FilenameLen        int    `binary:"uint32"`
			FilenamePaddedLen  int    `binary:"uint32"`
			Filename           string `binary:"string(FilenameLen),encoding=sjis"`
			FilenamePad        []byte `binary:"[FilenamePaddedLen - FilenameLen]byte"`
			Unknown1, Unknown2 uint32
			Offset, Size       int64 `binary:"uint32"`
		}
		entries := make([]infoEntryV2, infoTag.EntryCount)
		sz := 0
		sz, err = mst.Read(zReader, bst.LittleEndian, &entries)
		if err != nil {
			return
		}
		if sz != int(infoTag.DecompressedSize) {
			return nil, ErrInvalidArchive
		}
		for i, e := range entries {
			fileEntry[i].Name = e.Filename
			fileEntry[i].Offset = afaHeader.DataOffset + e.Offset
			fileEntry[i].Size = e.Size
		}
	}

	// Note: a "DUMM" dummy tag may follow the INFO tag, then actual DATA body tag appears

	return &AliceArch{Type: TypeAFA, Entry: fileEntry}, nil
}
