// Package httpapi routes the order-request API and maps errors to status codes.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/ploiid10/beautyscale-order-requests/api/internal/order"
)

type workspaceKey struct{}

type handler struct {
	svc *order.Service
}

// NewHandler returns the HTTP handler.
func NewHandler(svc *order.Service) http.Handler {
	h := &handler{svc: svc}
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "Beauty Scale test API for demo purposes\n")
	})
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Route("/api", func(r chi.Router) {
		r.Use(requireWorkspace)
		r.Get("/products", h.products)
		r.Post("/order-requests", h.create)
		r.Get("/order-requests", h.list)
		r.Get("/order-requests/{id}", h.get)
		r.Post("/order-requests/{id}/accept", h.accept)
		r.Post("/order-requests/{id}/reject", h.reject)
	})
	return r
}

func requireWorkspace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimSpace(r.Header.Get("X-Workspace-ID"))
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			writeError(w, http.StatusBadRequest, "X-Workspace-ID must be a UUID")
			return
		}
		ctx := context.WithValue(r.Context(), workspaceKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func workspaceFrom(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(workspaceKey{}).(uuid.UUID)
	return id
}

func (h *handler) products(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Products(r.Context(), workspaceFrom(r.Context()))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, limit, offset, total, err := h.svc.List(
		r.Context(),
		workspaceFrom(r.Context()),
		q.Get("status"),
		q.Get("limit"),
		q.Get("offset"),
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"limit":  limit,
		"offset": offset,
		"total":  total,
	})
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.Get(r.Context(), workspaceFrom(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var in order.CreateInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	created, err := h.svc.Create(r.Context(), workspaceFrom(r.Context()), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *handler) accept(w http.ResponseWriter, r *http.Request) {
	draft, err := h.svc.Accept(r.Context(), workspaceFrom(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

func (h *handler) reject(w http.ResponseWriter, r *http.Request) {
	decision, err := h.svc.Reject(r.Context(), workspaceFrom(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, decision)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeServiceError(w http.ResponseWriter, err error) {
	var oe *order.Error
	if errors.As(err, &oe) {
		switch {
		case errors.Is(err, order.ErrInvalid):
			writeError(w, http.StatusBadRequest, oe.Error())
		case errors.Is(err, order.ErrNotFound):
			writeError(w, http.StatusNotFound, oe.Error())
		case errors.Is(err, order.ErrConflict):
			writeError(w, http.StatusConflict, oe.Error())
		default:
			log.Printf("request failed: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	log.Printf("request failed: %v", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write json: %v", err)
	}
}
