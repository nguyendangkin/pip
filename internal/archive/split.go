package archive

import (
	"fmt"
	"io"
	"os"
)

// SplitFile interface covers both *os.File and our custom split logic
type SplitFile interface {
	io.ReadWriteSeeker
	io.Closer
	io.ReaderAt
	Truncate(size int64) error
	Sync() error
}

// SplitWriter handles writing across multiple files
type SplitWriter struct {
	basePath    string
	maxSize     int64
	currentFile *os.File
	partIndex   int
	currentSize int64
	totalSize   int64 // Virtual position
	openedFiles map[int]*os.File
}

func NewSplitWriter(basePath string, maxSize int64) (*SplitWriter, error) {
	// First file is just .chin
	f, err := os.Create(basePath)
	if err != nil {
		return nil, err
	}

	return &SplitWriter{
		basePath:    basePath,
		maxSize:     maxSize,
		currentFile: f,
		partIndex:   0,
		currentSize: 0,
		totalSize:   0,
		openedFiles: map[int]*os.File{0: f},
	}, nil
}

func (s *SplitWriter) Write(p []byte) (n int, err error) {
	if s.maxSize <= 0 {
		// No split limit
		n, err = s.currentFile.Write(p)
		s.totalSize += int64(n)
		s.currentSize += int64(n)
		return n, err
	}

	totalWritten := 0
	for len(p) > 0 {
		remainingSpace := s.maxSize - s.currentSize
		if remainingSpace <= 0 {
			if err := s.rotate(); err != nil {
				return totalWritten, err
			}
			remainingSpace = s.maxSize
		}

		toWrite := int64(len(p))
		if toWrite > remainingSpace {
			toWrite = remainingSpace
		}

		n, err := s.currentFile.Write(p[:toWrite])
		totalWritten += n
		s.currentSize += int64(n)
		s.totalSize += int64(n)
		
		if err != nil {
			return totalWritten, err
		}

		p = p[n:]
	}
	return totalWritten, nil
}

func (s *SplitWriter) rotate() error {
	s.partIndex++
	filename := fmt.Sprintf("%s.c%02d", s.basePath, s.partIndex) // .chin.c01
	
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	
	s.currentFile = f
	s.openedFiles[s.partIndex] = f
	s.currentSize = 0
	return nil
}

// Seek handles virtual seeking across files
func (s *SplitWriter) Seek(offset int64, whence int) (int64, error) {
	if s.maxSize <= 0 {
		newPos, err := s.currentFile.Seek(offset, whence)
		s.totalSize = newPos
		s.currentSize = newPos // Assuming simple case for single file
		return newPos, err
	}

	var absOffset int64
	switch whence {
	case io.SeekStart:
		absOffset = offset
	case io.SeekCurrent:
		absOffset = s.totalSize + offset
	case io.SeekEnd:
		// Not strictly supported easily without tracking all file sizes, 
		// but usually we seek to 0 or know exact offset.
		// For simplicity/current usage: we don't use SeekEnd in Writer logic except Truncate.
		// Let's assume unsupported or implement naive global size if needed.
		// Currently archive.go uses Seek(0, Start) and Seek(MetaOffset, Start).
		absOffset = s.getVirtualEnd() + offset
	default:
		return 0, fmt.Errorf("invalid whence")
	}

	s.totalSize = absOffset
	
	// Determine which part contains this offset
	targetPart := int(absOffset / s.maxSize)
	offsetInPart := absOffset % s.maxSize
	
	// If seeking to exactly the end of a part, it technically belongs to start of next?
	// But logically bytes [0..max-1] in part 0.
	
	if targetPart != s.partIndex {
		// Switch file
		f, ok := s.openedFiles[targetPart]
		if !ok {
			// Need to open it?
			// Writer usually creates sequentially. Random write seeking not fully supported 
			// unless file already exists.
			// But archive.go only Seeks back to Part 0 (Header).
			// So usually we go back to Part 0.
			
			// Re-open if necessary (if we closed it? we didn't close in rotate logic above)
			// My rotate logic kept them in map.
			return 0, fmt.Errorf("seek to unopened part %d", targetPart)
		}
		s.currentFile = f
		s.partIndex = targetPart
	}
	
	_, err := s.currentFile.Seek(offsetInPart, io.SeekStart)
	if err != nil {
		return 0, err
	}
	
	s.currentSize = offsetInPart
	return absOffset, nil
}

func (s *SplitWriter) getVirtualEnd() int64 {
	// Calculate total size based on parts
	// Simple approximation if we fill parts:
	// (partIndex * maxSize) + currentSize
	return (int64(s.partIndex) * s.maxSize) + s.currentSize
}

func (s *SplitWriter) Close() error {
	for _, f := range s.openedFiles {
		f.Close()
	}
	return nil
}

func (s *SplitWriter) Truncate(size int64) error {
	// Only support truncating current file for now
	// Logic in Finalize: Truncate at current position.
	if s.maxSize <= 0 {
		return s.currentFile.Truncate(size)
	}
	// For split, we might need to delete later parts?
	// Assuming we only truncate a few bytes at end of last part.
	return s.currentFile.Truncate(s.currentSize)
}

func (s *SplitWriter) Sync() error {
	for _, f := range s.openedFiles {
		f.Sync()
	}
	return nil
}

func (s *SplitWriter) Read(p []byte) (n int, err error) {
	// Minimal read support for R/W mode if needed
	return s.currentFile.Read(p)
}

func (s *SplitWriter) ReadAt(p []byte, off int64) (n int, err error) {
	// Not implemented for Writer
	return 0, fmt.Errorf("ReadAt not supported in SplitWriter")
}


// SplitReader handles reading split archives
type SplitReader struct {
	basePath string
	parts    []*os.File
	sizes    []int64
	totalSize int64
	currentPart int
	currentOffset int64 // global virtual offset
}

func NewSplitReader(basePath string) (*SplitReader, error) {
	// Open .chin
	f0, err := os.Open(basePath)
	if err != nil {
		return nil, err
	}
	
	parts := []*os.File{f0}
	
	// Detect other parts
	i := 1
	for {
		name := fmt.Sprintf("%s.c%02d", basePath, i)
		f, err := os.Open(name)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return nil, err
		}
		parts = append(parts, f)
		i++
	}
	
	// Calculate sizes
	sizes := make([]int64, len(parts))
	var total int64
	for i, f := range parts {
		info, _ := f.Stat()
		sizes[i] = info.Size()
		total += sizes[i]
	}

	return &SplitReader{
		basePath: basePath,
		parts:    parts,
		sizes:    sizes,
		totalSize: total,
	}, nil
}

func (r *SplitReader) Read(p []byte) (n int, err error) {
	// Read from current part
	if r.currentPart >= len(r.parts) {
		return 0, io.EOF
	}
	
	// Check if current part exhausted (should track offset within part?)
	// Let's use Seek to manage global position.
	
	// Easier: Just forward read to current OS file pointer?
	// But we need to switch file.
	
	// Let's rely on Seek state.
	// We need to know where we are in current file.
	// But `os.File` tracks its own offset.
	// When we switch to Next part, we read from 0.
	
	f := r.parts[r.currentPart]
	n, err = f.Read(p)
	
	if n > 0 {
		r.currentOffset += int64(n)
	}
	
	if err == io.EOF {
		// Try next part
		if r.currentPart < len(r.parts)-1 {
			r.currentPart++
			// Seek next part to 0 just in case
			r.parts[r.currentPart].Seek(0, io.SeekStart)
			
			// Recurse to read remaining bytes
			n2, err2 := r.Read(p[n:])
			return n + n2, err2
		}
	}
	
	return n, err
}

func (r *SplitReader) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.currentOffset + offset
	case io.SeekEnd:
		abs = r.totalSize + offset
	}
	
	if abs < 0 {
		return 0, fmt.Errorf("negative position")
	}
	
	// Find part
	var currentTotal int64
	found := false
	for i, size := range r.sizes {
		if abs < currentTotal + size {
			// In this part
			partOffset := abs - currentTotal
			r.currentPart = i
			r.parts[i].Seek(partOffset, io.SeekStart)
			found = true
			break
		}
		currentTotal += size
	}
	
	if !found && abs == r.totalSize {
		// EOF
		r.currentPart = len(r.parts) - 1
		r.parts[r.currentPart].Seek(r.sizes[r.currentPart], io.SeekStart)
	} else if !found {
		return 0, fmt.Errorf("seek past end")
	}
	
	r.currentOffset = abs
	return abs, nil
}

func (r *SplitReader) Close() error {
	for _, f := range r.parts {
		f.Close()
	}
	return nil
}

func (r *SplitReader) ReadAt(p []byte, off int64) (n int, err error) {
	// Helper for direct reading, verifying
	// Not critical for ExtractFile if we use Reader interface, 
	// but EncryptStream/DecryptStream might need basic Read.
	
	// Implement simple Seek+Read safe restoration?
	// ReadAt shouldn't change state.
	
	oldPos := r.currentOffset
	_, err = r.Seek(off, io.SeekStart)
	if err != nil {
		return 0, err
	}
	n, err = r.Read(p)
	
	r.Seek(oldPos, io.SeekStart)
	return n, err
}

func (r *SplitReader) Sync() error { return nil }
func (r *SplitReader) Truncate(size int64) error { return nil } // Read only
func (r *SplitReader) Write(p []byte) (n int, err error) {
	return 0, fmt.Errorf("write not supported on SplitReader")
}
