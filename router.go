package grest

import (
	"net/http"
	"strings"
	"encoding/json"
	"net/url"
)

type (
	Next func(err error)

	// implement this interface to handle http request
	HTTPHandler interface {
		HTTPHandle(res http.ResponseWriter, req *http.Request, next Next)
	}

	HTTPHandleFunc func(res http.ResponseWriter, req *http.Request, next Next)

	Router struct {
		stack        []*layer
		routerPrefix string // prefix path, trimmed off it when route
	}
)

// implement HTTPHandle, and call itself
func (h HTTPHandleFunc) HTTPHandle(res http.ResponseWriter, req *http.Request, next Next) {
	h(res, req, next)
}

// Create one new Router
func NewRouter() *Router {
	router := &Router{
		stack: make([]*layer, 0),
	}

	return router
}

// set handlers for `path`, default is `/`. you can use it as filters
func (this *Router) Use(path string, handlers ...HTTPHandler) *Router {
	if path == "" {
		path = "/" // default to root path
	}

	for _, handler := range handlers {
		// prepare router prefix path
		if r, ok := handler.(*Router); ok == true {
			r.routerPrefix = this.routerPrefix + path
		}

		l := newLayer(path, handler, false)
		l.route = nil
		this.stack = append(this.stack, l)
	}

	return this
}

// set handler funcitons for `path`, default is `/`. you can use it as filters
func (this *Router) UseFunc(path string, handlers ...HTTPHandleFunc) *Router {

	for _, handler := range handlers {
		this.Use(path, handler)
	}

	return this
}

// create a sub-route
func (this *Router) Route(path string) *Route {
	route := newRoute(path)
	l := newLayer(path, route, true) // route.HTTPHandler

	l.route = route

	this.stack = append(this.stack, l)

	return route
}

// set handlers for all types requests
func (this *Router)All(path string, handlers ...HTTPHandler) *Router{
	this.Route(path).All(handlers...)

	return this
}

// set handlers functions for all types requests
func (this *Router)AllFunc(path string, handlers ...HTTPHandleFunc) *Router{
	this.Route(path).AllFunc(handlers...)

	return this
}

func (this *Router) addHandler(method string, path string, handlers ...HTTPHandler) *Router {
	route := this.Route(path)

	switch method {
	case "GET":
		route.GET(handlers...);
	case "POST":
		route.POST(handlers...);
	case "PUT":
		route.PUT(handlers...);
	case "DELETE":
		route.DELETE(handlers...);
	case "HEAD":
		route.HEAD(handlers...);
	// ignore others
	}
	return this
}

// set handlers for `GET` request
func (this *Router) GET(path string, handlers ...HTTPHandler) *Router {
	return this.addHandler("GET", path, handlers...)
}

// set handlers for `POST` request
func (this *Router) POST(path string, handlers ...HTTPHandler) *Router {
	return this.addHandler("POST", path, handlers...)
}

// set handlers for `PUT` request
func (this *Router) PUT(path string, handlers ...HTTPHandler) *Router {
	return this.addHandler("PUT", path, handlers...)
}

// set handlers for `DELETE` request
func (this *Router) DELETE(path string, handlers ...HTTPHandler) *Router {
	return this.addHandler("DELETE", path, handlers...)
}

// set handlers for `HEAD` request
func (this *Router) HEAD(path string, handlers ...HTTPHandler) *Router {
	return this.addHandler("HEAD", path, handlers...)
}

// set handlers functions for `GET` request
func (this *Router) GETFunc(path string, handlers ...HTTPHandleFunc) *Router {
	for _, handler := range handlers {
		this.GET(path, handler); // pass them one by one, so that HTTPHandleFunc can be treat as HTTPHandler
	}
	return this
}

// set handlers functions for `POST` request
func (this *Router) POSTFunc(path string, handlers ...HTTPHandleFunc) *Router {
	for _, handler := range handlers {
		this.POST(path, handler);
	}
	return this
}

// set handlers functions for `PUT` request
func (this *Router) PUTFunc(path string, handlers ...HTTPHandleFunc) *Router {
	for _, handler := range handlers {
		this.PUT(path, handler);
	}
	return this
}

// set handlers functions for `DELETE` request
func (this *Router) DELETEFunc(path string, handlers ...HTTPHandleFunc) *Router {
	for _, handler := range handlers {
		this.DELETE(path, handler);
	}
	return this
}

// set handlers functions for `HEAD` request
func (this *Router) HEADFunc(path string, handlers ...HTTPHandleFunc) *Router {
	for _, handler := range handlers {
		this.HEAD(path, handler);
	}
	return this
}


func (this *Router) matchLayer(l *layer, path string) (url.Values, bool) {
	urlParams, match := l.match(path)
	return urlParams, match
}

func (this *Router) route(req *http.Request, res http.ResponseWriter, done Next) {
	var next func(err error)
	var idx = 0

	var allowOptionsMethods = make([]string, 0, 5)
	if req.Method == "OPTIONS" {
		// reply OPTIONS request automatically
		old := done
		done = func(err error) {
			if err != nil || len(allowOptionsMethods) == 0 {
				old(err)
			} else {
				res.Header().Add("Allow", strings.Join(allowOptionsMethods, ","))
				data, err := json.Marshal(allowOptionsMethods)
				if err != nil {
					old(err)
					return
				}
				res.Write(data)
			}

		}
	}

	next = func(err error) {
		if idx >= len(this.stack) {
			done(err)
			return
		}
		// get trimmed path for current router
		path := strings.TrimPrefix(req.URL.Path, this.routerPrefix)
		if path == "" {
			done(err)
			return
		}

		// find next matching layer
		var match = false
		var l *layer
		var route *Route
		var urlParams url.Values

		for ; match != true && idx < len(this.stack); {
			l = this.stack[idx]
			idx ++
			urlParams, match = this.matchLayer(l, path);
			route = l.route

			if match != true || route == nil {
				continue
			}
			method := req.Method
			hasMethod := route.handlesMethod(method)

			if !hasMethod && method == "OPTIONS" {
				for _, method := range route.optionsMethods() {
					allowOptionsMethods = append(allowOptionsMethods, method)
				}
			}

			if !hasMethod && method != "HEAD" {
				match = false
			}
		}

		if match != true || err != nil {
			done(err)
			return
		}
		l.registerParamsAsQuery(req, urlParams)

		l.handleRequest(res, req, next)
	}

	next(nil)
}

// implement HTTPHandler interface, make it can be as a handler
func (this *Router) HTTPHandle(res http.ResponseWriter, req *http.Request, next Next) {
	this.route(req, res, next)
}

// implement http.Handler interface
func (this Router) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	this.route(req, rw, func(err error) {
		if err != nil {
			http.Error(rw, "Something wrong", http.StatusInternalServerError)
			return
		}
		http.NotFound(rw, req)
	})
}



