package main

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
)

// Server represents a simple-upload server.
type Server struct {
	DocumentRoot string
	// MaxUploadSize limits the size of the uploaded content, specified with "byte".
	MaxUploadSize int64
	SecureToken   string
}

// NewServer creates a new simple-upload server.
func NewServer(documentRoot string, maxUploadSize int64, token string) Server {
	return Server{
		DocumentRoot:  documentRoot,
		MaxUploadSize: maxUploadSize,
		SecureToken:   token,
	}
}

func (s Server) handleGet(w http.ResponseWriter, r *http.Request) {
	re := regexp.MustCompile(`^/files/([^/]+)$`)
	if !re.MatchString(r.URL.Path) {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, fmt.Errorf("\"%s\" is not found", r.URL.Path))
		return
	}
	http.StripPrefix("/files/", http.FileServer(http.Dir(s.DocumentRoot))).ServeHTTP(w, r)
}

func (s Server) handlePost(w http.ResponseWriter, r *http.Request) {
	srcFile, info, err := r.FormFile("file")
	if err != nil {
		// logger.WithError(err).Error("failed to acquire the uploaded content")
		fmt.Println("failed to acquire the uploaded content, err:", err)
		w.WriteHeader(http.StatusInternalServerError)
		writeError(w, err)
		return
	}
	defer srcFile.Close()
	// logger.Debug(info)
	fmt.Println(info)
	size, err := getSize(srcFile)
	if err != nil {
		// logger.WithError(err).Error("failed to get the size of the uploaded content")
		fmt.Println("failed to get the size of the uploaded content, err:", err)
		w.WriteHeader(http.StatusInternalServerError)
		writeError(w, err)
		return
	}
	if size > s.MaxUploadSize {
		// logger.WithField("size", size).Info("file size exceeded")
		fmt.Println("file size exceeded, size:", size)
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		writeError(w, errors.New("uploaded file size exceeds the limit"))
		return
	}

	body, err := ioutil.ReadAll(srcFile)
	if err != nil {
		// logger.WithError(err).Error("failed to read the uploaded content")
		fmt.Println("failed to read the uploaded content, err:", err)
		w.WriteHeader(http.StatusInternalServerError)
		writeError(w, err)
		return
	}
	filename := info.Filename
	if filename == "" {
		filename = fmt.Sprintf("%x", sha1.Sum(body))
	}

	dstPath := path.Join(s.DocumentRoot, filename)
	dstFile, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		// logger.WithError(err).WithField("path", dstPath).Error("failed to open the file")
		fmt.Println("failed to open the file, err:", err, "path:", dstPath)
		w.WriteHeader(http.StatusInternalServerError)
		writeError(w, err)
		return
	}
	defer dstFile.Close()
	if written, err := dstFile.Write(body); err != nil {
		// logger.WithError(err).WithField("path", dstPath).Error("failed to write the content")
		fmt.Println("failed to write the content, err:", err, "path:", dstPath)
		w.WriteHeader(http.StatusInternalServerError)
		writeError(w, err)
		return
	} else if int64(written) != size {
		// logger.WithFields(logrus.Fields{
		// 	"size":    size,
		// 	"written": written,
		// }).Error("uploaded file size and written size differ")
		fmt.Println("uploaded file size and written size differ, size:", size, "written:", written)
		w.WriteHeader(http.StatusInternalServerError)
		writeError(w, fmt.Errorf("the size of uploaded content is %d, but %d bytes written", size, written))
	}
	uploadedURL := strings.TrimPrefix(dstPath, s.DocumentRoot)
	if !strings.HasPrefix(uploadedURL, "/") {
		uploadedURL = "/" + uploadedURL
	}
	uploadedURL = "/files" + uploadedURL
	// logger.WithFields(logrus.Fields{
	// 	"path": dstPath,
	// 	"url":  uploadedURL,
	// 	"size": size,
	// }).Info("file uploaded by POST")
	fmt.Println("file uploaded by POST, path:", dstPath, "url:", uploadedURL, "size:", size)
	w.WriteHeader(http.StatusOK)
	writeSuccess(w, uploadedURL)
}

func (s Server) handlePut(w http.ResponseWriter, r *http.Request) {
	re := regexp.MustCompile(`^/files/([^/]+)$`)
	matches := re.FindStringSubmatch(r.URL.Path)
	if matches == nil {
		// logger.WithField("path", r.URL.Path).Info("invalid path")
		fmt.Println("invalid path, path:", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		writeError(w, fmt.Errorf("\"%s\" is not found", r.URL.Path))
		return
	}
	targetPath := path.Join(s.DocumentRoot, matches[1])
	file, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		// logger.WithError(err).WithField("path", targetPath).Error("failed to open the file")
		fmt.Println("failed to open the file, err:", err, "path:", targetPath)
		w.WriteHeader(http.StatusInternalServerError)
		writeError(w, err)
		return
	}
	defer file.Close()
	defer r.Body.Close()
	srcFile, info, err := r.FormFile("file")
	if err != nil {
		// logger.WithError(err).WithField("path", targetPath).Error("failed to acquire the uploaded content")
		fmt.Println("failed to acquire the uploaded content, err:", err, "path:", targetPath)
		w.WriteHeader(http.StatusInternalServerError)
		writeError(w, err)
		return
	}
	defer srcFile.Close()
	// dump headers for the file
	// logger.Debug(info.Header)
	fmt.Println(info.Header)

	size, err := getSize(srcFile)
	if err != nil {
		// logger.WithError(err).WithField("path", targetPath).Error("failed to get the size of the uploaded content")
		fmt.Println("failed to get the size of the uploaded content, err:", err, "path:", targetPath)
		w.WriteHeader(http.StatusInternalServerError)
		writeError(w, err)
		return
	}
	if size > s.MaxUploadSize {
		// logger.WithFields(logrus.Fields{
		// 	"path": targetPath,
		// 	"size": size,
		// }).Info("file size exceeded")
		fmt.Println("file size exceeded, path:", targetPath, "size:", size)
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		writeError(w, errors.New("uploaded file size exceeds the limit"))
		return
	}

	n, err := io.Copy(file, srcFile)
	if err != nil {
		// logger.WithError(err).WithField("path", targetPath).Error("failed to write body to the file")
		fmt.Println("failed to write body to the file, err:", err, "path:", targetPath)
		w.WriteHeader(http.StatusInternalServerError)
		writeError(w, err)
		return
	}
	// logger.WithFields(logrus.Fields{
	// 	"path": r.URL.Path,
	// 	"size": n,
	// }).Info("file uploaded by PUT")
	fmt.Println("file uploaded by PUT, path:", targetPath, "size:", n)
	w.WriteHeader(http.StatusOK)
	writeSuccess(w, r.URL.Path)
}

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// first, try to get the token from the query strings
	token := r.URL.Query().Get("token")
	// if token is not found, check the form parameter.
	if token == "" {
		token = r.Form.Get("token")
	}
	if token != s.SecureToken {
		w.WriteHeader(http.StatusUnauthorized)
		writeError(w, fmt.Errorf("authentication required"))
		return
	}

	switch r.Method {
	case http.MethodGet, http.MethodHead:
		s.handleGet(w, r)
	case http.MethodPost:
		s.handlePost(w, r)
	case http.MethodPut:
		s.handlePut(w, r)
	default:
		w.Header().Add("Allow", "GET,HEAD,POST,PUT")
		w.WriteHeader(http.StatusMethodNotAllowed)
		writeError(w, fmt.Errorf("method \"%s\" is not allowed", r.Method))
	}
}
