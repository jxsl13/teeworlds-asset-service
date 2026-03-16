package server

import (
	"net/http"

	"github.com/jxsl13/asset-service/http/api"
)

// htmxHeaders collects HTMX response headers to be written before the
// inner response body is sent. Use the builder methods to populate them.
type htmxHeaders struct {
	pushURL    string
	retarget   string
	reswap     string
	reselect   string
	trigger    string
	replaceURL string
	redirect   string
	refresh    bool
}

func (h htmxHeaders) apply(w http.ResponseWriter) {
	if h.pushURL != "" {
		w.Header().Set("HX-Push-Url", h.pushURL)
	}
	if h.retarget != "" {
		w.Header().Set("HX-Retarget", h.retarget)
	}
	if h.reswap != "" {
		w.Header().Set("HX-Reswap", h.reswap)
	}
	if h.reselect != "" {
		w.Header().Set("HX-Reselect", h.reselect)
	}
	if h.trigger != "" {
		w.Header().Set("HX-Trigger", h.trigger)
	}
	if h.replaceURL != "" {
		w.Header().Set("HX-Replace-Url", h.replaceURL)
	}
	if h.redirect != "" {
		w.Header().Set("HX-Redirect", h.redirect)
	}
	if h.refresh {
		w.Header().Set("HX-Refresh", "true")
	}
}

// ── RenderUI response wrapper ─────────────────────────────────────────────────

type renderUIHtmxResponse struct {
	inner api.RenderUI200TexthtmlResponse
	hx    htmxHeaders
}

func (r renderUIHtmxResponse) VisitRenderUIResponse(w http.ResponseWriter) error {
	r.hx.apply(w)
	return r.inner.VisitRenderUIResponse(w)
}

// ── RenderItemList response wrapper ───────────────────────────────────────────

type renderItemListHtmxResponse struct {
	inner api.RenderItemList200TexthtmlResponse
	hx    htmxHeaders
}

func (r renderItemListHtmxResponse) VisitRenderItemListResponse(w http.ResponseWriter) error {
	r.hx.apply(w)
	return r.inner.VisitRenderItemListResponse(w)
}
