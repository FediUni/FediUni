package file

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	log "github.com/golang/glog"
	"github.com/google/uuid"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultDirectoryPermission = 0777 // This value is the default for directory creation, gives rwx to owner, group and others
)

var (
	allowedImageExtensions = map[string]bool{".jpeg": true, ".jpg": true, ".png": true}
)

type Handler struct {
	Images string
	Server http.Handler
}

// NewHandler returns a handler with operations for file storage.
func NewHandler(images string) (*Handler, error) {
	if images == "" {
		return nil, fmt.Errorf("directory for uploaded images is unspecified: got=%q", images)
	}
	if !filepath.IsAbs(images) {
		return nil, fmt.Errorf("directory is not an absolute path: got=%q", images)
	}
	if err := initializeDirectory(images); err != nil {
		return nil, err
	}

	return &Handler{
		Images: images,
		Server: http.FileServer(http.Dir(images)),
	}, nil
}

func (h *Handler) ProfilePictures() string {
	return filepath.Join(h.Images, "profiles")
}

// StoreProfilePicture stores the file and returns the relative path.
// Directories are created as required. Files are named using generated UUIDs.
func (h *Handler) StoreProfilePicture(tempFile multipart.File, filename string) (string, error) {
	extension := filepath.Ext(filename)
	if !allowedImageExtensions[strings.ToLower(extension)] {
		return "", fmt.Errorf("failed to receive an allowed image extension: got=%q", extension)
	}
	name := fmt.Sprintf("%s%s", uuid.New().String(), extension)
	hash := sha256.Sum256([]byte(name))
	log.Infof("Converted Filename=%q to Hash=%q", name, hash)
	encodedHash := base64.StdEncoding.EncodeToString(hash[:])
	fullPath := filepath.Join(h.ProfilePictures(), encodedHash[0:1], encodedHash[0:2])
	if err := initializeDirectory(fullPath); err != nil {
		return "", fmt.Errorf("failed to initialize directory: got err=%v", err)
	}
	fullPath = filepath.Join(fullPath, name)
	file, err := os.Create(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: got err=%v", err)
	}
	if _, err := io.Copy(file, tempFile); err != nil {
		return "", fmt.Errorf("failed to copy file contents: got err=%v", err)
	}
	return filepath.Rel(h.Images, fullPath)
}

func initializeDirectory(filepath string) error {
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		return os.MkdirAll(filepath, defaultDirectoryPermission)
	}
	return nil
}
