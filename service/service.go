package service

import (
	"context"
	cryptoRand "crypto/rand"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"text/template"
	"time"

	"giveaway-tool/config"
	"giveaway-tool/database/sqlc"

	"github.com/gorilla/sessions"
)

//go:embed templates
var templates embed.FS

type Service struct {
	router       *http.ServeMux
	logger       *slog.Logger
	tmpl         *template.Template
	queries      *sqlc.Queries
	sessionStore *sessions.CookieStore
	adminData    *AdminData
}

// generateRandomKey generates a random key for session encryption
func generateRandomKey(length int) ([]byte, error) {
	key := make([]byte, length)
	_, err := cryptoRand.Read(key)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func Start(router *http.ServeMux, logger *slog.Logger, db *sql.DB) {
	// Get session key from environment or generate a new one
	var sessionKey []byte
	sessionKeyStr := os.Getenv("SESSION_KEY")

	if sessionKeyStr != "" {
		// If provided in environment, decode from base64
		var err error
		sessionKey, err = base64.StdEncoding.DecodeString(sessionKeyStr)
		if err != nil {
			logger.LogAttrs(context.Background(), slog.LevelError,
				"Failed to decode SESSION_KEY from base64, generating a new one",
				slog.Any("error", err))
			sessionKey = nil
		}
	}

	// If we don't have a valid key yet, generate one
	if len(sessionKey) < 32 {
		var err error
		sessionKey, err = generateRandomKey(32)
		if err != nil {
			logger.LogAttrs(context.Background(), slog.LevelError,
				"Failed to generate random session key", slog.Any("error", err))
			panic(err)
		}

		// Log the generated key so it can be saved for future use
		encodedKey := base64.StdEncoding.EncodeToString(sessionKey)
		logger.LogAttrs(context.Background(), slog.LevelWarn,
			"Generated new session key. For persistence across restarts, set SESSION_KEY environment variable",
			slog.String("generated_key", encodedKey))
	}

	// Get admin credentials from environment or use defaults
	adminUsername := os.Getenv("ADMIN_USERNAME")
	if adminUsername == "" {
		adminUsername = "admin"
		logger.LogAttrs(context.Background(), slog.LevelWarn,
			"Using default admin username. Set ADMIN_USERNAME environment variable in production.")
	}

	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = "password"
		logger.LogAttrs(context.Background(), slog.LevelWarn,
			"Using default admin password. Set ADMIN_PASSWORD environment variable in production.")
	}

	svc := &Service{
		router:       router,
		logger:       logger,
		queries:      sqlc.New(db),
		sessionStore: sessions.NewCookieStore(sessionKey),
		adminData: &AdminData{
			Username: adminUsername,
			Password: adminPassword,
		},
	}

	// Configure session store
	svc.sessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 1 week
		HttpOnly: true,
	}

	tmpl := template.New("base")

	// Add functions once
	tmpl = tmpl.Funcs(template.FuncMap{
		"toJSON": func(v any) string {
			b, err := json.Marshal(v)
			if err != nil {
				svc.logger.LogAttrs(context.Background(), slog.LevelError, "Failed to marshal to JSON", slog.Any("error", err))
				return ""
			}
			return string(b)
		},
		"now": time.Now,
		"formatFloat": func(f float64) string {
			return fmt.Sprintf("%.2f", f)
		},
	})

	// Parse templates
	tmpl = template.Must(tmpl.ParseFS(templates, "templates/*.htmx"))

	svc.tmpl = tmpl

	// Public routes
	svc.router.HandleFunc("GET /", svc.handleEvents)
	svc.router.HandleFunc("GET /login", svc.handleLoginPage)
	svc.router.HandleFunc("POST /login", svc.handleLogin)
	svc.router.HandleFunc("GET /logout", svc.handleLogout)

	// Admin routes - protected by middleware
	svc.router.HandleFunc("GET /admin", svc.requireAdmin(svc.handleAdminDashboard))
	svc.router.HandleFunc("GET /admin/events/{id}", svc.requireAdmin(svc.handleGetEvent))
	svc.router.HandleFunc("PUT /admin/events/{id}", svc.requireAdmin(svc.handleUpdateEvent))
	svc.router.HandleFunc("POST /admin/events/{id}/current", svc.requireAdmin(svc.handleSetCurrentEvent))
	svc.router.HandleFunc("POST /admin/events/{id}/winners", svc.requireAdmin(svc.handleGetWinners))
	svc.router.HandleFunc("GET /admin/event", svc.requireAdmin(svc.handleCreateEventPage))
	svc.router.HandleFunc("POST /admin/event", svc.requireAdmin(svc.handleCreateEvent))
	svc.router.HandleFunc("DELETE /admin/events/{id}", svc.requireAdmin(svc.handleDeleteEvent))
	svc.router.HandleFunc("DELETE /admin/events/{eventID}/users/{userID}", svc.requireAdmin(svc.handleDeleteEventUser))
	svc.router.HandleFunc("PATCH /admin/events/{eventID}/users/{userID}", svc.requireAdmin(svc.handleUpdateUserCount))
}

// Middleware to check if user is admin
func (s *Service) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := s.sessionStore.Get(r, "session")
		if err != nil {
			s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to get session", slog.Any("error", err))
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Check if user is authenticated as admin
		isAdmin, ok := session.Values["isAdmin"].(bool)
		if !ok || !isAdmin {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		next(w, r)
	}
}

func (s *Service) runTemplate(w http.ResponseWriter, r *http.Request, name string, data any) {
	w.Header().Set("Content-Type", "text/html")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to execute template", slog.Any("error", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Service) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	events, err := s.queries.GetEvents(r.Context())
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to get events", slog.Any("error", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Check if user is admin
	session, err := s.sessionStore.Get(r, "session")
	isAdmin := false
	if err == nil {
		isAdmin, _ = session.Values["isAdmin"].(bool)
	}

	s.runTemplate(w, r, "events", Data{
		Events:         events,
		CurrentEventID: config.GetCurrentEventID(),
		IsAdmin:        isAdmin,
	})
}

func (s *Service) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	s.runTemplate(w, r, "login", nil)
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to parse form", slog.Any("error", err))
		fmt.Fprintf(w, errHTML, "Invalid form submission")
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	// Check credentials
	if username == s.adminData.Username && password == s.adminData.Password {
		// Set user as authenticated in session
		session, _ := s.sessionStore.Get(r, "session")

		session.Values["isAdmin"] = true
		if err := session.Save(r, w); err != nil {
			s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to save session", slog.Any("error", err))
			fmt.Fprintf(w, errHTML, "Failed to save session. Please try again.")
			return
		}

		// If this is an HTMX request, respond with a redirect instruction
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Redirect", "/admin")
			return
		}

		// Otherwise do a standard redirect
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	fmt.Fprintf(w, errHTML, "Invalid username or password")
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	session, err := s.sessionStore.Get(r, "session")
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to get session", slog.Any("error", err))
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Revoke authentication
	session.Values["isAdmin"] = false
	session.Options.MaxAge = -1 // Delete the cookie

	if err := session.Save(r, w); err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to save session", slog.Any("error", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Service) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	events, err := s.queries.GetEvents(r.Context())
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to get events", slog.Any("error", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	s.runTemplate(w, r, "admin_events", Data{
		Events:  events,
		IsAdmin: true,
	})
}

func (s *Service) handleCreateEventPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Render the create event page
	s.runTemplate(w, r, "admin_create_event", nil)
}

func (s *Service) handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	s.logger.LogAttrs(r.Context(), slog.LevelInfo, "Handling create event")
	// Parse form data for new event
	if err := r.ParseForm(); err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to parse form", slog.Any("error", err))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Extract event details from form
	name := r.FormValue("name")
	description := r.FormValue("description")

	if name == "" {
		fmt.Fprintf(w, errHTML, "Event name is required")
		return
	}

	// Get the date from the form
	formDate := r.FormValue("date")
	s.logger.LogAttrs(r.Context(), slog.LevelInfo, "Received date", slog.String("date", formDate))

	// Parse the date - corrected order of arguments
	// For datetime-local inputs, the format is typically "2006-01-02T15:04"
	date, err := time.Parse("2006-01-02T15:04", formDate)
	if err != nil {
		// Try alternative format if the first one fails
		date, err = time.Parse("2006-01-02 15:04:05", formDate)
		if err != nil {
			s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to parse date",
				slog.String("input", formDate),
				slog.Any("error", err))
			fmt.Fprintf(w, errHTML, "Invalid date format. Please use YYYY-MM-DDTHH:MM format.")
			return
		}
	}

	s.logger.LogAttrs(r.Context(), slog.LevelInfo, "Parsed date successfully",
		slog.String("original", formDate),
		slog.Time("parsed", date))

	// Create event in database
	_, err = s.queries.CreateEvent(r.Context(), &sqlc.CreateEventParams{
		Name:        name,
		Description: sql.NullString{String: description, Valid: description != ""},
		Date:        date,
	})

	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to create event", slog.Any("error", err))
		fmt.Fprintf(w, errHTML, "Failed to create event: "+err.Error())
		return
	}

	// If this is an HTMX request, respond with a redirect instruction
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/admin")
		return
	}

	// Otherwise do a standard redirect
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Service) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract event ID from URL
	eventID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Invalid event ID", slog.Any("error", err))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Delete event from database
	event, err := s.queries.GetEventByID(r.Context(), int64(eventID))
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to get event", slog.Any("error", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	users, err := s.queries.GetUsersByEventID(r.Context(), int64(eventID))
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to get users", slog.Any("error", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	type eventData struct {
		Event *sqlc.Events  `json:"event"`
		Users []*sqlc.Users `json:"users"`
	}

	s.runTemplate(w, r, "admin_event", eventData{
		Event: event,
		Users: users,
	})
}

func (s *Service) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract event ID from URL
	eventID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Invalid event ID", slog.Any("error", err))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Delete event from database
	err = s.queries.DeleteEvent(r.Context(), int64(eventID))
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to delete event", slog.Any("error", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/admin")
		return
	}
}

func (s *Service) handleUpdateEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract event ID from URL
	eventID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Invalid event ID", slog.Any("error", err))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Parse form data for updated event details
	if err := r.ParseForm(); err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to parse form", slog.Any("error", err))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")

	if name == "" {
		fmt.Fprintf(w, errHTML, "Event name is required")
		return
	}

	updateReq := &sqlc.UpdateEventParams{
		ID:          int64(eventID),
		Name:        name,
		Description: sql.NullString{String: description, Valid: description != ""},
	}

	formDate := r.FormValue("date")
	s.logger.LogAttrs(r.Context(), slog.LevelInfo, "Received date", slog.String("date", formDate))

	if formDate == "" {
		event, err := s.queries.GetEventByID(r.Context(), int64(eventID))
		if err != nil {
			s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to get event", slog.Any("error", err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		updateReq.Date = event.Date
	} else {
		date, err := time.Parse("2006-01-02T15:04", formDate)
		if err != nil {
			date, err = time.Parse("2006-01-02 15:04:05", formDate)
			if err != nil {
				s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to parse date",
					slog.String("input", formDate),
					slog.Any("error", err))
				fmt.Fprintf(w, errHTML, "Invalid date format. Please use YYYY-MM-DDTHH:MM format.")
				return
			}
		}
		updateReq.Date = date
	}

	_, err = s.queries.UpdateEvent(r.Context(), updateReq)

	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to update event", slog.Any("error", err))
		fmt.Fprintf(w, errHTML, "Failed to update event: "+err.Error())
		return
	}

	// If this is an HTMX request, respond with a redirect instruction
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/admin")
		return
	}
}

func (s *Service) handleSetCurrentEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	eventID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Invalid event ID", slog.Any("error", err))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	config.SetCurrentEventID(int64(eventID))

	fmt.Fprintf(w, successHTML, "Current event set successfully")
}

func (s *Service) handleDeleteEventUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	eventID, err := strconv.Atoi(r.PathValue("eventID"))
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Invalid event ID", slog.Any("error", err))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	userID, err := strconv.Atoi(r.PathValue("userID"))
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Invalid user ID", slog.Any("error", err))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	err = s.queries.DeleteUsersByIdAndEventId(r.Context(), &sqlc.DeleteUsersByIdAndEventIdParams{
		ID:      int64(userID),
		EventID: int64(eventID),
	})
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to delete user", slog.Any("error", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func (s *Service) handleGetWinners(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	countStr := r.FormValue("count")
	if countStr == "" {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Count is required")
		fmt.Fprintf(w, errHTML, "Count is required")
		return
	}

	count, err := strconv.Atoi(countStr)
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Invalid count", slog.Any("error", err))
		fmt.Fprintf(w, errHTML, "Invalid count")
		return
	}

	eventID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Invalid event ID", slog.Any("error", err))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	winners := make([]*sqlc.Users, 0)

	//get all event users
	users, err := s.queries.GetUsersByEventID(r.Context(), int64(eventID))
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to get users", slog.Any("error", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	n := len(users)
	for i := range n {
		n := users[i].N
		if n > 1 {
			for range n - 1 {
				users = append(users, users[i])
			}
		}
	}

	// Determine how many winners to select (minimum of count and available users)
	winnersCount := count
	if winnersCount > len(users) {
		winnersCount = len(users)
	}

	// Select random winners
	for i := 0; i < winnersCount; i++ {
		if len(users) == 0 {
			break
		}

		// Pick a random index within the valid range
		index := rand.IntN(len(users))

		winners = append(winners, users[index])

		// Remove the selected user from the pool
		users = append(users[:index], users[index+1:]...)
	}

	type winnersData struct {
		Users []*sqlc.Users `json:"event"`
	}
	s.runTemplate(w, r, "winners", winnersData{
		Users: winners,
	})
}

func (s *Service) handleUpdateUserCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	eventID, err := strconv.Atoi(r.PathValue("eventID"))
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Invalid event ID", slog.Any("error", err))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	userID, err := strconv.Atoi(r.PathValue("userID"))
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Invalid user ID", slog.Any("error", err))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	err = r.ParseForm()
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to parse form", slog.Any("error", err))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	n, err := strconv.Atoi(r.FormValue("n"))
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Invalid count", slog.Any("error", err))
		fmt.Fprintf(w, errHTML, "Invalid count")
		return
	}

	err = s.queries.UpdateUserN(r.Context(), &sqlc.UpdateUserNParams{
		ID:      int64(userID),
		EventID: int64(eventID),
		N:       int32(n),
	})
	if err != nil {
		s.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to delete user", slog.Any("error", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "%d", n)
}
