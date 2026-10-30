package main

import (
	measurement "github.com/EkeMinusYou/isucon-template/tools/measurement/app"
	"net/http"
)

// Bootstrap calls this after identifying the contest's session. No cookie name
// or authentication scheme is built into the generic application skeleton.
func setMeasurementIdentity(r *http.Request, sessionIdentity string) {
	measurement.SetSessionIdentity(r, sessionIdentity)
}

// connectionPolicy preserves long-lived HTTP/1 connections up to the source
// recipe's finite request cap. HTTP/2 lifecycle remains owned by net/http.
func connectionPolicy(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 1 && measurement.ConnectionRequestCount(r) >= 1000000 {
			r.Close = true
			w.Header().Set("Connection", "close")
		}
		next.ServeHTTP(w, r)
	})
}
