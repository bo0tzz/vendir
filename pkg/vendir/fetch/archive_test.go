// Copyright 2024 The Carvel Authors.
// SPDX-License-Identifier: Apache-2.0

package fetch_test

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	ctlfetch "carvel.dev/vendir/pkg/vendir/fetch"
	"github.com/stretchr/testify/require"
)

const (
	defaultDirMode     = 0755
	defaultSymlinkMode = 0777
	defaultFileMode    = 0644
	noMode             = 0
	lastCharIndex      = 1
)

// ArchiveEntry represents a simplified entry for creating archives in tests
type ArchiveEntry struct {
	Name     string // Path/name of the entry in the archive
	Type     byte   // Type: tar.TypeReg, tar.TypeDir, tar.TypeSymlink
	Content  string // Content for regular files (ignored for dirs/symlinks)
	Linkname string // Target for symlinks (ignored for files/dirs)
	Mode     int64  // File mode (optional, defaults will be applied)
}

// TarOptions contains options for creating tar archives
type TarOptions struct {
	Gzip bool // Whether to compress the archive with gzip
}

// createTar creates a tar file from a list of ArchiveEntry structs.
// This is a reusable helper for creating test archives.
// Use opts.Gzip to create a gzip-compressed tar.gz file.
func createTar(
	t *testing.T,
	tarPath string,
	entries []ArchiveEntry,
	opts TarOptions,
) {
	t.Helper()

	file, err := os.Create(tarPath)
	require.NoError(t, err)
	defer file.Close()

	var tarWriter *tar.Writer
	var gzipWriter *gzip.Writer

	if opts.Gzip {
		gzipWriter = gzip.NewWriter(file)
		defer gzipWriter.Close()
		tarWriter = tar.NewWriter(gzipWriter)
	} else {
		tarWriter = tar.NewWriter(file)
	}
	defer tarWriter.Close()

	for _, entry := range entries {
		writeEntryToTar(t, tarWriter, entry)
	}
}

func writeEntryToTar(
	t *testing.T,
	tarWriter *tar.Writer,
	entry ArchiveEntry,
) {
	t.Helper()
	mode := getEntryMode(entry)

	switch entry.Type {
	case tar.TypeDir:
		writeTarDir(t, tarWriter, entry.Name, mode)
	case tar.TypeReg:
		writeTarFile(t, tarWriter, entry.Name, entry.Content, mode)
	case tar.TypeSymlink:
		writeTarSymlink(t, tarWriter, entry.Name, entry.Linkname, mode)
	default:
		t.Fatalf("Unknown Entry type %c for entry '%s'", entry.Type, entry.Name)
	}
}

func getEntryMode(entry ArchiveEntry) int64 {
	if entry.Mode == noMode {
		switch entry.Type {
		case tar.TypeDir:
			return defaultDirMode
		case tar.TypeSymlink:
			return defaultSymlinkMode
		default:
			return defaultFileMode
		}
	}
	return entry.Mode
}

func writeTarDir(t *testing.T, w *tar.Writer, name string, mode int64) {
	t.Helper()
	err := w.WriteHeader(&tar.Header{
		Name:     name,
		Mode:     mode,
		Typeflag: tar.TypeDir,
	})
	require.NoError(t, err)
}

func writeTarFile(
	t *testing.T,
	w *tar.Writer,
	name, content string,
	mode int64,
) {
	t.Helper()
	contentBytes := []byte(content)
	err := w.WriteHeader(&tar.Header{
		Name:     name,
		Mode:     mode,
		Size:     int64(len(contentBytes)),
		Typeflag: tar.TypeReg,
	})
	require.NoError(t, err)
	_, err = w.Write(contentBytes)
	require.NoError(t, err)
}

func writeTarSymlink(
	t *testing.T,
	w *tar.Writer,
	name, linkname string,
	mode int64,
) {
	t.Helper()
	err := w.WriteHeader(&tar.Header{
		Name:     name,
		Mode:     mode,
		Typeflag: tar.TypeSymlink,
		Linkname: linkname,
	})
	require.NoError(t, err)
}

// createZip creates a zip file from a list of ArchiveEntry structs.
// Note: ZIP does not support symlinks, entries with Type
// tar.TypeSymlink will be skipped.
func createZip(t *testing.T, zipPath string, entries []ArchiveEntry) {
	t.Helper()

	file, err := os.Create(zipPath)
	require.NoError(t, err)
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	for _, entry := range entries {
		writeEntryToZip(t, zipWriter, entry)
	}
}

func writeEntryToZip(
	t *testing.T,
	zipWriter *zip.Writer,
	entry ArchiveEntry,
) {
	t.Helper()

	switch entry.Type {
	case tar.TypeDir:
		writeZipDir(t, zipWriter, entry.Name)
	case tar.TypeReg:
		writeZipFile(t, zipWriter, entry.Name, entry.Content)
	case tar.TypeSymlink:
		logSkippedSymlink(t, entry.Name)
	default:
		t.Fatalf("Unknown Entry type %c for entry '%s'", entry.Type, entry.Name)
	}
}

func writeZipDir(t *testing.T, w *zip.Writer, name string) {
	t.Helper()
	if name[len(name)-lastCharIndex] != '/' {
		name += "/"
	}
	_, err := w.Create(name)
	require.NoError(t, err)
}

func writeZipFile(t *testing.T, w *zip.Writer, name, content string) {
	t.Helper()
	writer, err := w.Create(name)
	require.NoError(t, err)
	_, err = writer.Write([]byte(content))
	require.NoError(t, err)
}

func logSkippedSymlink(t *testing.T, name string) {
	t.Helper()
	t.Logf(
		"Skipping symlink entry %s (ZIP does not support symlinks)",
		name,
	)
}

// verifyExtractedFile checks file exists and has expected content
func verifyExtractedFile(t *testing.T, path, expectedContent string) {
	t.Helper()
	require.FileExists(t, path)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, expectedContent, string(content))
}

// verifyExtractedSymlink checks that a symlink exists and points
// to the expected target
func verifyExtractedSymlink(t *testing.T, path, expectedTarget string) {
	t.Helper()
	linkInfo, err := os.Lstat(path)
	require.NoError(t, err, "Symlink should exist")
	require.True(t, linkInfo.Mode()&os.ModeSymlink != 0,
		"Should be a symlink")
	target, err := os.Readlink(path)
	require.NoError(t, err)
	require.Equal(t, expectedTarget, target)
}

// setupTestDir creates a temporary directory and returns it
// along with a cleanup function
func setupTestDir(t *testing.T) (tmpDir string, dstPath string) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "vendir-archive-test")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	dstPath = filepath.Join(tmpDir, "extracted")
	require.NoError(t, os.MkdirAll(dstPath, defaultDirMode))

	return tmpDir, dstPath
}

func TestArchiveUnpackTgz(t *testing.T) {
	t.Run("Unpack tgz file with dir, file and symlink", func(t *testing.T) {
		tmpDir, dstPath := setupTestDir(t)

		tgzPath := filepath.Join(tmpDir, "test.tgz")
		createTar(t, tgzPath, []ArchiveEntry{
			{Name: "testdir/", Type: tar.TypeDir},
			{Name: "testdir/file.txt", Type: tar.TypeReg,
				Content: "hello world"},
			{Name: "testdir/subdir/nested.txt", Type: tar.TypeReg,
				Content: "nested content"},
			{Name: "testdir/link", Type: tar.TypeSymlink,
				Linkname: "file.txt"},
		}, TarOptions{Gzip: true})

		unpacked, err := ctlfetch.NewArchive(
			tgzPath, false, "").Unpack(dstPath)
		require.NoError(t, err, "Unpacking should not return an error")
		require.True(t, unpacked,
			"Unpacking should return true indicating success")

		verifyExtractedFile(t,
			filepath.Join(dstPath, "testdir", "file.txt"),
			"hello world")
		verifyExtractedFile(t,
			filepath.Join(dstPath, "testdir", "subdir", "nested.txt"),
			"nested content")
		verifyExtractedSymlink(t,
			filepath.Join(dstPath, "testdir", "link"),
			"file.txt")
	})
}

func TestArchiveUnpackTar(t *testing.T) {
	t.Run("Unpack tar file dir, file and symlink", func(t *testing.T) {
		tmpDir, dstPath := setupTestDir(t)

		tarPath := filepath.Join(tmpDir, "test.tar")
		createTar(t, tarPath, []ArchiveEntry{
			{Name: "testdir/", Type: tar.TypeDir},
			{Name: "testdir/file.txt", Type: tar.TypeReg,
				Content: "hello world"},
			{Name: "testdir/subdir/nested.txt", Type: tar.TypeReg,
				Content: "nested content"},
			{Name: "testdir/link", Type: tar.TypeSymlink,
				Linkname: "file.txt"},
		}, TarOptions{Gzip: false})

		unpacked, err := ctlfetch.NewArchive(
			tarPath, false, "").Unpack(dstPath)
		require.NoError(t, err, "Unpacking should not return an error")
		require.True(t, unpacked,
			"Unpacking should return true indicating success")

		verifyExtractedFile(t,
			filepath.Join(dstPath, "testdir", "file.txt"),
			"hello world")
		verifyExtractedFile(t,
			filepath.Join(dstPath, "testdir", "subdir", "nested.txt"),
			"nested content")
		verifyExtractedSymlink(t,
			filepath.Join(dstPath, "testdir", "link"),
			"file.txt")
	})
}

func TestArchiveUnpackZip(t *testing.T) {
	t.Run("Unpack zip file without symlinks succeeds", func(t *testing.T) {
		tmpDir, dstPath := setupTestDir(t)

		zipPath := filepath.Join(tmpDir, "test.zip")
		createZip(t, zipPath, []ArchiveEntry{
			{Name: "testdir/", Type: tar.TypeDir},
			{Name: "testdir/file.txt", Type: tar.TypeReg,
				Content: "hello world"},
			{Name: "testdir/subdir/nested.txt", Type: tar.TypeReg,
				Content: "nested content"},
		})

		unpacked, err := ctlfetch.NewArchive(
			zipPath, false, "").Unpack(dstPath)
		require.NoError(t, err, "Unpacking should not return an error")
		require.True(t, unpacked,
			"Unpacking should return true indicating success")

		verifyExtractedFile(t,
			filepath.Join(dstPath, "testdir", "file.txt"),
			"hello world")
		verifyExtractedFile(t,
			filepath.Join(dstPath, "testdir", "subdir", "nested.txt"),
			"nested content")
	})
}
