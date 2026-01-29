package archive

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"chin/internal/crypto"
	"chin/internal/utils"
	"strings"
	"time"
)

const (
	Magic       = "CHIN"
	Version     = 6 // New Version
	MagicLength = 4
	HeaderSize  = 72 // 4+2+2+8+8+32+16
)

const (
	FlagEncrypted = 1 << iota
	FlagSplit
)

type Header struct {
	Magic          [MagicLength]byte
	Version        uint16
	Flags          uint16
	FileCount      uint64
	MetadataOffset uint64
	DataChecksum   [32]byte
	Salt           [16]byte // Master Salt (for Metadata & Key Derivation)
}

type FileEntry struct {
	Name     string
	Size     uint64
	Offset   uint64
	Checksum uint64
	Mode     uint32
	ModTime  time.Time
	IsDir    bool
}

type Metadata struct {
	Version          uint16
	FileCount        uint64
	CreatedAt        time.Time
	DataChecksum     [32]byte
	Files            []FileEntry
	MetadataChecksum [32]byte
}

func (m *Metadata) Serialize() ([]byte, error) {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, m.Version)
	binary.Write(buf, binary.BigEndian, m.FileCount)

	createdUnix := uint64(m.CreatedAt.Unix())
	binary.Write(buf, binary.BigEndian, createdUnix)

	buf.Write(m.DataChecksum[:])

	binary.Write(buf, binary.BigEndian, uint32(len(m.Files)))
	for _, file := range m.Files {
		binary.Write(buf, binary.BigEndian, uint32(len(file.Name)))
		buf.WriteString(file.Name)
		binary.Write(buf, binary.BigEndian, file.Size)
		binary.Write(buf, binary.BigEndian, file.Offset)
		binary.Write(buf, binary.BigEndian, file.Checksum)
		binary.Write(buf, binary.BigEndian, file.Mode)
		if file.IsDir {
			binary.Write(buf, binary.BigEndian, uint8(1))
		} else {
			binary.Write(buf, binary.BigEndian, uint8(0))
		}
		modTimeUnix := uint64(file.ModTime.Unix())
		binary.Write(buf, binary.BigEndian, modTimeUnix)
	}

	return buf.Bytes(), nil
}

func DeserializeMetadata(data []byte) (*Metadata, error) {
	buf := bytes.NewReader(data)

	m := &Metadata{}

	if err := binary.Read(buf, binary.BigEndian, &m.Version); err != nil {
		return nil, fmt.Errorf("reading version: %w", err)
	}

	if err := binary.Read(buf, binary.BigEndian, &m.FileCount); err != nil {
		return nil, fmt.Errorf("reading filecount: %w", err)
	}

	var createdUnix uint64
	if err := binary.Read(buf, binary.BigEndian, &createdUnix); err != nil {
		return nil, fmt.Errorf("reading createdunix: %w", err)
	}
	m.CreatedAt = time.Unix(int64(createdUnix), 0)

	if _, err := buf.Read(m.DataChecksum[:]); err != nil {
		return nil, fmt.Errorf("reading datachecksum: %w", err)
	}

	var fileCount uint32
	if err := binary.Read(buf, binary.BigEndian, &fileCount); err != nil {
		return nil, fmt.Errorf("reading filecount2: %w", err)
	}
	
	// Safety Check: Avoid OOM on corrupted data
	if fileCount > 10_000_000 {
		return nil, fmt.Errorf("file count too large (%d): corrupted metadata", fileCount)
	}

	m.Files = make([]FileEntry, fileCount)
	for i := uint32(0); i < fileCount; i++ {
		var nameLen uint32
		if err := binary.Read(buf, binary.BigEndian, &nameLen); err != nil {
			return nil, fmt.Errorf("reading namelen for file %d: %w", i, err)
		}
		
		// Safety Check: Filename length
		if nameLen > 4096 {
			return nil, fmt.Errorf("filename too long (%d) for file %d: corrupted metadata", nameLen, i)
		}

		nameBytes := make([]byte, nameLen)
		if _, err := buf.Read(nameBytes); err != nil {
			return nil, fmt.Errorf("reading name for file %d: %w", i, err)
		}
		m.Files[i].Name = string(nameBytes)

		if err := binary.Read(buf, binary.BigEndian, &m.Files[i].Size); err != nil {
			return nil, fmt.Errorf("reading size for file %d: %w", i, err)
		}

		if err := binary.Read(buf, binary.BigEndian, &m.Files[i].Offset); err != nil {
			return nil, fmt.Errorf("reading offset for file %d: %w", i, err)
		}

		if err := binary.Read(buf, binary.BigEndian, &m.Files[i].Checksum); err != nil {
			return nil, fmt.Errorf("reading checksum for file %d: %w", i, err)
		}

		if err := binary.Read(buf, binary.BigEndian, &m.Files[i].Mode); err != nil {
			return nil, fmt.Errorf("reading mode for file %d: %w", i, err)
		}

		var isDir uint8
		if err := binary.Read(buf, binary.BigEndian, &isDir); err != nil {
			return nil, fmt.Errorf("reading isdir for file %d: %w", i, err)
		}
		m.Files[i].IsDir = isDir == 1

		var modTimeUnix uint64
		if err := binary.Read(buf, binary.BigEndian, &modTimeUnix); err != nil {
			return nil, fmt.Errorf("reading modtime for file %d: %w", i, err)
		}
		m.Files[i].ModTime = time.Unix(int64(modTimeUnix), 0)
	}

	if _, err := buf.Read(m.MetadataChecksum[:]); err != nil {
		if err == io.EOF {
			return m, nil
		}
		return nil, fmt.Errorf("reading metadatachecksum: %w", err)
	}

	return m, nil
}

var (
	ErrInvalidFormat    = errors.New("invalid chin format")
	ErrInvalidVersion   = errors.New("unsupported version (requires v6)")
	ErrFileNotFound     = errors.New("file not found in archive")
	ErrChecksumMismatch = errors.New("checksum mismatch")
)

type Writer struct {
	file       SplitFile
	dataOffset uint64
	dataHasher hash.Hash
	metadata   Metadata
	password   string
	salt       []byte
	OnProgress func(int)
	OnFileStart func(string)
}

func NewWriter(filename string, password string, splitSize int64) (*Writer, error) {
	var file SplitFile
	var err error

	if splitSize > 0 {
		file, err = NewSplitWriter(filename, splitSize)
	} else {
		file, err = os.Create(filename)
	}
	
	if err != nil {
		return nil, err
	}

	// Generate Master Salt if encrypted
	var salt []byte
	if password != "" {
		salt, err = crypto.GenerateSalt()
		if err != nil {
			file.Close()
			return nil, err
		}
	} else {
		salt = make([]byte, 16)
	}

	// Write Placeholder Header
	header := make([]byte, HeaderSize)
	copy(header[:MagicLength], Magic)
	binary.BigEndian.PutUint16(header[MagicLength:MagicLength+2], Version)
	
	flags := uint16(0)
	if password != "" {
		flags |= FlagEncrypted
	}
	binary.BigEndian.PutUint16(header[MagicLength+2:MagicLength+4], flags)
	copy(header[HeaderSize-16:], salt)

	if _, err := file.Write(header); err != nil {
		file.Close()
		return nil, err
	}

	return &Writer{
		file:       file,
		dataOffset: uint64(HeaderSize),
		dataHasher: utils.NewBlake3(),
		metadata: Metadata{
			Version:   Version,
			CreatedAt: time.Now(),
			Files:     []FileEntry{},
		},
		password: password,
		salt:     salt,
	}, nil
}

func (w *Writer) AddFile(path string, nameInArchive string) error {
	if strings.HasSuffix(strings.ToLower(path), ".chin") {
		return nil 
	}
	if strings.Contains(strings.ToLower(path), ".chin.c") {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return w.addDirectory(path, nameInArchive)
	}

	return w.addSingleFile(path, nameInArchive, info)
}

func (w *Writer) addSingleFile(path, name string, info os.FileInfo) error {
	if w.OnFileStart != nil {
		w.OnFileStart(name)
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	hasher := utils.NewXXHash64()

	offset := w.dataOffset
	size := uint64(info.Size())

	var totalWritten uint64 = 0
	var checksum uint64

	if w.password != "" {
		// Use Streaming Encryption
		pipeReader, pipeWriter := io.Pipe()
		
		plainHasher := utils.NewXXHash64()
		sourceWithHash := io.TeeReader(file, plainHasher)
		
		errChan := make(chan error, 1)
		
		go func() {
			defer pipeWriter.Close()
			// v6: EncryptStream handles file salt generation & writing internally
			err := crypto.EncryptStream(sourceWithHash, pipeWriter, []byte(w.password), w.salt)
			if err != nil {
				pipeWriter.CloseWithError(err)
				errChan <- err
				return
			}
			errChan <- nil
		}()
		
		countingWriter := &utils.CountingWriter{Writer: w.file, Callback: w.OnProgress}
		multiWriter := io.MultiWriter(countingWriter, w.dataHasher)
		
		if _, err := io.Copy(multiWriter, pipeReader); err != nil {
			return err
		}
		
		if err := <-errChan; err != nil {
			return err
		}

		totalWritten = countingWriter.Count
		checksum = plainHasher.Sum64()
		
	} else {
		buf := make([]byte, 64*1024)
		var copied uint64 = 0
		
		for {
			n, err := file.Read(buf)
			if err != nil && err != io.EOF {
				return err
			}
			if n == 0 {
				break
			}

			if _, err := w.file.Write(buf[:n]); err != nil {
				return err
			}
			
			if w.OnProgress != nil {
				w.OnProgress(n)
			}

			w.dataHasher.Write(buf[:n])
			hasher.Write(buf[:n])
			copied += uint64(n)
		}
		totalWritten = copied
		checksum = hasher.Sum64()
	}

	w.metadata.Files = append(w.metadata.Files, FileEntry{
		Name:     name,
		Size:     size,          // Original Size
		Offset:   offset,        // Offset in Archive (start of stream)
		Checksum: checksum,      // Plaintext Checksum
		Mode:     uint32(info.Mode()),
		ModTime:  info.ModTime(),
		IsDir:    false,
	})

	w.dataOffset += totalWritten
	w.metadata.FileCount++

	return nil
}

func (w *Writer) addDirectory(path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	w.metadata.Files = append(w.metadata.Files, FileEntry{
		Name:    name,
		Size:    0,
		Offset:  0,
		Mode:    uint32(info.Mode()),
		ModTime: info.ModTime(),
		IsDir:   true,
	})

	w.metadata.FileCount++

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		fullPath := filepath.Join(path, entry.Name())
		archiveName := filepath.Join(name, entry.Name())

		if entry.IsDir() {
			if err := w.addDirectory(fullPath, archiveName); err != nil {
				return err
			}
		} else {
			if err := w.AddFile(fullPath, archiveName); err != nil {
				return err
			}
		}
	}

	return nil
}

func (w *Writer) Close() error {
	return w.file.Close()
}

func (w *Writer) Finalize(password string) error {
	metadataBytes, err := w.metadata.Serialize()
	if err != nil {
		return err
	}
	dataChecksum := w.dataHasher.Sum(nil)
	copy(w.metadata.DataChecksum[:], dataChecksum)

	metadataChecksum := utils.Blake3(metadataBytes)
	copy(w.metadata.MetadataChecksum[:], metadataChecksum)

	// Encrypt Metadata if needed
	if w.password != "" {
		// Used Master Salt for Metadata Encryption
		encrypted, nonce, err := crypto.Encrypt(metadataBytes, []byte(w.password), w.salt)
		if err != nil {
			return err
		}
		// Combine: [Nonce 12][Ciphertext...]
		combined := make([]byte, len(nonce)+len(encrypted))
		copy(combined, nonce)
		copy(combined[len(nonce):], encrypted)
		metadataBytes = combined
	}

	metadataOffset := w.dataOffset
	w.dataOffset += uint64(len(metadataBytes))
	
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	header := make([]byte, HeaderSize)
	copy(header[:MagicLength], Magic)
	binary.BigEndian.PutUint16(header[MagicLength:MagicLength+2], Version)
	
	flags := uint16(0)
	if w.password != "" {
		flags |= FlagEncrypted
	}
	if _, ok := w.file.(*SplitWriter); ok {
		flags |= FlagSplit
	}
	binary.BigEndian.PutUint16(header[MagicLength+2:MagicLength+4], flags)

	binary.BigEndian.PutUint64(header[MagicLength+4:MagicLength+12], w.metadata.FileCount)
	binary.BigEndian.PutUint64(header[MagicLength+12:MagicLength+20], metadataOffset)
	copy(header[MagicLength+20:MagicLength+52], w.metadata.DataChecksum[:])
	
	if w.password != "" {
		copy(header[HeaderSize-16:], w.salt)
	}

	if _, err := w.file.Write(header); err != nil {
		return err
	}

	if _, err := w.file.Seek(int64(metadataOffset), io.SeekStart); err != nil {
		return err
	}

	if _, err := w.file.Write(metadataBytes); err != nil {
		return err
	}

	currentPos, err := w.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if err := w.file.Truncate(currentPos); err != nil {
		return err
	}

	if err := w.file.Sync(); err != nil {
		return err
	}

	return w.file.Close()
}

type Reader struct {
	file          SplitFile
	header        Header
	metadata      Metadata
	password      string
	salt          []byte
	OnProgress    func(int)
	OnFileStart   func(string)
}

func NewReader(filename string, password string) (*Reader, error) {
	var file SplitFile
	
	tempFile, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	
	headerBytes := make([]byte, HeaderSize)
	if _, err := tempFile.Read(headerBytes); err != nil {
		tempFile.Close()
		return nil, err
	}
	
	flags := binary.BigEndian.Uint16(headerBytes[MagicLength+2 : MagicLength+4])
	if flags&FlagSplit != 0 {
		tempFile.Close()
		file, err = NewSplitReader(filename)
		if err != nil {
			return nil, err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			file.Close()
			return nil, err
		}
		if _, err := file.Read(headerBytes); err != nil {
			file.Close()
			return nil, err
		}
	} else {
		file = tempFile
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			file.Close()
			return nil, err
		}
	}

	var header Header
	copy(header.Magic[:], headerBytes[:MagicLength])
	header.Version = binary.BigEndian.Uint16(headerBytes[MagicLength : MagicLength+2])
	header.Flags = binary.BigEndian.Uint16(headerBytes[MagicLength+2 : MagicLength+4])

	header.FileCount = binary.BigEndian.Uint64(headerBytes[MagicLength+4 : MagicLength+12])
	header.MetadataOffset = binary.BigEndian.Uint64(headerBytes[MagicLength+12 : MagicLength+20])
	copy(header.DataChecksum[:], headerBytes[MagicLength+20:MagicLength+52])
	copy(header.Salt[:], headerBytes[HeaderSize-16:])

	if string(header.Magic[:]) != Magic {
		file.Close()
		return nil, ErrInvalidFormat
	}

	if header.Version != Version {
		file.Close()
		return nil, ErrInvalidVersion
	}

	r := &Reader{
		file:          file,
		header:        header,
		password:      password,
		salt:          header.Salt[:],
	}

	if _, err := file.Seek(int64(header.MetadataOffset), 0); err != nil {
		file.Close()
		return nil, err
	}

	metadataBytes := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			file.Close()
			return nil, err
		}
		if n == 0 {
			break
		}
		metadataBytes = append(metadataBytes, buf[:n]...)
	}

	if len(metadataBytes) == 0 {
		file.Close()
		return nil, errors.New("empty metadata")
	}

	if header.Flags&FlagEncrypted != 0 {
		// New Metadata Format: [Nonce 12][Ciphertext...]
		if len(metadataBytes) < 12 {
			file.Close()
			return nil, errors.New("metadata too short for nonce")
		}
		nonce := metadataBytes[:12]
		ciphertext := metadataBytes[12:]
		
		decrypted, err := crypto.Decrypt(ciphertext, nonce, []byte(r.password), r.salt)
		if err != nil {
			file.Close()
			return nil, err
		}
		metadataBytes = decrypted
	}

	metadata, err := DeserializeMetadata(metadataBytes)
	if err != nil {
		file.Close()
		return nil, err
	}

	r.metadata = *metadata

	return r, nil
}

func (r *Reader) Close() error {
	return r.file.Close()
}

func (r *Reader) SetPassword(password string) {
	r.password = password
}

func (r *Reader) ListFiles() []FileEntry {
	return r.metadata.Files
}

func (r *Reader) ExtractFile(entry FileEntry, outputPath string, verify bool) error {
	// Security: Zip Slip Prevention
	destPath, err := filepath.Abs(outputPath)
	if err != nil {
		return err
	}
	
	fullPath := filepath.Join(destPath, entry.Name)
	if !strings.HasPrefix(fullPath, destPath+string(os.PathSeparator)) && fullPath != destPath {
		// Attempted Zip Slip
		return fmt.Errorf("security error: illegal file path '%s'", entry.Name)
	}

	if entry.IsDir {
		if err := os.MkdirAll(fullPath, os.FileMode(entry.Mode)); err != nil {
			return err
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}

	// Ensure we can overwrite if exists (handle read-only files)
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		// Try to chmod if remove failed (might be read-only on Windows)
		os.Chmod(fullPath, 0666)
		if err := os.Remove(fullPath); err != nil {
			return err
		}
	}

	if r.OnFileStart != nil {
		r.OnFileStart(entry.Name)
	}

	outFile, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	if _, err := r.file.Seek(int64(entry.Offset), io.SeekStart); err != nil {
		return err
	}

	// Call helper to extract data
	if r.header.Flags&FlagEncrypted != 0 {
		err = r.extractFileEncrypted(entry, outFile, verify)
	} else {
		err = r.extractFilePlain(entry, outFile, verify)
	}
	
	if err != nil {
		return err
	}
	
	// Apply metadata
	if err := outFile.Chmod(os.FileMode(entry.Mode)); err != nil {
		return err
	}

	// Close explicitly before Chtimes (important on Windows)
	if err := outFile.Close(); err != nil {
		return err
	}

	return os.Chtimes(fullPath, entry.ModTime, entry.ModTime)
}

func (r *Reader) extractFilePlain(entry FileEntry, outFile *os.File, verify bool) error {
	if _, err := r.file.Seek(int64(entry.Offset), io.SeekStart); err != nil {
		return err
	}

	hasher := utils.NewXXHash64()
	tee := io.MultiWriter(outFile, hasher)

	buf := make([]byte, 64*1024)
	remaining := entry.Size

	for remaining > 0 {
		readSize := uint64(len(buf))
		if remaining < readSize {
			readSize = remaining
		}

		n, err := r.file.Read(buf[:readSize])
		if err != nil && err != io.EOF {
			return err
		}

		if _, err := tee.Write(buf[:n]); err != nil {
			return err
		}

		if r.OnProgress != nil {
			r.OnProgress(n)
		}

		remaining -= uint64(n)
	}
	
	if verify {
		if hasher.Sum64() != entry.Checksum {
			return ErrChecksumMismatch
		}
	}
	
	return nil
}

func (r *Reader) extractFileEncrypted(entry FileEntry, outFile *os.File, verify bool) error {
	if _, err := r.file.Seek(int64(entry.Offset), io.SeekStart); err != nil {
		return err
	}

	var writer io.Writer = outFile
	var hasher hash.Hash64
	
	if verify {
		hasher = utils.NewXXHash64()
		writer = io.MultiWriter(outFile, hasher)
	}

	if r.OnProgress != nil {
		writer = &utils.CountingWriter{
			Writer:   writer,
			Callback: r.OnProgress,
		}
	}

	err := crypto.DecryptStream(r.file, writer, []byte(r.password), r.salt)
	if err != nil {
		return err
	}
	
	// Verify checksum
	if verify {
		if hasher.Sum64() != entry.Checksum {
			return ErrChecksumMismatch
		}
	}
	return nil
}

func (r *Reader) ExtractAll(outputPath string, verify bool) error {
	for _, entry := range r.metadata.Files {
		if err := r.ExtractFile(entry, outputPath, verify); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reader) Verify() error {
	dataBytes := make([]byte, r.header.MetadataOffset-uint64(HeaderSize))
	if _, err := r.file.Seek(int64(HeaderSize), io.SeekStart); err != nil {
		return err
	}
	if _, err := r.file.Read(dataBytes); err != nil {
		return err
	}

	dataChecksum := utils.Blake3(dataBytes)
	for i := 0; i < 32; i++ {
		if dataChecksum[i] != r.header.DataChecksum[i] {
			return ErrChecksumMismatch
		}
	}

	for _, entry := range r.metadata.Files {
		if entry.IsDir {
			continue
		}

		if _, err := r.file.Seek(int64(entry.Offset), io.SeekStart); err != nil {
			return err
		}

		hasher := utils.NewXXHash64()
		remaining := entry.Size
		buf := make([]byte, 64*1024)

		for remaining > 0 {
			readSize := uint64(len(buf))
			if remaining < readSize {
				readSize = remaining
			}

			n, err := r.file.Read(buf[:readSize])
			if err != nil && err != io.EOF {
				return err
			}

			hasher.Write(buf[:n])
			remaining -= uint64(n)
		}

		checksum := hasher.Sum64()
		if checksum != entry.Checksum {
			return ErrChecksumMismatch
		}
	}

	return nil
}

func DecryptMetadata(data []byte, nonce []byte, password string, salt []byte) ([]byte, error) {
	return crypto.Decrypt(data, nonce, []byte(password), salt)
}

func TestEncrypt(data []byte, password string, salt []byte) ([]byte, []byte, error) {
	return crypto.Encrypt(data, []byte(password), salt)
}

func (r *Reader) FindFile(name string) (*FileEntry, bool) {
	for i := range r.metadata.Files {
		if r.metadata.Files[i].Name == name {
			return &r.metadata.Files[i], true
		}
	}
	return nil, false
}
