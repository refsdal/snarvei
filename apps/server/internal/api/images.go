package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/refsdal/snarvei/server/internal/api/middleware"
	"github.com/refsdal/snarvei/server/internal/api/respond"
	"github.com/refsdal/snarvei/server/internal/auth"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
)

const (
	maxProfileImageBytes = 2 << 20
	profileImagePrefix   = "/images/profile/"
)

var imageExtensions = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}

// mountImageRoutes registers the multipart upload, the delete and the public
// image stream. Binary bodies have no place in the JSON strict server.
func (d Deps) mountImageRoutes(mux *http.ServeMux) {
	session := d.chain(tierSession)
	mux.Handle("POST /api/me/profile-image", session(http.HandlerFunc(d.uploadProfileImage)))
	mux.Handle("DELETE /api/me/profile-image", session(http.HandlerFunc(d.deleteProfileImage)))
	mux.HandleFunc("GET /images/profile/{userId}/{file}", d.serveProfileImage)
}

func storageKey(publicPath string) string { return strings.TrimPrefix(publicPath, "/images/") }

func (d Deps) uploadProfileImage(w http.ResponseWriter, r *http.Request) {
	s := middleware.SessionFromContext(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxProfileImageBytes+64<<10)
	if err := r.ParseMultipartForm(maxProfileImageBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			respond.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Profile image must be 2 MiB or smaller")
			return
		}
		respond.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Expected a multipart upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Profile image file is required")
		return
	}
	defer file.Close()
	if header.Size > maxProfileImageBytes {
		respond.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Profile image must be 2 MiB or smaller")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxProfileImageBytes+1))
	if err != nil || len(data) > maxProfileImageBytes {
		respond.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Profile image must be 2 MiB or smaller")
		return
	}
	ctype := http.DetectContentType(data)
	ext, ok := imageExtensions[ctype]
	if !ok {
		respond.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Unsupported image type")
		return
	}
	key := "profile/" + s.UserID + "/" + auth.NewID() + "." + ext
	if err := d.Storage.Put(r.Context(), key, bytes.NewReader(data), int64(len(data)), ctype); err != nil {
		d.responseErrorHandler(w, r, err)
		return
	}
	publicPath := "/images/" + key
	if err := d.Q.UpdateUserImage(r.Context(), dbgen.UpdateUserImageParams{ID: s.UserID, Image: &publicPath}); err != nil {
		_ = d.Storage.Delete(r.Context(), key)
		d.responseErrorHandler(w, r, err)
		return
	}
	if s.Image != nil && strings.HasPrefix(*s.Image, profileImagePrefix+s.UserID+"/") {
		_ = d.Storage.Delete(r.Context(), storageKey(*s.Image))
	}
	respond.JSON(w, http.StatusOK, map[string]any{"imageUrl": publicPath})
}

func (d Deps) deleteProfileImage(w http.ResponseWriter, r *http.Request) {
	s := middleware.SessionFromContext(r.Context())
	if err := d.Q.UpdateUserImage(r.Context(), dbgen.UpdateUserImageParams{ID: s.UserID, Image: nil}); err != nil {
		d.responseErrorHandler(w, r, err)
		return
	}
	if s.Image != nil && strings.HasPrefix(*s.Image, profileImagePrefix+s.UserID+"/") {
		_ = d.Storage.Delete(r.Context(), storageKey(*s.Image))
	}
	respond.JSON(w, http.StatusOK, map[string]any{"imageUrl": nil})
}

func (d Deps) serveProfileImage(w http.ResponseWriter, r *http.Request) {
	userID, file := r.PathValue("userId"), r.PathValue("file")
	ext := strings.TrimPrefix(path.Ext(file), ".")
	ctype := ""
	for t, e := range imageExtensions {
		if e == ext {
			ctype = t
		}
	}
	if ctype == "" || strings.ContainsAny(userID+file, "/\\") {
		respond.Error(w, http.StatusNotFound, "NOT_FOUND", "Not found")
		return
	}
	rc, found, err := d.Storage.GetStream(r.Context(), "profile/"+userID+"/"+file)
	if err != nil {
		d.responseErrorHandler(w, r, err)
		return
	}
	if !found {
		respond.Error(w, http.StatusNotFound, "NOT_FOUND", "Not found")
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, rc)
}
