package api

import (
	"net/http"

	"github.com/refsdal/snarvei/server/internal/api/respond"
)

// mountTestHooks exposes the recorded mailbox for the Playwright suite.
// Only mounted when Deps.TestHooks is true (E2E_TEST_HOOKS=1, loopback
// APP_URL only, enforced by config).
func (d Deps) mountTestHooks(mux *http.ServeMux) {
	if d.Mail == nil {
		panic("api: TestHooks requires a recording mailer")
	}
	mux.HandleFunc("GET /api/_test/mail", func(w http.ResponseWriter, r *http.Request) {
		msgs := d.Mail.Messages()
		out := make([]map[string]string, 0, len(msgs))
		for i := len(msgs) - 1; i >= 0; i-- {
			out = append(out, map[string]string{"to": msgs[i].To, "subject": msgs[i].Subject, "text": msgs[i].Text})
		}
		respond.JSON(w, http.StatusOK, map[string]any{"messages": out})
	})
	mux.HandleFunc("DELETE /api/_test/mail", func(w http.ResponseWriter, r *http.Request) {
		d.Mail.Reset()
		w.WriteHeader(http.StatusNoContent)
	})
}
