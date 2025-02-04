package main

import (
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"encoding/json"
	_ "embed" // embed file into the binary

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
	Searches       []Search       `yaml:"search"`
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

	var config Config
	err = yaml.Unmarshal(configData, &config)
	if err != nil {
		log.Fatal(err)
	}

	var tmpl *template.Template
    if *staticDir != "" {
	    log.Printf("Serving from %s", *staticDir)
        templatePath := filepath.Join(*staticDir, "index.html.hbs")
        tmpl, err = template.New(filepath.Base(templatePath)).Funcs(template.FuncMap{
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
        }).ParseFiles(templatePath)
        if err != nil {
            log.Fatalf("Error parsing template file: %v", err)
        }
    } else {
	    log.Printf("Serving from embedded template")
        tmpl, err = template.New("template").Funcs(template.FuncMap{
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
        }).Parse(indexTemplate)
        if err != nil {
            log.Fatalf("Error parsing embedded template: %v", err)
        }
    }

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
        err = tmpl.Execute(c.Writer, config)
        if err != nil {
            log.Printf("Error executing template: %v", err)
            c.String(http.StatusInternalServerError, "Error rendering template")
        }
    })

    // Start server
    log.Printf("Listening on http://%s", *listenAddress)
    if err := router.Run(*listenAddress); err != nil {
        log.Fatal(err)
    }
}
