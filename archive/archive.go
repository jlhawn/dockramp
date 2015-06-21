package archive

import (
	"archive/tar"
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/fileutils"
	"github.com/docker/docker/pkg/pools"
	"github.com/docker/docker/pkg/system"
)

type (
	// Archive is a readable archive that must be closed.
	Archive io.ReadCloser
	// ArchiveReader is a readabel archive.
	ArchiveReader io.Reader
	// TarOptions specifis all the options to archive files and directories.
	TarOptions struct {
		IncludeFiles     []string
		ExcludePatterns  []string
		NoLchown         bool
		Name             string
		IncludeSourceDir bool
	}
)

type tarAppender struct {
	TarWriter *tar.Writer
	Buffer    *bufio.Writer

	// for hardlink mapping
	SeenFiles map[uint64]string
}

// canonicalTarName provides a platform-independent and consistent posix-style
//path for files and directories to be archived regardless of the platform.
func canonicalTarName(name string, isDir bool) (string, error) {
	name, err := CanonicalTarNameForPath(name)
	if err != nil {
		return "", err
	}

	// suffix with '/' for directories
	if isDir && !strings.HasSuffix(name, "/") {
		name += "/"
	}
	return name, nil
}

func (ta *tarAppender) addTarFile(path, name string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}

	link := ""
	if fi.Mode()&os.ModeSymlink != 0 {
		if link, err = os.Readlink(path); err != nil {
			return err
		}
	}

	hdr, err := tar.FileInfoHeader(fi, link)
	if err != nil {
		return err
	}
	hdr.Mode = int64(chmodTarEntry(os.FileMode(hdr.Mode)))

	name, err = canonicalTarName(name, fi.IsDir())
	if err != nil {
		return fmt.Errorf("tar: cannot canonicalize path: %v", err)
	}
	hdr.Name = name

	nlink, inode, err := setHeaderForSpecialDevice(hdr, ta, name, fi.Sys())
	if err != nil {
		return err
	}

	// if it's a regular file and has more than 1 link,
	// it's hardlinked, so set the type flag accordingly
	if fi.Mode().IsRegular() && nlink > 1 {
		// a link should have a name that it links too
		// and that linked name should be first in the tar archive
		if oldpath, ok := ta.SeenFiles[inode]; ok {
			hdr.Typeflag = tar.TypeLink
			hdr.Linkname = oldpath
			hdr.Size = 0 // This Must be here for the writer math to add up!
		} else {
			ta.SeenFiles[inode] = name
		}
	}

	capability, _ := system.Lgetxattr(path, "security.capability")
	if capability != nil {
		hdr.Xattrs = make(map[string]string)
		hdr.Xattrs["security.capability"] = string(capability)
	}

	if err := ta.TarWriter.WriteHeader(hdr); err != nil {
		return err
	}

	if hdr.Typeflag == tar.TypeReg {
		file, err := os.Open(path)
		if err != nil {
			return err
		}

		ta.Buffer.Reset(ta.TarWriter)
		defer ta.Buffer.Reset(nil)
		_, err = io.Copy(ta.Buffer, file)
		file.Close()
		if err != nil {
			return err
		}
		err = ta.Buffer.Flush()
		if err != nil {
			return err
		}
	}

	return nil
}

// TarWithOptions creates an archive from the directory at `path`, only including files whose relative
// paths are included in `options.IncludeFiles` (if non-nil) or not in `options.ExcludePatterns`.
func TarWithOptions(srcPath string, options *TarOptions) (io.ReadCloser, error) {

	patterns, patDirs, exceptions, err := fileutils.CleanPatterns(options.ExcludePatterns)

	if err != nil {
		return nil, err
	}

	pipeReader, pipeWriter := io.Pipe()

	go func() {
		ta := &tarAppender{
			TarWriter: tar.NewWriter(pipeWriter),
			Buffer:    pools.BufioWriter32KPool.Get(nil),
			SeenFiles: make(map[uint64]string),
		}

		defer func() {
			// Make sure to check the error on Close.
			if err := ta.TarWriter.Close(); err != nil {
				log.Debugf("Can't close tar writer: %s", err)
			}
			if err := pipeWriter.Close(); err != nil {
				log.Debugf("Can't close pipe writer: %s", err)
			}
		}()

		// this buffer is needed for the duration of this piped stream
		defer pools.BufioWriter32KPool.Put(ta.Buffer)

		// In general we log errors here but ignore them because
		// during e.g. a diff operation the container can continue
		// mutating the filesystem and we can see transient errors
		// from this

		stat, err := os.Lstat(srcPath)
		if err != nil {
			return
		}

		if !stat.IsDir() {
			// We can't later join a non-dir with any includes because the
			// 'walk' will error if "file/." is stat-ed and "file" is not a
			// directory. So, we must split the source path and use the
			// basename as the include.
			if len(options.IncludeFiles) > 0 {
				log.Warn("Tar: Can't archive a file with includes")
			}

			dir, base := SplitPathDirEntry(srcPath)
			srcPath = dir
			options.IncludeFiles = []string{base}
		}

		if len(options.IncludeFiles) == 0 {
			options.IncludeFiles = []string{"."}
		}

		seen := make(map[string]bool)

		var renamedRelFilePath string // For when tar.Options.Name is set
		for _, include := range options.IncludeFiles {
			// We can't use filepath.Join(srcPath, include) because this will
			// clean away a trailing "." or "/" which may be important.
			walkRoot := strings.Join([]string{srcPath, include}, string(filepath.Separator))
			filepath.Walk(walkRoot, func(filePath string, f os.FileInfo, err error) error {
				if err != nil {
					log.Debugf("Tar: Can't stat file %s to tar: %s", srcPath, err)
					return nil
				}

				relFilePath, err := filepath.Rel(srcPath, filePath)
				if err != nil || (!options.IncludeSourceDir && relFilePath == "." && f.IsDir()) {
					// Error getting relative path OR we are looking
					// at the source directory path. Skip in both situations.
					return nil
				}

				if options.IncludeSourceDir && include == "." && relFilePath != "." {
					relFilePath = strings.Join([]string{".", relFilePath}, string(filepath.Separator))
				}

				skip := false

				// If "include" is an exact match for the current file
				// then even if there's an "excludePatterns" pattern that
				// matches it, don't skip it. IOW, assume an explicit 'include'
				// is asking for that file no matter what - which is true
				// for some files, like .dockerignore and Dockerfile (sometimes)
				if include != relFilePath {
					skip, err = fileutils.OptimizedMatches(relFilePath, patterns, patDirs)
					if err != nil {
						log.Debugf("Error matching %s", relFilePath, err)
						return err
					}
				}

				if skip {
					if !exceptions && f.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}

				if seen[relFilePath] {
					return nil
				}
				seen[relFilePath] = true

				// Rename the base resource
				if options.Name != "" && filePath == srcPath+"/"+filepath.Base(relFilePath) {
					renamedRelFilePath = relFilePath
				}
				// Set this to make sure the items underneath also get renamed
				if options.Name != "" {
					relFilePath = strings.Replace(relFilePath, renamedRelFilePath, options.Name, 1)
				}

				if err := ta.addTarFile(filePath, relFilePath); err != nil {
					log.Debugf("Can't add file %s to tar: %s", filePath, err)
				}
				return nil
			})
		}
	}()

	return pipeReader, nil
}
