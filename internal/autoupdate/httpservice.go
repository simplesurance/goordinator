package autoupdate

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	_ "embed" // used to embed html templates and static docs

	"go.uber.org/zap"
)

// templFS contains the web pages.
//
//go:embed pages/templates/*
var templFS embed.FS

//go:embed pages/static/*
var staticFS embed.FS

var templFuncs = template.FuncMap{
	"add": func(a, b int) int {
		return a + b
	},
}

type HTTPService struct {
	autoupdater *Autoupdater
	templates   *template.Template
	logger      *zap.Logger
}

func NewHTTPService(autoupdater *Autoupdater) *HTTPService {
	return &HTTPService{
		autoupdater: autoupdater,
		templates: template.Must(
			template.New("").
				Funcs(templFuncs).
				ParseFS(templFS, "pages/templates/*"),
		),
		logger: autoupdater.logger.Named("http_service"),
	}
}

func (h *HTTPService) RegisterHandlers(mux *http.ServeMux, endpoint string) {
	mux.HandleFunc(endpoint, h.HandlerListFunc)

	staticPath := endpoint + "static" + "/"

	mux.Handle(
		staticPath,
		http.StripPrefix(
			staticPath,
			h.HandlerStaticFiles(),
		),
	)
}

func (h *HTTPService) HandlerStaticFiles() http.Handler {
	subFs, err := fs.Sub(staticFS, "pages/static")
	if err != nil {
		h.logger.Panic("creating sub fs for static http files failed", zap.Error(err))
	}

	return http.FileServer(http.FS(subFs))
}

func (h *HTTPService) HandlerListFunc(respWr http.ResponseWriter, _ *http.Request) {
	data := h.autoupdater.httpListData()

	err := h.templates.ExecuteTemplate(respWr, "list.html.tmpl", data)
	if err != nil {
		h.logger.Info("applying template and sending back result failed", zap.Error(err))
		http.Error(respWr, err.Error(), http.StatusInternalServerError)
		return
	}
}
