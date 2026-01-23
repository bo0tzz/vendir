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

// ArchiveEntry represents a simplified entry for creating archives in tests
type ArchiveEntry struct {
	Name     string // Path/name of the entry in the archive
	Type     byte   // Type of entry: tar.TypeReg, tar.TypeDir, or tar.TypeSymlink
	Content  string // Content for regular files (ignored for dirs/symlinks)
	Linkname string // Target for symlinks (ignored for files/dirs)
	Mode     int64  // File mode (optional, defaults will be applied)
}

// TarOptions contains options for creating tar archives
type TarOptions struct {
	Gzip bool // Whether to compress the archive with gzip
}

// createTar creates a tar file from a list of ArchiveEntry structs.
// This is a reusable helper for creating test archives with various contents.
// Use opts.Gzip to create a gzip-compressed tar.gz file.
func createTar(t *testing.T, tarPath string, entries []ArchiveEntry, opts TarOptions) {
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
		mode := entry.Mode
		if mode == 0 {
			// Apply default modes based on type
			switch entry.Type {
			case tar.TypeDir:
				mode = 0755
			case tar.TypeSymlink:
				mode = 0777
			default:
				mode = 0644
			}
		}

		switch entry.Type {
		case tar.TypeDir:
			err = tarWriter.WriteHeader(&tar.Header{
				Name:     entry.Name,
				Mode:     mode,
				Typeflag: tar.TypeDir,
			})
			require.NoError(t, err)

		case tar.TypeReg:
			content := []byte(entry.Content)
			err = tarWriter.WriteHeader(&tar.Header{
				Name:     entry.Name,
				Mode:     mode,
				Size:     int64(len(content)),
				Typeflag: tar.TypeReg,
			})
			require.NoError(t, err)
			_, err = tarWriter.Write(content)
			require.NoError(t, err)

		case tar.TypeSymlink:
			err = tarWriter.WriteHeader(&tar.Header{
				Name:     entry.Name,
				Mode:     mode,
				Typeflag: tar.TypeSymlink,
				Linkname: entry.Linkname,
			})
			require.NoError(t, err)
		}
	}
}

// createZip creates a zip file from a list of ArchiveEntry structs.
// Note: ZIP does not support symlinks, so ArchiveEntry with Type tar.TypeSymlink will be skipped.
func createZip(t *testing.T, zipPath string, entries []ArchiveEntry) {
	t.Helper()

	file, err := os.Create(zipPath)
	require.NoError(t, err)
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	for _, entry := range entries {
		switch entry.Type {
		case tar.TypeDir:
			// Directories in zip must end with /
			name := entry.Name
			if name[len(name)-1] != '/' {
				name += "/"
			}
			_, err = zipWriter.Create(name)
			require.NoError(t, err)

		case tar.TypeReg:
			writer, err := zipWriter.Create(entry.Name)
			require.NoError(t, err)
			_, err = writer.Write([]byte(entry.Content))
			require.NoError(t, err)

		case tar.TypeSymlink:
			// ZIP does not support symlinks, skip
			t.Logf("Skipping symlink entry %s (ZIP does not support symlinks)", entry.Name)
		}
	}
}

// verifyExtractedFile checks that a file exists and has the expected content
func verifyExtractedFile(t *testing.T, path, expectedContent string) {
	t.Helper()
	require.FileExists(t, path)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, expectedContent, string(content))
}

// verifyExtractedSymlink checks that a symlink exists and points to the expected target
func verifyExtractedSymlink(t *testing.T, path, expectedTarget string) {
	t.Helper()
	linkInfo, err := os.Lstat(path)
	require.NoError(t, err, "Symlink should exist")
	require.True(t, linkInfo.Mode()&os.ModeSymlink != 0, "Should be a symlink")
	target, err := os.Readlink(path)
	require.NoError(t, err)
	require.Equal(t, expectedTarget, target)
}

// setupTestDir creates a temporary directory and returns it along with a cleanup function
func setupTestDir(t *testing.T) (tmpDir string, dstPath string) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "vendir-archive-test")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	dstPath = filepath.Join(tmpDir, "extracted")
	require.NoError(t, os.MkdirAll(dstPath, 0755))

	return tmpDir, dstPath
}

func TestArchiveUnpackTgz(t *testing.T) {
	t.Run("Unpack tgz file with dir, file and symlink", func(t *testing.T) {
		tmpDir, dstPath := setupTestDir(t)

		// Create a synthetic tgz file without symlinks
		tgzPath := filepath.Join(tmpDir, "test.tgz")
		createTar(t, tgzPath, []ArchiveEntry{
			{Name: "testdir/", Type: tar.TypeDir},
			{Name: "testdir/file.txt", Type: tar.TypeReg, Content: "hello world"},
			{Name: "testdir/subdir/nested.txt", Type: tar.TypeReg, Content: "nested content"},
			{Name: "testdir/link", Type: tar.TypeSymlink, Linkname: "file.txt"},
		}, TarOptions{Gzip: true})

		unpacked, err := ctlfetch.NewArchive(tgzPath, false, "").Unpack(dstPath)
		require.NoError(t, err, "Unpacking should not return an error")
		require.True(t, unpacked, "Unpacking should return true indicating success")

		// Verify that files were extracted
		verifyExtractedFile(t, filepath.Join(dstPath, "testdir", "file.txt"), "hello world")
		verifyExtractedFile(t, filepath.Join(dstPath, "testdir", "subdir", "nested.txt"), "nested content")

		// Verify the symlink was created
		verifyExtractedSymlink(t, filepath.Join(dstPath, "testdir", "link"), "file.txt")
	})
}

func TestArchiveUnpackTar(t *testing.T) {
	t.Run("Unpack tar file dir, file and symlink", func(t *testing.T) {
		tmpDir, dstPath := setupTestDir(t)

		// Create a synthetic tgz file without symlinks
		tarPath := filepath.Join(tmpDir, "test.tar")
		createTar(t, tarPath, []ArchiveEntry{
			{Name: "testdir/", Type: tar.TypeDir},
			{Name: "testdir/file.txt", Type: tar.TypeReg, Content: "hello world"},
			{Name: "testdir/subdir/nested.txt", Type: tar.TypeReg, Content: "nested content"},
			{Name: "testdir/link", Type: tar.TypeSymlink, Linkname: "file.txt"},
		}, TarOptions{Gzip: false})

		unpacked, err := ctlfetch.NewArchive(tarPath, false, "").Unpack(dstPath)
		require.NoError(t, err, "Unpacking should not return an error")
		require.True(t, unpacked, "Unpacking should return true indicating success")

		// Verify that files were extracted
		verifyExtractedFile(t, filepath.Join(dstPath, "testdir", "file.txt"), "hello world")
		verifyExtractedFile(t, filepath.Join(dstPath, "testdir", "subdir", "nested.txt"), "nested content")

		// Verify the symlink was created
		verifyExtractedSymlink(t, filepath.Join(dstPath, "testdir", "link"), "file.txt")
	})
}

func TestArchiveUnpackZip(t *testing.T) {
	t.Run("Unpack zip file without symlinks succeeds", func(t *testing.T) {
		tmpDir, dstPath := setupTestDir(t)

		// Create a synthetic zip file
		zipPath := filepath.Join(tmpDir, "test.zip")
		createZip(t, zipPath, []ArchiveEntry{
			{Name: "testdir/", Type: tar.TypeDir},
			{Name: "testdir/file.txt", Type: tar.TypeReg, Content: "hello world"},
			{Name: "testdir/subdir/nested.txt", Type: tar.TypeReg, Content: "nested content"},
		})

		unpacked, err := ctlfetch.NewArchive(zipPath, false, "").Unpack(dstPath)
		require.NoError(t, err, "Unpacking should not return an error")
		require.True(t, unpacked, "Unpacking should return true indicating success")

		// Verify that files were extracted
		verifyExtractedFile(t, filepath.Join(dstPath, "testdir", "file.txt"), "hello world")
		verifyExtractedFile(t, filepath.Join(dstPath, "testdir", "subdir", "nested.txt"), "nested content")
	})
}
