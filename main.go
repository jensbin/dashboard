package main

import (
	"context"
	_ "embed" // embed file into the binary
	"encoding/json"
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

//go:embed static/index.html.hbs
var indexTemplate string

//go:embed static/css/apps.css
var appsCSS string

//go:embed static/js/apps.js
var appsJS string

type App struct {
	Name       string `yaml:"name"`
	URL        string `yaml:"url"`
	DisplayURL string `yaml:"display_url"`
	Icon       string `yaml:"icon"`
}

type AppCategory struct {
	Name string
	Apps []App
}

type Bookmark struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type BookmarkGroup struct {
	Name      string
	Bookmarks []Bookmark
}

type Search struct {
	Name   string `yaml:"name"`
	URL    string `yaml:"url"`
	Prefix string `yaml:"prefix"`
}

type Config struct {
	AppCategories  []AppCategory   `yaml:"app_categories"`
	BookmarkGroups []BookmarkGroup `yaml:"bookmark_groups"`
	Searches       []Search        `yaml:"search"`
}

func main() {
	listenAddress := flag.String("listen", "127.0.0.1:8080", "IP and port to listen on [127.0.0.1:8080]")
	staticDir := flag.String("static", "", "Path to the static directory (optional)")
	configFile := flag.String("config", "config.yaml", "Path to the config.yaml file [config.yaml]")
	flag.Parse()

	// Read config.yaml
	configData, err := os.ReadFile(*configFile)
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	} else {
		log.Printf("Configfile: %s", *configFile)
	}

	var configMux sync.RWMutex
	var config Config
	err = yaml.Unmarshal(configData, &config)
	if err != nil {
		log.Fatal(err)
	}

	var tmpl *template.Template
	var templateFuncs = template.FuncMap{
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"toJSON": func(v interface{}) template.JS {
			data, err := json.Marshal(v)
			if err != nil {
				log.Printf("Error marshaling data for template: %v", err)
				return ""
			}
			return template.JS(string(data))
		},
	}

	reloadTemplate := func() {
		var err error
		if *staticDir != "" {
			log.Printf("Serving from %s", *staticDir)
			templatePath := filepath.Join(*staticDir, "index.html.hbs")
			tmpl, err = template.New(filepath.Base(templatePath)).Funcs(templateFuncs).ParseFiles(templatePath)
			if err != nil {
				log.Fatalf("Error parsing template file: %v", err)
			}
		} else {
			log.Printf("Serving from embedded template")
			tmpl, err = template.New("template").Funcs(templateFuncs).Parse(indexTemplate)
			if err != nil {
				log.Fatalf("Error parsing embedded template: %v", err)
			}
		}
	}

	reloadTemplate()

	// Start config file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	done := make(chan bool) // Channel to signal watcher shutdown

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					log.Println("Config file changed:", event.Name, event.Op.String())
					time.Sleep(100 * time.Millisecond) // Debounce
					reloadConfig(*configFile, &config, &configMux)
					reloadTemplate()
				}
				if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
					log.Printf("Config file remove/rename: %s. Re-adding watcher", event.Name)
					addWatch(*configFile, watcher) // Add or re-add the watch
					reloadConfig(*configFile, &config, &configMux)
					reloadTemplate()
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("Error watching config file:", err)
			case <-done: // Shutdown signal received
				return
			}
		}
	}()

	addWatch(*configFile, watcher)

	// Setup Gin router
	router := gin.Default()

	if *staticDir != "" { // Serve from static directory if specified
		router.Static("/static", *staticDir)
	} else { // Serve embedded static files
		router.GET("/static/css/apps.css", func(c *gin.Context) {
			c.Data(http.StatusOK, "text/css; charset=utf-8", []byte(appsCSS))
		})
		router.GET("/static/js/apps.js", func(c *gin.Context) {
			c.Data(http.StatusOK, "text/javascript; charset=utf-8", []byte(appsJS))
		})
	}

	router.GET("/", func(c *gin.Context) {
		configMux.RLock() // Lock for read access to the config
		tmplData := config
		configMux.RUnlock() // Release the read lock

		err = tmpl.Execute(c.Writer, tmplData)
		if err != nil {
			log.Printf("Error executing template: %v", err)
			c.String(http.StatusInternalServerError, "Error rendering template")
		}
	})

	// Start server
	log.Printf("Listening on http://%s", *listenAddress)
	srv := &http.Server{
		Addr:    *listenAddress,
		Handler: router,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Graceful shutdown on interrupt (Ctrl+C)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	<-quit // Wait for interrupt signal

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
	close(done)                 // Signal watcher goroutine to exit
	time.Sleep(1 * time.Second) // Give time for the watcher to close
	log.Println("Server exiting")
}

func addWatch(configFile string, watcher *fsnotify.Watcher) {
	err := watcher.Add(configFile)
	if err != nil {
		log.Printf("Failed to add watch on config file %s: %v, retrying in 5 seconds", configFile, err)
		time.Sleep(5 * time.Second)
		addWatch(configFile, watcher)
	} else {
		log.Printf("Watching %s for changes", configFile)
	}
}

func reloadConfig(configFile string, config *Config, configMux *sync.RWMutex) {
	configData, err := os.ReadFile(configFile)
	if err != nil {
		log.Printf("Error reading config file after change: %v", err)
		return
	}

	configMux.Lock() // Lock before unmarshalling to prevent race conditions
	defer configMux.Unlock()
	err = yaml.Unmarshal(configData, config) // Unmarshal directly into the config variable
	if err != nil {
		log.Printf("Error unmarshalling config file after change: %v", err)
		return
	}
	log.Printf("Config reloaded")
}
