package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestGetAndUpdateMe(t *testing.T) {
	a := testrig.App(t)
	id := a.SignUp("Kari", "kari@example.com")
	cookie := a.SignIn("kari@example.com")

	resp := a.Do(http.MethodGet, "/api/me", nil, cookie)
	if resp.Code != 200 {
		t.Fatalf("me: %d %s", resp.Code, resp.Body)
	}
	user := resp.JSON["user"].(map[string]any)
	session := resp.JSON["session"].(map[string]any)
	if user["id"] != id || user["name"] != "Kari" || user["email"] != "kari@example.com" || user["image"] != nil || user["twoFactorEnabled"] != false {
		t.Fatalf("user: %v", user)
	}
	if session["id"] == "" || session["activeOrganizationId"] != nil || strings.Contains(string(resp.Body), "token") {
		t.Fatalf("session: %v", session)
	}
	if resp := a.Do(http.MethodGet, "/api/me", nil, ""); resp.Code != 401 || resp.JSON["code"] != "UNAUTHENTICATED" {
		t.Fatalf("anonymous: %d %s", resp.Code, resp.Body)
	}

	resp = a.Do(http.MethodPatch, "/api/me", map[string]string{"name": "  Kari N.  "}, cookie)
	if resp.Code != 200 || resp.JSON["user"].(map[string]any)["name"] != "Kari N." {
		t.Fatalf("patch: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPatch, "/api/me", map[string]string{"name": ""}, cookie); resp.Code != 400 || resp.JSON["code"] != "VALIDATION_FAILED" {
		t.Fatalf("empty name: %d %s", resp.Code, resp.Body)
	}
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func upload(t *testing.T, a *testrig.AppRig, cookie string, field string, data []byte) testrig.Response {
	t.Helper()
	var body bytes.Buffer
	mp := multipart.NewWriter(&body)
	part, _ := mp.CreateFormFile(field, "avatar.bin")
	_, _ = part.Write(data)
	_ = mp.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/me/profile-image", &body)
	req.Header.Set("Content-Type", mp.FormDataContentType())
	req.Header.Set("Origin", "http://127.0.0.1")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	rec := a.DoRaw(req)
	resp := testrig.Response{Code: rec.Code, Header: rec.Header(), Body: rec.Body.Bytes()}
	if trimmed := bytes.TrimSpace(resp.Body); len(trimmed) > 0 && trimmed[0] == '{' {
		_ = json.Unmarshal(trimmed, &resp.JSON)
	}
	return resp
}

func TestProfileImageLifecycle(t *testing.T) {
	a := testrig.App(t)
	a.SignUp("Kari", "kari@example.com")
	cookie := a.SignIn("kari@example.com")

	if resp := upload(t, a, "", "file", pngBytes(t)); resp.Code != 401 {
		t.Fatalf("anonymous upload: %d", resp.Code)
	}
	if resp := upload(t, a, cookie, "file", []byte("not an image at all")); resp.Code != 400 {
		t.Fatalf("non-image: %d %s", resp.Code, resp.Body)
	}
	if resp := upload(t, a, cookie, "wrong", pngBytes(t)); resp.Code != 400 {
		t.Fatalf("wrong field: %d %s", resp.Code, resp.Body)
	}
	if resp := upload(t, a, cookie, "file", bytes.Repeat([]byte{0}, 2<<20+1)); resp.Code != 400 && resp.Code != 413 {
		t.Fatalf("oversized: %d %s", resp.Code, resp.Body)
	}

	resp := upload(t, a, cookie, "file", pngBytes(t))
	if resp.Code != 200 {
		t.Fatalf("upload: %d %s", resp.Code, resp.Body)
	}
	imageURL, _ := resp.JSON["imageUrl"].(string)
	if !strings.HasPrefix(imageURL, "/images/profile/") || !strings.HasSuffix(imageURL, ".png") {
		t.Fatalf("imageUrl: %q", imageURL)
	}
	me := a.Do(http.MethodGet, "/api/me", nil, cookie)
	if me.JSON["user"].(map[string]any)["image"] != imageURL {
		t.Fatalf("me.image: %v", me.JSON)
	}

	get := a.Do(http.MethodGet, imageURL, nil, "")
	if get.Code != 200 || get.Header.Get("Content-Type") != "image/png" || get.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" || !bytes.Equal(get.Body, pngBytes(t)) {
		t.Fatalf("serve: %d %s %s", get.Code, get.Header.Get("Content-Type"), get.Header.Get("Cache-Control"))
	}
	if miss := a.Do(http.MethodGet, "/images/profile/nobody/none.png", nil, ""); miss.Code != 404 {
		t.Fatalf("missing image: %d", miss.Code)
	}

	second := upload(t, a, cookie, "file", pngBytes(t))
	if a.Do(http.MethodGet, imageURL, nil, "").Code != 404 {
		t.Fatal("previous image must be deleted after replacement")
	}
	secondURL := second.JSON["imageUrl"].(string)

	if del := a.Do(http.MethodDelete, "/api/me/profile-image", nil, cookie); del.Code != 200 || del.JSON["imageUrl"] != nil {
		t.Fatalf("delete: %d %s", del.Code, del.Body)
	}
	if a.Do(http.MethodGet, secondURL, nil, "").Code != 404 {
		t.Fatal("deleted image still served")
	}
	if me := a.Do(http.MethodGet, "/api/me", nil, cookie); me.JSON["user"].(map[string]any)["image"] != nil {
		t.Fatal("me.image not cleared")
	}
}

func TestSessions(t *testing.T) {
	a := testrig.App(t)
	a.SignUp("Kari", "kari@example.com")
	c1 := a.SignIn("kari@example.com")
	c2 := a.SignIn("kari@example.com")
	c3 := a.SignIn("kari@example.com")

	list := a.Do(http.MethodGet, "/api/me/sessions", nil, c1)
	if list.Code != 200 || len(list.Array) != 3 || strings.Contains(string(list.Body), "token") {
		t.Fatalf("list: %d %s", list.Code, list.Body)
	}
	current, otherID := 0, ""
	for _, s := range list.Array {
		if s["current"] == true {
			current++
		} else if otherID == "" {
			otherID = s["id"].(string)
		}
	}
	if current != 1 || otherID == "" {
		t.Fatalf("current flag: %v", list.Array)
	}
	if resp := a.Do(http.MethodDelete, "/api/me/sessions/"+otherID, nil, c1); resp.Code != 204 {
		t.Fatalf("revoke one: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodDelete, "/api/me/sessions/"+otherID, nil, c1); resp.Code != 404 {
		t.Fatalf("revoke twice: %d", resp.Code)
	}
	if resp := a.Do(http.MethodDelete, "/api/me/sessions", nil, c1); resp.Code != 204 {
		t.Fatalf("revoke others: %d", resp.Code)
	}
	if a.Do(http.MethodGet, "/api/me", nil, c1).Code != 200 {
		t.Fatal("current session must survive")
	}
	if a.Do(http.MethodGet, "/api/me", nil, c2).Code != 401 || a.Do(http.MethodGet, "/api/me", nil, c3).Code != 401 {
		t.Fatal("other sessions must be gone")
	}
	other := a.SignUp("Other", "other@example.com")
	_ = other
	oc := a.SignIn("other@example.com")
	mine := a.Do(http.MethodGet, "/api/me/sessions", nil, c1).Array[0]["id"].(string)
	if resp := a.Do(http.MethodDelete, "/api/me/sessions/"+mine, nil, oc); resp.Code != 404 {
		t.Fatalf("revoking someone else's session must look like 404: %d", resp.Code)
	}
}

func TestEmailChange(t *testing.T) {
	a := testrig.App(t)
	a.SignUp("Kari", "kari@example.com")
	a.SignUp("Taken", "taken@example.com")
	cookie := a.SignIn("kari@example.com")

	if resp := a.Do(http.MethodPost, "/api/me/email", map[string]string{"newEmail": "new@example.com", "password": "wrong"}, cookie); resp.Code != 401 || resp.JSON["code"] != "INVALID_PASSWORD" {
		t.Fatalf("wrong password: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/me/email", map[string]string{"newEmail": "taken@example.com", "password": testrig.Password}, cookie); resp.Code != 409 || resp.JSON["code"] != "EMAIL_TAKEN" {
		t.Fatalf("taken: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/me/email", map[string]string{"newEmail": "new@example.com", "password": testrig.Password}, cookie); resp.Code != 202 {
		t.Fatalf("request: %d %s", resp.Code, resp.Body)
	}
	msg, ok := a.Mail.Last("new@example.com")
	if !ok || !strings.Contains(msg.Text, "/app/settings?emailToken=") {
		t.Fatalf("mail: %+v", msg)
	}
	tok := strings.Fields(msg.Text[strings.Index(msg.Text, "emailToken=")+len("emailToken="):])[0]

	if resp := a.Do(http.MethodPost, "/api/me/email/confirm", map[string]string{"token": "nope"}, cookie); resp.Code != 400 {
		t.Fatalf("bad token: %d %s", resp.Code, resp.Body)
	}
	resp := a.Do(http.MethodPost, "/api/me/email/confirm", map[string]string{"token": tok}, cookie)
	if resp.Code != 200 || resp.JSON["user"].(map[string]any)["email"] != "new@example.com" {
		t.Fatalf("confirm: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/me/email/confirm", map[string]string{"token": tok}, cookie); resp.Code != 400 {
		t.Fatalf("token reuse: %d", resp.Code)
	}
	if a.SignIn("new@example.com") == "" {
		t.Fatal("sign in with the new address")
	}
}

func TestDeleteMe(t *testing.T) {
	a := testrig.App(t)
	orgID, ownerCookie := a.NewOrg("Acme", "acme", "owner@example.com")
	_ = orgID
	if resp := a.Do(http.MethodDelete, "/api/me", map[string]string{"password": "wrong"}, ownerCookie); resp.Code != 401 {
		t.Fatalf("wrong password: %d", resp.Code)
	}
	if resp := a.Do(http.MethodDelete, "/api/me", map[string]string{"password": testrig.Password}, ownerCookie); resp.Code != 409 || resp.JSON["code"] != "LAST_OWNER" {
		t.Fatalf("sole owner: %d %s", resp.Code, resp.Body)
	}
	a.SignUp("Loner", "loner@example.com")
	loner := a.SignIn("loner@example.com")
	if resp := a.Do(http.MethodDelete, "/api/me", map[string]string{"password": testrig.Password}, loner); resp.Code != 204 {
		t.Fatalf("delete: %d %s", resp.Code, resp.Body)
	}
	if a.Do(http.MethodGet, "/api/me", nil, loner).Code != 401 {
		t.Fatal("deleted user still signed in")
	}
	var n int
	if err := a.Rig.Pool.QueryRow(context.Background(), `SELECT count(*) FROM users WHERE email = 'loner@example.com'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("user row remains: %v %d", err, n)
	}
}
