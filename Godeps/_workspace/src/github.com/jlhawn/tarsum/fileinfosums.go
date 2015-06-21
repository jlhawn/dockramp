package tarsum

import "sort"

// This info will be accessed through interface so the actual name and sum cannot be medled with
type fileInfoSumInterface interface {
	// File name
	Name() string
	// Checksum of this particular file and its headers
	Sum() string
	// Position of file in the tar
	Pos() int64
}

type fileInfoSum struct {
	name string
	sum  string
	pos  int64
}

func (fis fileInfoSum) Name() string {
	return fis.name
}
func (fis fileInfoSum) Sum() string {
	return fis.sum
}
func (fis fileInfoSum) Pos() int64 {
	return fis.pos
}

type fileInfoSums []fileInfoSumInterface

// GetFile returns the first FileInfoSumInterface with a matching name
func (fis fileInfoSums) GetFile(name string) fileInfoSumInterface {
	for i := range fis {
		if fis[i].Name() == name {
			return fis[i]
		}
	}
	return nil
}

// GetAllFile returns a FileInfoSums with all matching names
func (fis fileInfoSums) GetAllFile(name string) fileInfoSums {
	f := fileInfoSums{}
	for i := range fis {
		if fis[i].Name() == name {
			f = append(f, fis[i])
		}
	}
	return f
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func (fis fileInfoSums) GetDuplicatePaths() (dups fileInfoSums) {
	seen := make(map[string]int, len(fis)) // allocate earl. no need to grow this map.
	for i := range fis {
		f := fis[i]
		if _, ok := seen[f.Name()]; ok {
			dups = append(dups, f)
		} else {
			seen[f.Name()] = 0
		}
	}
	return dups
}

func (fis fileInfoSums) Len() int      { return len(fis) }
func (fis fileInfoSums) Swap(i, j int) { fis[i], fis[j] = fis[j], fis[i] }

func (fis fileInfoSums) SortByPos() {
	sort.Sort(byPos{fis})
}

func (fis fileInfoSums) SortByNames() {
	sort.Sort(byName{fis})
}

func (fis fileInfoSums) SortBySums() {
	dups := fis.GetDuplicatePaths()
	if len(dups) > 0 {
		sort.Sort(bySum{fis, dups})
	} else {
		sort.Sort(bySum{fis, nil})
	}
}

// byName is a sort.Sort helper for sorting by file names.
// If names are the same, order them by their appearance in the tar archive
type byName struct{ fileInfoSums }

func (bn byName) Less(i, j int) bool {
	if bn.fileInfoSums[i].Name() == bn.fileInfoSums[j].Name() {
		return bn.fileInfoSums[i].Pos() < bn.fileInfoSums[j].Pos()
	}
	return bn.fileInfoSums[i].Name() < bn.fileInfoSums[j].Name()
}

// bySum is a sort.Sort helper for sorting by the sums of all the fileinfos in the tar archive
type bySum struct {
	fileInfoSums
	dups fileInfoSums
}

func (bs bySum) Less(i, j int) bool {
	if bs.dups != nil && bs.fileInfoSums[i].Name() == bs.fileInfoSums[j].Name() {
		return bs.fileInfoSums[i].Pos() < bs.fileInfoSums[j].Pos()
	}
	return bs.fileInfoSums[i].Sum() < bs.fileInfoSums[j].Sum()
}

// byPos is a sort.Sort helper for sorting by the sums of all the fileinfos by their original order
type byPos struct{ fileInfoSums }

func (bp byPos) Less(i, j int) bool {
	return bp.fileInfoSums[i].Pos() < bp.fileInfoSums[j].Pos()
}
