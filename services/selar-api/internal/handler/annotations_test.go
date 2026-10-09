package handler

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnnotationPatchRejectsIdentityTampering(t *testing.T) {
	for _, body := range []string{`{"user_id":"other","comment":"x"}`, `{"document_id":"other"}`, `{"anchor":null}`, `{"bbox":[]}`, `{"color":"red"}`, `{}`, `{"comment":"x"} {}`} {
		t.Run(body, func(t *testing.T) {
			h := &Handler{}
			w := httptest.NewRecorder()
			r := httptest.NewRequest("PATCH", "/annotations/test", strings.NewReader(body))
			h.UpdateAnnotation(w, r)
			if w.Code != 400 {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAnnotationCreateRejectsMalformedAnchors(t *testing.T) {
	for _, extra := range []string{`"page":0`, `"page":1,"anchor":{"version":2}`, `"page":1,"anchor":{"version":1,"source_hash":"s","exact":"text","start":0,"end":999}`, `"page":1,"anchor":{"version":1,"source_hash":"s","exact":"text","start":0,"end":4},"bbox":[{"x":2,"y":0,"w":1,"h":1}]`} {
		t.Run(extra, func(t *testing.T) {
			h := &Handler{}
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/annotations", strings.NewReader(`{"document_id":"00000000-0000-0000-0000-000000000001","type":"highlight","color":"yellow","bbox":[],`+extra+`}`))
			h.CreateAnnotation(w, r)
			if w.Code != 400 {
				t.Fatalf("status %d", w.Code)
			}
		})
	}
}
