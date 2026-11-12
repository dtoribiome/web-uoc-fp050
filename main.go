// Producto 1 - DevOps y Cloud Computing (UOC)
// Aplicación web en Go: muestra "Soy alumno de la UOC" y una imagen,
// y gestiona distintos tipos de peticiones HTTP (GET, POST, PUT, DELETE...).
// Producto 2: añadida la ruta /pod para comprobar el balanceo en Kubernetes.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// go:embed mete la plantilla y las imágenes DENTRO del ejecutable.
//
//go:embed templates static
var ficheros embed.FS

// Plantilla HTML de la página principal.
// Plantilla cargada desde templates/index.html (incrustada en el binario).
var pagina = template.Must(template.ParseFS(ficheros, "templates/index.html"))

// Almacén en memoria para la API de mensajes.
type Mensaje struct {
	ID    int    `json:"id"`
	Texto string `json:"texto"`
}

type almacen struct {
	mu        sync.Mutex
	datos     map[int]Mensaje
	siguiente int
}

var store = &almacen{datos: map[int]Mensaje{1: {1, "Soy alumno de la UOC"}}, siguiente: 2}

// responderJSON escribe una respuesta JSON con el código de estado indicado.
func responderJSON(w http.ResponseWriter, codigo int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codigo)
	json.NewEncoder(w).Encode(v)
}

// GET / -> página HTML con el mensaje y la imagen.
func inicio(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		responderJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método no permitido"})
		return
	}
	host, _ := os.Hostname()
	pagina.Execute(w, map[string]string{
		"Mensaje": "Soy alumno de la UOC",
		"Alumno":  getenv("ALUMNO", "Daniel Toribio Méndez"),
		"Host":    host,
		"Hora":    time.Now().Format("02/01/2006 15:04:05"),
	})
}

// GET /pod -> indica qué pod (réplica) ha atendido la petición.
// Sirve para demostrar que el Service de Kubernetes reparte el tráfico.
func pod(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Hola desde el pod %s\n", host)
}

// /api/mensajes -> API REST que captura distintos tipos de peticiones.
func mensajes(w http.ResponseWriter, r *http.Request) {
	store.mu.Lock()
	defer store.mu.Unlock()

	switch r.Method {
	case http.MethodGet: // listar
		lista := make([]Mensaje, 0, len(store.datos))
		for i := 1; i < store.siguiente; i++ {
			if m, ok := store.datos[i]; ok {
				lista = append(lista, m)
			}
		}
		responderJSON(w, http.StatusOK, lista)

	case http.MethodPost: // crear
		var m Mensaje
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil || strings.TrimSpace(m.Texto) == "" {
			responderJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido: se espera {\"texto\":\"...\"}"})
			return
		}
		m.ID = store.siguiente
		store.siguiente++
		store.datos[m.ID] = m
		responderJSON(w, http.StatusCreated, m)

	case http.MethodPut: // modificar
		var m Mensaje
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			responderJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
			return
		}
		if _, ok := store.datos[m.ID]; !ok {
			responderJSON(w, http.StatusNotFound, map[string]string{"error": "mensaje no encontrado"})
			return
		}
		store.datos[m.ID] = m
		responderJSON(w, http.StatusOK, m)

	case http.MethodDelete: // borrar (?id=N)
		id, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil {
			responderJSON(w, http.StatusBadRequest, map[string]string{"error": "parámetro id obligatorio"})
			return
		}
		if _, ok := store.datos[id]; !ok {
			responderJSON(w, http.StatusNotFound, map[string]string{"error": "mensaje no encontrado"})
			return
		}
		delete(store.datos, id)
		w.WriteHeader(http.StatusNoContent)

	default:
		w.Header().Set("Allow", "GET, POST, PUT, DELETE")
		responderJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método no permitido"})
	}
}

// GET /health -> comprobación de salud (usada por el HEALTHCHECK de Docker
// y por las sondas readiness/liveness de Kubernetes).
func salud(w http.ResponseWriter, r *http.Request) {
	responderJSON(w, http.StatusOK, map[string]string{"estado": "ok"})
}

// logger: middleware que registra cada petición (método, ruta, duración).
func logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s (%v)", r.RemoteAddr, r.Method, r.URL.Path, time.Since(inicio))
	})
}

func getenv(clave, defecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return defecto
}

func main() {
	// Modo healthcheck: el binario se comprueba a sí mismo (la imagen final no tiene curl).
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		resp, err := http.Get("http://127.0.0.1:" + getenv("PORT", "8080") + "/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	sub, _ := fs.Sub(ficheros, "static")
	mux := http.NewServeMux()
	mux.HandleFunc("/", inicio)
	mux.HandleFunc("/pod", pod)
	mux.HandleFunc("/api/mensajes", mensajes)
	mux.HandleFunc("/health", salud)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))

	srv := &http.Server{
		Addr:         ":" + getenv("PORT", "8080"),
		Handler:      logger(mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Apagado ordenado al recibir SIGINT/SIGTERM (docker stop / kubectl delete pod).
	go func() {
		log.Printf("Servidor escuchando en %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	parar := make(chan os.Signal, 1)
	signal.Notify(parar, syscall.SIGINT, syscall.SIGTERM)
	<-parar
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	log.Println("Servidor detenido correctamente")
}
