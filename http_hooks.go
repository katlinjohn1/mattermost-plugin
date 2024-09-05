package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// ServeHTTP allows the plugin to implement the http.Handler interface. Requests destined for the
// /plugins/{id} path will be routed to the plugin.
//
// The Mattermost-User-Id header will be present if (and only if) the request is by an
// authenticated user.
//
// This demo implementation sends back whether or not the plugin hooks are currently enabled. It
// is used by the web app to recover from a network reconnection and synchronize the state of the
// plugin's hooks.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}

func (p *Plugin) initializeAPI() {
	router := mux.NewRouter()

	dialogRouter := router.PathPrefix("/dialog").Subrouter()
	dialogRouter.Use(p.withDelay)
	dialogRouter.HandleFunc("/submit-dialog", p.handleDialog)
	dialogRouter.HandleFunc("/error", p.handleDialogWithError)

	p.router = router
}

func (p *Plugin) withDelay(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delay := p.getConfiguration().IntegrationRequestDelay
		if delay > 0 {
			time.Sleep(time.Duration(delay) * time.Second)
		}

		next.ServeHTTP(w, r)
	})
}

func (p *Plugin) handleDialog(w http.ResponseWriter, r *http.Request) {

	configuration := p.getConfiguration()

	teams, err2 := p.API.GetTeams()
	if err2 != nil {
		errors.Wrap(err2, "failed to query teams OnActivate")
	}

	for _, team := range teams {
		_, ok := configuration.demoChannelIDs[team.Id]
		if !ok {
			p.API.LogWarn("No demo channel id for team", "team", team.Id)
			continue
		}

		msg := fmt.Sprintf("OnActivate: %s", manifest.Id)
		if err := p.postPluginMessage(team.Id, msg); err != nil {
			errors.Wrap(err, "failed to post OnActivate message")
		}
	}

	var request model.SubmitDialogRequest

	if request.Cancelled {
		return
	}
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		p.API.LogError("Failed to decode SubmitDialogRequest", "err", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	
	defer r.Body.Close()

	user, appErr := p.API.GetUser(request.UserId)
	if appErr != nil {
		p.API.LogError("Failed to get user for dialog", "err", appErr.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	msg := "@%v submitted a ticket\n```json\n%v\n```"

	requestJSON, jsonErr := json.MarshalIndent(request, "", "  ")
	if jsonErr != nil {
		p.API.LogError("Failed to marshal json for interactive action", "err", jsonErr.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	rootPost, appErr := p.API.CreatePost(&model.Post{
		UserId:    p.botID,
		ChannelId: request.ChannelId,
		Message:   fmt.Sprintf(msg, user.Username, string(requestJSON)),
	})
	if appErr != nil {
		p.API.LogError("Failed to post handleDialog1 message", "err", appErr.Error())
		return
	}

	msg = "cc: @%v"
	if !request.Cancelled {

		if _, appErr = p.API.CreatePost(&model.Post{
			UserId:    p.botID,
			ChannelId: request.ChannelId,
			RootId:    rootPost.Id,
			//set user to incident responders group
			Message:   fmt.Sprintf(msg, user.Username),
			// Type:      "custom_demo_plugin",
			Type: model.PostActionTypeSelect,
			Props:     request.Submission,
		}); appErr != nil {
			p.API.LogError("Failed to post handleDialog1 message", "err", appErr.Error())
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	
}

func (p *Plugin) handleDialogWithError(w http.ResponseWriter, r *http.Request) {
	// Always return an error
	response := &model.SubmitDialogResponse{
		Error: "some error",
	}
	p.writeJSON(w, response)
}

func (p *Plugin) writeJSON(w http.ResponseWriter, response any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		p.API.LogError("Failed to write JSON response", "err", err.Error())
	}
}