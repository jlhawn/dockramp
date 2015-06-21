package tarsum

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"

	"github.com/jlhawn/tarsum/archive/tar"
)

func TestImplementsHash(t *testing.T) {
	tsd, err := NewDigest(Version1)
	if err != nil {
		t.Fatal(err)
	}

	_ = hash.Hash(tsd)
}

// TestEmptyTarSumDigest tests that tarsum digest does not fail to read
// an empty tar and correctly returns the hex digest of an empty hash.
func TestEmptyTarSumDigest(t *testing.T) {
	// An empty tar archive is exactly 1024 zero bytes.
	zeroBlock := make([]byte, 1024)

	ts, err := NewDigest(Version1)
	if err != nil {
		t.Fatal(err)
	}

	n, err := io.Copy(ts, bytes.NewReader(zeroBlock))
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(zeroBlock)) {
		t.Fatalf("tarSum did not write the correct number of zeroed bytes: %d", n)
	}

	expectedSum := sha256.New().Sum(nil)
	resultSum := ts.Sum(nil)

	if !bytes.Equal(resultSum, expectedSum) {
		t.Fatalf("expected [%x] but got [%x]", expectedSum, resultSum)
	}

	// Ensure that we finished.
	if !ts.Finished() {
		t.Error("tarSum digest is not finished when it shoud be!")
	}

	// Test without ever actually writing anything.
	ts, err = NewDigest(Version1)
	if err != nil {
		t.Fatal(err)
	}

	resultSum = ts.Sum(nil)

	if !bytes.Equal(resultSum, expectedSum) {
		t.Fatalf("expected [%s] but got [%s]", expectedSum, resultSum)
	}
}

func TestTarSumsDigest(t *testing.T) {
	for _, layer := range testLayers {
		if layer.hash != nil {
			// Only support sha256 for now.
			continue
		}

		var (
			fh  io.Reader
			err error
		)

		if len(layer.filename) > 0 {
			fh, err = os.Open(layer.filename)
			if err != nil {
				t.Errorf("failed to open %s: %s", layer.filename, err)
				continue
			}
		} else if layer.options != nil {
			fh = sizedTar(*layer.options)
		} else {
			// What else is there to test?
			t.Errorf("what to do with %#v", layer)
			continue
		}

		if file, ok := fh.(*os.File); ok {
			defer file.Close()
		}

		ts, err := NewDigest(layer.version)
		if err != nil {
			t.Error(err)
			continue
		}

		// Write all bytes into the file.
		_, err = io.Copy(ts, fh)
		if err != nil {
			t.Errorf("failed to copy from %s: %s", layer.filename, err)
			continue
		}

		var gotSum []byte
		if len(layer.jsonfile) > 0 {
			jfh, err := os.Open(layer.jsonfile)
			if err != nil {
				t.Errorf("failed to open %s: %s", layer.jsonfile, err)
				continue
			}
			buf, err := ioutil.ReadAll(jfh)
			if err != nil {
				t.Errorf("failed to readAll %s: %s", layer.jsonfile, err)
				continue
			}
			gotSum = ts.Sum(buf)
		} else {
			gotSum = ts.Sum(nil)
		}

		if layer.tarsum != fmt.Sprintf("%s:%x", ts.Label(), gotSum) {
			t.Errorf("expecting [%s], but got [%s]", layer.tarsum, gotSum)
		}
	}
}

func TestIterationDigest(t *testing.T) {
	headerTests := []struct {
		expectedSum string // TODO(vbatts) it would be nice to get individual sums of each
		version     Version
		hdr         *tar.Header
		data        []byte
	}{
		{
			"tarsum+sha256:626c4a2e9a467d65c33ae81f7f3dedd4de8ccaee72af73223c4bc4718cbc7bbd",
			Version0,
			&tar.Header{
				Name:     "file.txt",
				Size:     0,
				Typeflag: tar.TypeReg,
				Devminor: 0,
				Devmajor: 0,
			},
			[]byte(""),
		},
		{
			"tarsum.v1+sha256:6ffd43a1573a9913325b4918e124ee982a99c0f3cba90fc032a65f5e20bdd465",
			Version1,
			&tar.Header{
				Name:     "file.txt",
				Size:     0,
				Typeflag: tar.TypeReg,
				Devminor: 0,
				Devmajor: 0,
			},
			[]byte(""),
		},
		{
			"tarsum.v1+sha256:b38166c059e11fb77bef30bf16fba7584446e80fcc156ff46d47e36c5305d8ef",
			Version1,
			&tar.Header{
				Name:     "another.txt",
				Uid:      1000,
				Gid:      1000,
				Uname:    "slartibartfast",
				Gname:    "users",
				Size:     4,
				Typeflag: tar.TypeReg,
				Devminor: 0,
				Devmajor: 0,
			},
			[]byte("test"),
		},
		{
			"tarsum.v1+sha256:4cc2e71ac5d31833ab2be9b4f7842a14ce595ec96a37af4ed08f87bc374228cd",
			Version1,
			&tar.Header{
				Name:     "xattrs.txt",
				Uid:      1000,
				Gid:      1000,
				Uname:    "slartibartfast",
				Gname:    "users",
				Size:     4,
				Typeflag: tar.TypeReg,
				Xattrs: map[string]string{
					"user.key1": "value1",
					"user.key2": "value2",
				},
			},
			[]byte("test"),
		},
		{
			"tarsum.v1+sha256:65f4284fa32c0d4112dd93c3637697805866415b570587e4fd266af241503760",
			Version1,
			&tar.Header{
				Name:     "xattrs.txt",
				Uid:      1000,
				Gid:      1000,
				Uname:    "slartibartfast",
				Gname:    "users",
				Size:     4,
				Typeflag: tar.TypeReg,
				Xattrs: map[string]string{
					"user.KEY1": "value1", // adding different case to ensure different sum
					"user.key2": "value2",
				},
			},
			[]byte("test"),
		},
		{
			"tarsum+sha256:c12bb6f1303a9ddbf4576c52da74973c00d14c109bcfa76b708d5da1154a07fa",
			Version0,
			&tar.Header{
				Name:     "xattrs.txt",
				Uid:      1000,
				Gid:      1000,
				Uname:    "slartibartfast",
				Gname:    "users",
				Size:     4,
				Typeflag: tar.TypeReg,
				Xattrs: map[string]string{
					"user.NOT": "CALCULATED",
				},
			},
			[]byte("test"),
		},
	}
	for _, htest := range headerTests {
		s, err := renderDigestSumForHeader(htest.version, htest.hdr, htest.data)
		if err != nil {
			t.Fatal(err)
		}

		if s != htest.expectedSum {
			t.Errorf("expected sum: %q, got: %q", htest.expectedSum, s)
		}
	}

}

func renderDigestSumForHeader(v Version, h *tar.Header, data []byte) (string, error) {
	// First, create the digester.
	ts, err := NewDigest(v)
	if err != nil {
		return "", err
	}

	// Then build our test tar, writing to the digest.
	tw := tar.NewWriter(ts)
	if err := tw.WriteHeader(h); err != nil {
		return "", err
	}
	if _, err := tw.Write(data); err != nil {
		return "", err
	}
	tw.Close()

	// Ensure that we finished.
	if !ts.Finished() {
		return "", errors.New("tarSum digest is not finished when it shoud be!")
	}

	return fmt.Sprintf("%s:%x", ts.Label(), ts.Sum(nil)), nil
}

func TestDigestStateRestore(t *testing.T) {
	tarBuf := new(bytes.Buffer)
	n, err := io.Copy(tarBuf, sizedTar(sizedOptions{16, 1024 * 1024, true, false, true}))
	if err != nil {
		t.Fatal(err)
	}

	tarReader := bytes.NewReader(tarBuf.Bytes())

	// Treat the original read-through TarSum as the 'golden' sum value.
	tarSumReader, err := newTarSum(tarReader, true, Version1)
	if err != nil {
		t.Fatal(err)
	}

	m, err := io.Copy(ioutil.Discard, tarSumReader)
	if err != nil {
		t.Fatal(err)
	}
	if m != n {
		t.Fatalf("wrote %d bytes, expected %d", m, n)
	}
	tarReader.Seek(0, 0)
	goldenSum := tarSumReader.Sum(nil)

	digest, err := NewDigest(Version1)
	if err != nil {
		t.Fatal(err)
	}

	m, err = io.Copy(digest, tarReader)
	if err != nil {
		t.Fatal(err)
	}
	if m != n {
		t.Fatalf("wrote %d bytes, expected %d", m, n)
	}
	if !digest.Finished() {
		t.Fatal("digest not finished when it should be")
	}
	tarReader.Seek(0, 0)
	digestSum := digest.SumString(nil)
	if digestSum != goldenSum {
		t.Fatalf("tarSumReader and digest sum did not match: expected %s but got %s", goldenSum, digestSum)
	}
	digest.Reset()
	digest.toggleDebug()

	m = 0
	for m < n {
		bufSize := rand.Int()%(10*blockSize-1) + 5*blockSize
		nn, err := digest.Write(tarBuf.Next(bufSize))
		if err != nil {
			t.Fatalf("error after %d bytes: %s", digest.Len(), err)
		}
		m += int64(nn)
		if nn != bufSize && m != n {
			t.Fatalf("didn't write enough bytes: wrote %d, expected %d", m, n)
		}

		// Get State and Restore.
		state, err := digest.State()
		if err != nil {
			t.Fatalf("unable to save digest state: %s", err)
		}
		digest.Reset()
		if err = digest.Restore(state); err != nil {
			t.Fatalf("unable to restore digest state: %s", err)
		}
	}

	if !digest.Finished() {
		t.Fatal("digest not finished when it should be")
	}
	finalSum := digest.SumString(nil)

	if finalSum != goldenSum {
		t.Fatalf("stateRestoreDigest and golden sum did not match: expected %s but got %s", goldenSum, finalSum)
	} else {
		t.Logf("Success! %s:", finalSum)
		finalState, _ := digest.State()
		ratio := float32(len(finalState)) / float32(n)
		t.Logf("Final state size: %d bytes for %d byte archive, ratio: %f", len(finalState), n, ratio)
	}
}

func Benchmark9kTarDigest(b *testing.B) {
	buf := bytes.NewBuffer([]byte{})
	fh, err := os.Open("testdata/46af0962ab5afeb5ce6740d4d91652e69206fc991fd5328c1a94d364ad00e457/layer.tar")
	if err != nil {
		b.Error(err)
		return
	}
	n, err := io.Copy(buf, fh)
	fh.Close()

	b.SetBytes(n)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ts, err := NewDigest(Version1)
		if err != nil {
			b.Error(err)
			return
		}

		nw, err := io.Copy(ts, bytes.NewReader(buf.Bytes()))

		if err != nil {
			b.Error(err)
			return
		}

		if nw != n {
			b.Errorf("wrote %d bytes, expected %d", nw, n)
			return
		}

		ts.Sum(nil)
	}
}

// this is a single big file in the tar archive
func Benchmark1mbSingleFileTarDigest(b *testing.B) {
	benchmarkTarDigest(b, sizedOptions{1, 1024 * 1024, true, true, false})
}

// this is 1024 1k files in the tar archive
func Benchmark1kFilesTarDigest(b *testing.B) {
	benchmarkTarDigest(b, sizedOptions{1024, 1024, true, true, false})
}

func benchmarkTarDigest(b *testing.B, opts sizedOptions) {
	var tarReader io.ReadSeeker

	tarReader, ok := sizedTar(opts).(io.ReadSeeker)
	if !ok {
		b.Fatal("sizedTar did not return an io.ReadSeeker")
	}

	if f, ok := tarReader.(*os.File); ok {
		defer os.Remove(f.Name())
		defer f.Close()
	}

	b.SetBytes(opts.size * opts.num)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ts, err := NewDigest(Version1)
		if err != nil {
			b.Error(err)
			return
		}

		_, err = io.Copy(ts, tarReader)

		if err != nil {
			b.Error(err)
			return
		}

		ts.Sum(nil)
		tarReader.Seek(0, 0)
	}
}
