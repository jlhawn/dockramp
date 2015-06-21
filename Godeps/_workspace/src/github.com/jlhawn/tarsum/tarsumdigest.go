package tarsum

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/jlhawn/tarsum/archive/tar"
	"github.com/jlhawn/tarsum/sha256"
)

const blockSize = 1 << 9

var archiveEndBlock = make([]byte, blockSize*2) // 2 blocks of zeroed bytes.

func computeBlockPadding(size int64) int {
	// since blockSize is a power of 2, we can do this instead of:
	// 		blocksize - (size % blocksize)
	// and its safe to cast to int because it is 255 max.
	return int(-size & (blockSize - 1))
}

// Digest implements a write-driven interface for calculating TarSums
type Digest struct {
	// Critical State/Fields
	version         Version
	digestStage     string
	headerBuffer    bytes.Buffer
	tarReader       *tar.Reader
	entryHash       sha256.Resumable
	sums            fileInfoSums
	fileCounter     int64
	bytesWritten    int64
	currentFilename string
	pad             int

	// Miscellaneous State/Fields
	err            error
	currentBuffer  bytes.Buffer
	headerSelector tarHeaderSelector
	copyBuf        []byte

	// Enable debug logging.
	debug bool
}

const (
	stageReadHeader  = "readHeader"
	stageReadEntry   = "readEntry"
	stageSkipPadding = "skipPadding"
	stageFinished    = "finished"
)

func NewDigest(version Version) (*Digest, error) {
	headerSelector, err := getTarHeaderSelector(version)
	if err != nil {
		return nil, err
	}

	tsd := &Digest{
		headerSelector: headerSelector,
		version:        version,
	}

	tsd.Reset()

	return tsd, nil
}

func (tsd *Digest) toggleDebug() bool {
	tsd.debug = !tsd.debug
	return tsd.debug
}

func (tsd *Digest) logDebug(format string, args ...interface{}) {
	if tsd.debug {
		fmt.Printf(format, args...)
	}
}

func (tsd *Digest) Size() int {
	return sha256.New().Size()
}

func (tsd *Digest) BlockSize() int {
	return sha256.New().BlockSize()
}

func (tsd *Digest) Reset() {
	tsd.headerBuffer.Reset()
	tsd.currentBuffer.Reset()

	tsd.digestStage = stageReadHeader
	tsd.tarReader = new(tar.Reader)
	tsd.entryHash = sha256.New()
	tsd.sums = fileInfoSums{}
	tsd.fileCounter = 0
	tsd.bytesWritten = 0
	tsd.currentFilename = ""
	tsd.pad = 0
	tsd.err = nil
}

func (tsd *Digest) encodeHeader(header *tar.Header) error {
	for _, elem := range tsd.headerSelector.selectHeaders(header) {
		if _, err := tsd.entryHash.Write([]byte(elem[0] + elem[1])); err != nil {
			return err
		}
	}
	return nil
}

func (tsd *Digest) Write(p []byte) (n int, err error) {
	var (
		wb      io.Writer
		handler func() error
	)

	switch tsd.digestStage {
	case stageReadHeader:
		wb, handler = &tsd.headerBuffer, tsd.readHeader
	case stageReadEntry:
		wb, handler = &tsd.currentBuffer, tsd.readEntry
	case stageSkipPadding:
		wb, handler = &tsd.currentBuffer, tsd.skipPadding
	case stageFinished:
		return len(p), tsd.err
	default:
		tsd.err = fmt.Errorf("unknown TarSum digest stage: %q", tsd.digestStage)
		tsd.digestStage = stageFinished
		return len(p), tsd.err
	}

	tsd.logDebug("\nwriting %d bytes to digest\n", len(p))
	n, _ = wb.Write(p) // Writes on these buffers always return a nil error.
	tsd.bytesWritten += int64(n)

	if tsd.err = handler(); tsd.err != nil {
		tsd.logDebug("fatal error at stage %s: %s\n\n", tsd.digestStage, tsd.err)
		tsd.digestStage = stageFinished
	}

	return n, tsd.err
}

// Len returns the number of bytes written to this digest.
func (tsd *Digest) Len() int64 {
	return tsd.bytesWritten
}

func (tsd *Digest) isEndOfArchive() bool {
	buf, endLength := tsd.headerBuffer, len(archiveEndBlock)
	return buf.Len() >= endLength && bytes.Equal(buf.Bytes()[:endLength], archiveEndBlock)
}

func (tsd *Digest) readHeader() (err error) {
	tsd.logDebug("reading header with %d bytes\n", tsd.headerBuffer.Len())
	if tsd.headerBuffer.Len() < 2*blockSize {
		// Wait until we have at least two blocks.
		tsd.logDebug("waiting for more header bytes\n")
		return nil
	}

	tsd.currentBuffer.Reset()
	tsd.currentBuffer.Write(tsd.headerBuffer.Bytes())

	tsd.tarReader.Reset(&tsd.currentBuffer)

	var tarHeader *tar.Header
	tarHeader, err = tsd.tarReader.Next()
	if err != nil {
		switch {
		case err == io.EOF && tsd.isEndOfArchive():
			// Signals the end of the archive.
			tsd.logDebug("finished TarSum digest %s with %d bytes left\n\n", tsd.SumString(nil), tsd.currentBuffer.Len())
			tsd.digestStage, err = stageFinished, nil
		case err == io.EOF:
			fallthrough // Treat like an unexpected EOF.
		case err == io.ErrUnexpectedEOF:
			// We weren't able to read the full header of the next entry.
			// This is okay, perhaps the next write will complete the header.
			tsd.logDebug("unable to get header, waitin for more bytes\n")
			err = nil
		default:
			// Some unexpected error.
		}
		return
	}
	tsd.logDebug("got Tar Header for file of size %d bytes\n", tarHeader.Size)

	// Write selected header info to current entry hasher.
	tsd.currentFilename = strings.TrimSuffix(strings.TrimPrefix(tarHeader.Name, "./"), "/")
	if err = tsd.encodeHeader(tarHeader); err != nil {
		return
	}

	// Get the expected padding after the current entry.
	tsd.pad = computeBlockPadding(tarHeader.Size)

	// Continue to process the current entry.
	tsd.digestStage = stageReadEntry
	return tsd.readEntry()
}

func (tsd *Digest) readEntry() (err error) {
	tsd.logDebug("reading entry with %d bytes\n", tsd.currentBuffer.Len())
	if tsd.currentBuffer.Len() == 0 {
		tsd.logDebug("waiting for more entry bytes\n")
		return nil // Nothing to read yet. Wait for more writes.
	}

	// Write current file contents to current entry hasher.
	// If the copy succeeds with no error, then we have reached the
	// end of the current entry. If we get an unexpected EOF error
	// that is okay too, the caller just needs to write more data.
	var n int64
	if n, err = tsd.copyWithBuf(tsd.entryHash, tsd.tarReader); err != nil {
		tsd.logDebug("consumed %d bytes of current entry, waiting for more\n", n)
		if err == io.ErrUnexpectedEOF {
			// We weren't able to read the current entry completely.
			// This is okay, perhaps the next write will complete the
			// current entry.
			err = nil
		}
		return
	}

	tsd.logDebug("consumed %d bytes of current entry, none remain\n", n)

	// We have completed the current archive entry, but there
	// may be some padding before the next Tar archive block!
	tsd.digestStage = stageSkipPadding
	return tsd.skipPadding()
}

func (tsd *Digest) skipPadding() error {
	tsd.logDebug("skipping padding with %d bytes\n", tsd.currentBuffer.Len())
	padding := tsd.currentBuffer.Next(tsd.pad)
	tsd.pad -= len(padding)
	tsd.logDebug("consumed %d bytes of padding,", len(padding))

	if tsd.pad > 0 {
		tsd.logDebug(" waiting for more padding bytes\n")
		// Wait for more writes so we can discard remaining padding.
		return nil
	}

	tsd.logDebug(" no padding remaining\n")

	// Finalize the entry, reset the current entry
	// hasher, incremement the file counter, etc.
	tsd.sums = append(tsd.sums, fileInfoSum{
		name: tsd.currentFilename,
		sum:  hex.EncodeToString(tsd.entryHash.Sum(nil)),
		pos:  tsd.fileCounter,
	})
	tsd.entryHash.Reset()
	tsd.fileCounter++

	// We should now reset the headerBuffer with
	// whatever is left over from the currentBuffer.
	tsd.headerBuffer.Reset()
	tsd.headerBuffer.ReadFrom(&tsd.currentBuffer)

	// Continue to the next entry!
	tsd.digestStage = stageReadHeader
	return tsd.readHeader()
}

// Like `io.Copy` except it only ever does one allocation of the 32K buffer.
func (tsd *Digest) copyWithBuf(dst io.Writer, src io.Reader) (written int64, err error) {
	if tsd.copyBuf == nil {
		tsd.copyBuf = make([]byte, 32*1024)
	}

	for {
		nr, er := src.Read(tsd.copyBuf)
		if nr > 0 {
			nw, ew := dst.Write(tsd.copyBuf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
	}
	return
}

func (tsd *Digest) Finished() bool { return tsd.digestStage == stageFinished }

func (tsd *Digest) Label() string {
	return fmt.Sprintf("%s+%s", tsd.version.String(), "sha256")
}

func (tsd *Digest) Sum(extra []byte) []byte {
	tsd.sums.SortBySums()
	hasher := sha256.New()

	if extra != nil {
		hasher.Write(extra)
	}

	for _, fis := range tsd.sums {
		hasher.Write([]byte(fis.Sum()))
	}

	return hasher.Sum(nil)
}

func (tsd *Digest) SumString(extra []byte) string {
	return fmt.Sprintf("%s:%x", tsd.Label(), tsd.Sum(extra))
}

func (tsd *Digest) State() ([]byte, error) {
	// Critical State/Fields
	// 		version         Version
	//			hashName, finished?
	// 		bytesWritten    int64
	// 		fileCounter     int64
	// 		digestStage     string
	// 		currentFilename string
	// 		pad             int
	// 		headerBuffer    bytes.Buffer
	// 		tarReader       *tar.Reader
	// 		entryHash       sha256.Resumable
	// 		sums            FileInfoSums
	if tsd.err != nil {
		return nil, tsd.err
	}

	buf := new(bytes.Buffer)
	encoder := gob.NewEncoder(buf)

	// Encode the simple stuff first.
	isFinished := tsd.Finished()
	vals := []interface{}{
		tsd.version, "sha256", isFinished,
		tsd.bytesWritten, tsd.fileCounter,
	}

	if !isFinished {
		// Extra fields to save for an unfinished digest.
		vals = append(
			vals, tsd.digestStage, tsd.currentFilename,
			tsd.pad, tsd.headerBuffer.Bytes(),
		)
	}

	for _, val := range vals {
		if err := encoder.Encode(val); err != nil {
			return nil, err
		}
	}

	if !isFinished {
		// Encode the internal tar reader.
		encodeTarReader := tsd.tarReader != nil
		if err := encoder.Encode(encodeTarReader); err != nil {
			return nil, err
		}

		if encodeTarReader {
			tarReaderState, err := tsd.tarReader.State()
			if err != nil {
				return nil, err
			}
			if err = encoder.Encode(tarReaderState); err != nil {
				return nil, err
			}
		}

		// Encode current entry hash state.
		hashState, err := tsd.entryHash.State()
		if err != nil {
			return nil, err
		}
		if err = encoder.Encode(hashState); err != nil {
			return nil, err
		}
	}

	// Encode all FileInfoSums.
	tsd.sums.SortBySums()

	if err := encoder.Encode(len(tsd.sums)); err != nil {
		return nil, err
	}

	for _, fis := range tsd.sums {
		vals := []interface{}{
			fis.Name(), fis.Sum(), fis.Pos(),
		}

		for _, val := range vals {
			if err := encoder.Encode(val); err != nil {
				return nil, err
			}
		}
	}

	return buf.Bytes(), nil
}

func (tsd *Digest) Restore(state []byte) error {
	decoder := gob.NewDecoder(bytes.NewReader(state))

	// Decode the simple stuff first.
	var (
		isFinished bool
		hashType   string
		vals       = []interface{}{
			&tsd.version, &hashType, &isFinished,
			&tsd.bytesWritten, &tsd.fileCounter,
		}
	)

	for _, val := range vals {
		if err := decoder.Decode(val); err != nil {
			return err
		}
	}

	if isFinished {
		tsd.digestStage = stageFinished
	} else {
		// Extra fields for an unfinished digest.
		headerBuf := []byte{}
		vals = []interface{}{
			&tsd.digestStage, &tsd.currentFilename, &tsd.pad, &headerBuf,
		}

		for _, val := range vals {
			if err := decoder.Decode(val); err != nil {
				return err
			}
		}

		tsd.headerBuffer.Write(headerBuf)

		// Decode the internal tar reader.
		var decodeTarReader bool
		if err := decoder.Decode(&decodeTarReader); err != nil {
			return err
		}

		tsd.tarReader.Reset(&tsd.currentBuffer)

		if decodeTarReader {
			var tarReaderState []byte
			if err := decoder.Decode(&tarReaderState); err != nil {
				return err
			}
			if err := tsd.tarReader.Restore(tarReaderState); err != nil {
				return err
			}
		}

		// Decode current entry hash state.
		var hashState []byte
		if err := decoder.Decode(&hashState); err != nil {
			return err
		}
		if err := tsd.entryHash.Restore(hashState); err != nil {
			return err
		}
	}

	// Decode all FileInfoSums.
	var lenSums int
	if err := decoder.Decode(&lenSums); err != nil {
		return err
	}

	tsd.sums = make(fileInfoSums, 0, lenSums)

	for i := 0; i < lenSums; i++ {
		var fis fileInfoSum
		for _, val := range []interface{}{&fis.name, &fis.sum, &fis.pos} {
			if err := decoder.Decode(val); err != nil {
				return err
			}
		}

		tsd.sums = append(tsd.sums, fis)
	}

	return nil
}
