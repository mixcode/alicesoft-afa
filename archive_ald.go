// Package aliceafa is a hander for AliceSoft's ALD and AFA archive format.
// This package also contains decoder for AliceSoft QNT and DCF image files.
package aliceafa

import (
	"errors"
	"io"

	"golang.org/x/text/encoding/japanese"

	bst "github.com/mixcode/binarystruct"
)

var (
	ErrInvalidArchive = errors.New("invalid archive file")
	ErrInvalidEntry   = errors.New("invalid entry index")
	ErrUnknownVersion = errors.New("unknown archive version")
)

// info of each file entry in the ALD/AFA archive
type FileEntry struct {
	Name         string // filename
	Offset, Size int64  // absolute file offset and size to the file entry
}

type FileType int

const (
	TypeALD FileType = 0x01 // .ald archive
	TypeAFA FileType = 0x11 // .afa archive
)

// AliceSoft ALD/AFA archive
type AliceArch struct {
	Type  FileType
	Entry []FileEntry // info of file entries in the archive
}

// Number of entries in the archive
func (p *AliceArch) Size() int {
	return len(p.Entry)
}

// Read the data body of a file entry.
// entryIndex is an index of p.Entry, and r must be the open file handle of the archive file
func (p *AliceArch) Read(r io.ReadSeeker, entryIndex int) (data []byte, err error) {
	if entryIndex < 0 || entryIndex >= p.Size() {
		err = ErrInvalidEntry
		return
	}
	entry := p.Entry[entryIndex]
	if entry.Size == 0 {
		// simply no data
		return nil, nil
	}
	_, err = r.Seek(entry.Offset, io.SeekStart)
	if err != nil {
		return
	}

	buf := make([]byte, entry.Size)
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return
	}
	return buf, nil
}

// Load file info of Alicesoft ALD archive file.
// An ALD archive may have an extension of ".ald", ".alk" and ".dat".
func OpenALD(rs io.ReadSeeker) (ald *AliceArch, err error) {

	/*
		// get "driver letter index" using the last letter of the filename
		// Ex: FILENAME_SA.ALD -> the last letter is 'A' -> 1
		_, fname := filepath.Split(filename)
		ext := filepath.Ext(fname)
		fbody := fname[:len(fname)-len(ext)]
		drvletter := 0
		if len(fbody) > 2 {
			fb := strings.ToUpper(fbody)
			l := fb[len(fb)-1]
			if l == '@' {
				drvletter = 0
			} else {
				drvletter = l - 'A' + 1
			}
		}
	*/

	//==============================================================================
	// ALD file format
	// +00~+03  offsetBlockSize	// actual offset block size is offsetBlockSize<<8
	// +06~+offsetBlockSize [][3]offsets	// actual offsets are each offset<<8
	// +offsetBlockSize~+offsets[0]: [][3]fileIdMap // file id directory [0]: drvletter, [1:3]:id
	//-------------------------------------------------------------------------------

	// read file offset list
	// first 3 bytes is the size of offset block
	_, err = rs.Seek(0, io.SeekStart)
	if err != nil {
		return
	}
	buf3 := make([]byte, 3) // 3-byte buffer
	_, err = io.ReadFull(rs, buf3)
	if err != nil {
		return
	}
	offsetBlockSize := ((int64(buf3[2]) << 16) | (int64(buf3[1]) << 8) | int64(buf3[0])) << 8
	entryOffset := make([]int64, 0) // file entry offset of each entry
	lastS := int64(0)
	offsetBlockSize -= 3 // block size includes the size itself
	for sz := int64(0); sz < offsetBlockSize; {
		n := 0
		n, err = io.ReadFull(rs, buf3)
		if err != nil {
			return
		}
		s := ((int64(buf3[2]) << 16) | (int64(buf3[1]) << 8) | int64(buf3[0])) << 8
		if s == 0 {
			break
		}
		if s <= lastS {
			// new offset must be larger than the old offset
			err = ErrInvalidArchive
			return
		}
		entryOffset = append(entryOffset, s)
		lastS = s
		sz += int64(n)
	}
	/*
		// read file ID block
		// The file ID block is may be a global table of global file ID to the archive file ID and the file number.
		fileIdSize := offsetList[0] - int64(offsetBlockSize)
		if fileIdSize > 0 {
			_, err = fi.Seek(offsetBlockSize, io.SeekStart)
			if err != nil {
				return
			}
			type FileId struct {
				DriveId, FileId int
			}
			l := fileIdSize / 3
			fileIdList = make([]FileId, l)
			for i:=0; i<l; i++
				_, err = io.ReadFull(fi, buf3)
				if err != nil {
					return
				}
				fileIdList[i].ArchiveId = int(buf3[0])
				fileIdList[i].FileNo = int(buf3[1])|(int(buf3[2])<<8)
			}
		}
	*/

	// Read file headers
	fileCount := len(entryOffset)
	aldInfo := make([]FileEntry, len(entryOffset))
	var u32sz uint32
	sjisDecoder := japanese.ShiftJIS.NewDecoder()
	for i := 0; i < fileCount; i++ {
		_, err = rs.Seek(entryOffset[i], io.SeekStart)
		if err != nil {
			return
		}
		// read first 4 byte as the header size
		_, err = bst.Read(rs, bst.LittleEndian, &u32sz)
		if err != nil {
			return
		}
		if u32sz < 16 || u32sz > 256 {
			// invalid header size
			if i == fileCount-1 {
				// the last entry may not be a file entry
				// and may have the invalid block size
				// 4e 4c 01 00 10 00 00 00 .. .. .. .. .. .. .. ..
				aldInfo = aldInfo[:len(aldInfo)-1]
				continue
			}
			err = ErrInvalidArchive
			return
		}
		aldInfo[i].Offset = entryOffset[i] + int64(u32sz) // set file offset
		// read header
		buf := make([]byte, int(u32sz))
		for i := 0; i < 4; i++ { // set first 4 byte to the size of header block
			buf[i] = byte(u32sz & 0xff)
			u32sz >>= 8
		}
		_, err = io.ReadFull(rs, buf[4:])
		if err != nil {
			return
		}
		// get data size
		_, err = bst.Unmarshal(buf[4:8], bst.LittleEndian, &u32sz)
		if err != nil {
			return
		}
		aldInfo[i].Size = int64(u32sz)

		// get filename
		const filenameOffset = 0x10 // in the header
		nameLen := 0
		for filenameOffset+nameLen < len(buf) && buf[filenameOffset+nameLen] != 0 { // check the end of the filename
			nameLen++
		}
		if nameLen > 0 {
			// convert SJIS to UTF8
			var u8 []byte
			u8, err = sjisDecoder.Bytes(buf[filenameOffset : filenameOffset+nameLen])
			if err != nil {
				return
			}
			aldInfo[i].Name = string(u8)
		}
	}

	return &AliceArch{Type: TypeALD, Entry: aldInfo}, nil
}
