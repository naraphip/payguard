package main

import "net/http"

func (a app) registerTenantSimulation(mux *http.ServeMux, auth bookingAuth) {
	const path = "/api/v1/booking/network/runs"
	mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
		if actor := auth.requireBookingActor(w, r, bookingRoleMerchant, bookingRoleAnalyst); actor.MerchantID != "" {
			a.createSimulation(w, r, actor.MerchantID)
		}
	})
	mux.HandleFunc("GET "+path+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		actor := auth.requireBookingActor(w, r, bookingRoleMerchant, bookingRoleAnalyst, bookingRoleViewer)
		if actor.MerchantID != "" {
			if saved, ok := a.find(w, r, actor.MerchantID); ok {
				respond(w, http.StatusOK, saved)
			}
		}
	})
	mux.HandleFunc("GET "+path+"/{id}/report", func(w http.ResponseWriter, r *http.Request) {
		if actor := auth.requireBookingActor(w, r, bookingRoleMerchant, bookingRoleAnalyst); actor.MerchantID != "" {
			a.readSimulationReport(w, r, actor.MerchantID)
		}
	})
}
