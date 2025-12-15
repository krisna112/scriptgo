package web

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/krisna112/scriptxray/go_panel/pkg/core"
)

// We might embed templates if we move them, but for now we read from disk
// or we can use a variable.
var TemplatesDir = "../web_panel/templates"
var StaticDir = "../web_panel/static"

type Server struct {
	Port   int
	Router *http.ServeMux
}

func NewServer(port int) *Server {
	return &Server{
		Port:   port,
		Router: http.NewServeMux(),
	}
}

func (s *Server) Start() error {
	s.routes()
	addr := fmt.Sprintf(":%d", s.Port)
	log.Printf("Starting Web Server on %s", addr)
	return http.ListenAndServe(addr, s.Router)
}

func (s *Server) routes() {
	// Static files
	fs := http.FileServer(http.Dir(StaticDir))
	s.Router.Handle("GET /static/", http.StripPrefix("/static/", fs))

	// Pages
	s.Router.HandleFunc("GET /", AuthMiddleware(DashboardHandler))
	s.Router.HandleFunc("GET /login", LoginHandler)
	s.Router.HandleFunc("POST /login", LoginPostHandler)
	s.Router.HandleFunc("GET /logout", LogoutHandler)

	// User Management
	s.Router.HandleFunc("GET /add", AuthMiddleware(AddUserFormHandler))
	s.Router.HandleFunc("POST /add", AuthMiddleware(AddUserPostHandler))
	s.Router.HandleFunc("GET /delete/{username}", AuthMiddleware(DeleteUserHandler))

	// Settings
	s.Router.HandleFunc("GET /settings", AuthMiddleware(SettingsHandler))
	// Add more routes as needed
}

// Middleware
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err != nil || !IsValidSession(cookie.Value) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

// Simple Session Store (In-Memory for now)
var validSessions = make(map[string]time.Time)

func IsValidSession(token string) bool {
	exp, ok := validSessions[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(validSessions, token)
		return false
	}
	return true
}

func CreateSession() string {
	token := core.GenerateRandomID(16) // We need to move utils or duplicate
	validSessions[token] = time.Now().Add(24 * time.Hour)
	return token
}

// Helper to render templates
func Render(w http.ResponseWriter, tmplName string, data interface{}) {
	// Parse all templates effectively allowing layouts if needed
	// For efficiency in prod, ParseGlob should be done once at startup.
	// But for dev/migration, parsing on request is safer.
	pk := fmt.Sprintf("%s/*.html", TemplatesDir)
	tmpl, err := template.ParseGlob(pk)
	if err != nil {
		http.Error(w, "Template Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tmpl.ExecuteTemplate(w, tmplName, data); err != nil {
		http.Error(w, "Render Error: "+err.Error(), http.StatusInternalServerError)
	}
}
