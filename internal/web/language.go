package web

import (
	"net/http"
	"strings"
)

const languageCookieName = "neca_lang"

func (s *Server) translatorFor(r *http.Request) *webTranslator {
	if cookie, err := r.Cookie(languageCookieName); err == nil {
		candidate := normalizeWebLanguage(cookie.Value)
		if translator, ok := s.translators[candidate]; ok {
			return translator
		}
	}
	return s.translator
}

func (s *Server) handleLanguage(w http.ResponseWriter, r *http.Request) {
	lang := normalizeWebLanguage(r.URL.Query().Get("lang"))
	http.SetCookie(w, &http.Cookie{
		Name: languageCookieName, Value: lang, Path: "/",
		HttpOnly: false, Secure: s.secureCookies, SameSite: http.SameSiteLaxMode,
		MaxAge: 365 * 24 * 60 * 60,
	})
	next := safeNext(r.URL.Query().Get("next"))
	if next == "" {
		next = "/"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (d pageData) T(key string, args ...any) string {
	if d.Tr == nil {
		return key
	}
	return d.Tr.T(key, args...)
}

func (d pageData) StatusLabel(status string) string {
	if d.Tr == nil {
		return status
	}
	return d.T("web.status." + strings.TrimSpace(status))
}
