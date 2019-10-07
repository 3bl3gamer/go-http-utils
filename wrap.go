package httputils

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/ansel1/merry"
	"github.com/julienschmidt/httprouter"
)

type ctxKey string
type HndFormat int
type HandlerExt func(http.ResponseWriter, *http.Request, httprouter.Params) error
type HandlerExtJSON func(http.ResponseWriter, *http.Request, httprouter.Params) (interface{}, error)
type HandlerExtTmpl func(http.ResponseWriter, *http.Request, httprouter.Params) (TemplateCtx, error)
type Middleware func(HandlerExt) HandlerExt
type TemplateCtx map[string]interface{}

type ServerEvent struct {
	Name    string      `json:"name"`
	Details interface{} `json:"details"`
}

type MainCtx struct {
	Format HndFormat
	Sess   struct {
		ID     string
		UserID int64
	}
	ServerEvents    []ServerEvent
	TemplateHandler *TemplateHandler
}

type JsonOk struct {
	Ok     bool          `json:"ok"`
	Result interface{}   `json:"result"`
	Events []ServerEvent `json:"events,omitempty"`
}

type JsonError struct {
	Ok          bool   `json:"ok"`
	Code        int64  `json:"code"`
	Error       string `json:"error"`
	Description string `json:"description"`
}

const (
	Html HndFormat = iota
	Json
)

const CtxKeyMain = ctxKey("main")

var jsonHandlerType = reflect.TypeOf(HandlerExtJSON(nil))
var tmplHandlerType = reflect.TypeOf(HandlerExtTmpl(nil))
var extHandlerType = reflect.TypeOf(HandlerExt(nil))

// func IsIdempotent(method string) bool {
// 	return method == "GET" || method ==  "HEAD" || method == "OPTIONS" || method == "TRACE"
// }

type Wrapper struct {
	ShowErrorDetails bool
	SessionStore     SessionStore
	ExtraChainItem   Middleware
	TemplateHandler  *TemplateHandler
	HandleHtml500    HandlerExt
	LogError         func(error, *http.Request)
}

func (w *Wrapper) WrapChain(chain ...interface{}) httprouter.Handle {
	mainHnd := chain[len(chain)-1]
	mainHndVal := reflect.ValueOf(mainHnd)

	var format = Html
	var handler HandlerExt
	if mainHndVal.Type().ConvertibleTo(jsonHandlerType) {
		format = Json
		handler = AsJSON(mainHndVal.Convert(jsonHandlerType).Interface().(HandlerExtJSON))
	} else if mainHndVal.Type().ConvertibleTo(tmplHandlerType) {
		handler = AsTemplate(w.TemplateHandler, mainHndVal.Convert(tmplHandlerType).Interface().(HandlerExtTmpl))
	} else if mainHndVal.Type().ConvertibleTo(extHandlerType) {
		handler = mainHndVal.Convert(extHandlerType).Interface().(HandlerExt)
	} else {
		panic(fmt.Sprintf("wrong handler: %#v", mainHnd))
	}

	for i := len(chain) - 2; i >= 0; i-- {
		handler = chain[i].(Middleware)(handler)
	}
	if w.SessionStore != nil {
		handler = WrapSession(w.SessionStore, handler)
	}
	if w.ExtraChainItem != nil {
		handler = w.ExtraChainItem(handler)
	}
	return w.wrap(format, handler)
}

func (w *Wrapper) wrap(format HndFormat, handle HandlerExt) httprouter.Handle {
	handleError := func(err error, wr http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		w.LogError(err, r)
		if format == Json {
			if w.ShowErrorDetails {
				json.NewEncoder(wr).Encode(JsonError{false, 500, "SERVER_ERROR", merry.Details(err)})
			} else {
				json.NewEncoder(wr).Encode(JsonError{false, 500, "SERVER_ERROR", ""})
			}
		} else {
			if w.ShowErrorDetails {
				wr.Write([]byte("<pre>" + merry.Details(err)))
			} else {
				if err := w.HandleHtml500(wr, r, ps); err != nil {
					w.LogError(err, r)
				}
			}
		}
	}
	return func(wr http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		mainCtx := &MainCtx{Format: format, TemplateHandler: w.TemplateHandler}
		ctx := r.Context()
		ctx = context.WithValue(ctx, CtxKeyMain, mainCtx)
		r = r.WithContext(ctx)
		if err := handle(wr, r, ps); err != nil {
			handleError(err, wr, r, ps)
			return
		}
	}
}

func WithoutParams(handle httprouter.Handle) http.HandlerFunc {
	return func(wr http.ResponseWriter, r *http.Request) {
		handle(wr, r, httprouter.Params(nil))
	}
}

func AsJSON(handle HandlerExtJSON) HandlerExt {
	return func(wr http.ResponseWriter, r *http.Request, ps httprouter.Params) error {
		ctx := r.Context().Value(CtxKeyMain).(*MainCtx)
		wr.Header().Set("Content-Type", "application/json")
		res, err := handle(wr, r, ps)
		if err != nil {
			return merry.Wrap(err)
		}

		switch t := res.(type) {
		case JsonOk:
			t.Ok = true
			t.Events = ctx.ServerEvents
			wr.WriteHeader(http.StatusOK)
		case JsonError:
			t.Ok = false
			wr.WriteHeader(int(t.Code))
		default:
			res = JsonOk{true, res, ctx.ServerEvents}
		}
		return merry.Wrap(json.NewEncoder(wr).Encode(res))
	}
}

type TemplateHandler struct {
	CacheParsed bool
	BasePath    string
	FuncMap     template.FuncMap
	ParamsFunc  func(*http.Request, *MainCtx, TemplateCtx) error
	LogBuild    func(string)
	cache       map[string]*template.Template
}

func (h *TemplateHandler) ParseTemplate(path string) (*template.Template, error) {
	if h.CacheParsed {
		if tmpl, ok := h.cache[path]; ok {
			return tmpl, nil
		}
	}

	h.LogBuild(path)
	tmpl := template.New(path)
	tmpl = tmpl.Funcs(h.FuncMap)

	tmplGlob, err := tmpl.ParseGlob(h.BasePath + "/_*.html")
	if err == nil {
		tmpl = tmplGlob
	} else if !strings.HasPrefix(err.Error(), "template: pattern matches no files") {
		return nil, merry.Wrap(err)
	}

	tmpl, err = tmpl.ParseFiles(h.BasePath + "/" + path)
	if err != nil {
		return nil, merry.Wrap(err)
	}

	if h.CacheParsed {
		if h.cache == nil {
			h.cache = make(map[string]*template.Template)
		}
		h.cache[path] = tmpl
	}
	return tmpl, nil
}

func (h *TemplateHandler) RenderTemplate(wr io.Writer, r *http.Request, params TemplateCtx) error {
	ctx := r.Context().Value(CtxKeyMain).(*MainCtx)
	fpath, ok := params["FPath"]
	if !ok {
		return merry.New("template path missing")
	}
	tmpl, err := h.ParseTemplate(fpath.(string))
	if err != nil {
		return merry.Wrap(err)
	}
	block := "base"
	if blk, ok := params["Block"]; ok {
		block = blk.(string)
	}
	params["Block"] = block
	if h.ParamsFunc != nil {
		if err := h.ParamsFunc(r, ctx, params); err != nil {
			return merry.Wrap(err)
		}
	}
	return merry.Wrap(tmpl.ExecuteTemplate(wr, block, params))
}

func AsTemplate(handler *TemplateHandler, handle HandlerExtTmpl) HandlerExt {
	return func(wr http.ResponseWriter, r *http.Request, ps httprouter.Params) error {
		params, err := handle(wr, r, ps)
		if err != nil {
			return merry.Wrap(err)
		}
		if params == nil {
			return nil //обработчик уже чем-то ответил, генерить шаблон не нужно
		}
		return handler.RenderTemplate(wr, r, params)
	}
}
