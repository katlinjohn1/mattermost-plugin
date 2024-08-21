package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pkg/errors"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
	

	root "github.com/mattermost/mattermost-plugin-demo"
)

var (
	manifest model.Manifest = root.Manifest
)

// Helper method for the demo plugin. Posts a message to the "demo" channel
// for the team specified. If the teamID specified is empty, the method
// will post the message to the "demo" channel for each team.
func (p *Plugin) postPluginMessage(teamID, msg string) *model.AppError {
	configuration := p.getConfiguration()

	if configuration.disabled {
		return nil
	}

	if configuration.EnableMentionUser {
		msg = fmt.Sprintf("tag @%s | %s", configuration.MentionUser, msg)
	}
	msg = fmt.Sprintf("%s%s%s", configuration.TextStyle, msg, configuration.TextStyle)

	if teamID != "" {
		_, err := p.API.CreatePost(&model.Post{
			UserId:    p.botID,
			ChannelId: configuration.demoChannelIDs[teamID],
			Message:   msg,
		})
		return err
	}

	for _, channelID := range configuration.demoChannelIDs {
		_, err := p.API.CreatePost(&model.Post{
			UserId:    p.botID,
			ChannelId: channelID,
			Message:   msg,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// configuration captures the plugin's external configuration as exposed in the Mattermost server
// configuration, as well as values computed from the configuration. Any public fields will be
// deserialized from the Mattermost server configuration in OnConfigurationChange.
//
// As plugins are inherently concurrent (hooks being called asynchronously), and the plugin
// configuration can change at any time, access to the configuration must be synchronized. The
// strategy used in this plugin is to guard a pointer to the configuration, and clone the entire
// struct whenever it changes. You may replace this with whatever strategy you choose.
type configuration struct {
	// The user to use as part of the demo plugin, created automatically if it does not exist.
	Username string

	// The channel to use as part of the demo plugin, created for each team automatically if it does not exist.
	ChannelName string

	// LastName is the last name of the demo user.
	LastName string

	// TextStyle controls the text style of the messages posted by the demo user.
	TextStyle string

	// RandomSecret is a generated key that, when mentioned in a message by a user, will trigger the demo user to post the 'SecretMessage'.
	RandomSecret string

	// SecretMessage is the message posted to the demo channel when the 'RandomSecret' is pasted somewhere in the team.
	SecretMessage string

	// EnableMentionUser controls whether the 'MentionUser' is prepended to all demo messages or not.
	EnableMentionUser bool

	// MentionUser is the user that is prepended to demo messages when enabled.
	MentionUser string

	// SecretNumber is an integer that, when mentioned in a message by a user, will trigger the demo user to post a message.
	SecretNumber int

	// A deplay in seconds that is applied to Slash Command responses, Post Actions responses and Interactive Dialog responses.
	// It's useful for testing.
	IntegrationRequestDelay int

	// disabled tracks whether or not the plugin has been disabled after activation. It always starts enabled.
	disabled bool

	// demoUserID is the id of the user specified above.
	demoUserID string

	// demoChannelIDs maps team ids to the channels created for each using the channel name above.
	demoChannelIDs map[string]string
}

func PrettyJSON(in interface{}) (string, error) {
	bb, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bb), nil
}


// Clone deep copies the configuration. Your implementation may only require a shallow copy if
// your configuration has no reference types.
func (c *configuration) Clone() *configuration {
	// Deep copy demoChannelIDs, a reference type.
	demoChannelIDs := make(map[string]string)
	for key, value := range c.demoChannelIDs {
		demoChannelIDs[key] = value
	}

	return &configuration{
		Username:                c.Username,
		ChannelName:             c.ChannelName,
		LastName:                c.LastName,
		TextStyle:               c.TextStyle,
		RandomSecret:            c.RandomSecret,
		SecretMessage:           c.SecretMessage,
		EnableMentionUser:       c.EnableMentionUser,
		MentionUser:             c.MentionUser,
		SecretNumber:            c.SecretNumber,
		IntegrationRequestDelay: c.IntegrationRequestDelay,
		disabled:                c.disabled,
		demoUserID:              c.demoUserID,
		demoChannelIDs:          demoChannelIDs,
	}
}

// getConfiguration retrieves the active configuration under lock, making it safe to use
// concurrently. The active configuration may change underneath the client of this method, but
// the struct returned by this API call is considered immutable.
func (p *Plugin) getConfiguration() *configuration {
	p.configurationLock.RLock()
	defer p.configurationLock.RUnlock()

	if p.configuration == nil {
		return &configuration{}
	}

	return p.configuration
}

// setConfiguration replaces the active configuration under lock.
//
// Do not call setConfiguration while holding the configurationLock, as sync.Mutex is not
// reentrant. In particular, avoid using the plugin API entirely, as this may in turn trigger a
// hook back into the plugin. If that hook attempts to acquire this lock, a deadlock may occur.
//
// This method panics if setConfiguration is called with the existing configuration. This almost
// certainly means that the configuration was modified without being cloned and may result in
// an unsafe access.
func (p *Plugin) setConfiguration(configuration *configuration) {
	p.configurationLock.Lock()
	defer p.configurationLock.Unlock()

	if configuration != nil && p.configuration == configuration {
		panic("setConfiguration called with the existing configuration")
	}

	p.configuration = configuration
}

func (p *Plugin) diffConfiguration(newConfiguration *configuration) {
	oldConfiguration := p.getConfiguration()
	configurationDiff := make(map[string]interface{})

	if newConfiguration.Username != oldConfiguration.Username {
		configurationDiff["username"] = newConfiguration.Username
	}
	if newConfiguration.ChannelName != oldConfiguration.ChannelName {
		configurationDiff["channel_name"] = newConfiguration.ChannelName
	}
	if newConfiguration.LastName != oldConfiguration.LastName {
		configurationDiff["lastname"] = newConfiguration.LastName
	}
	if newConfiguration.TextStyle != oldConfiguration.TextStyle {
		configurationDiff["text_style"] = newConfiguration.ChannelName
	}
	if newConfiguration.RandomSecret != oldConfiguration.RandomSecret {
		configurationDiff["random_secret"] = "<HIDDEN>"
	}
	if newConfiguration.SecretMessage != oldConfiguration.SecretMessage {
		configurationDiff["secret_message"] = newConfiguration.SecretMessage
	}
	if newConfiguration.EnableMentionUser != oldConfiguration.EnableMentionUser {
		configurationDiff["enable_mention_user"] = newConfiguration.EnableMentionUser
	}
	if newConfiguration.MentionUser != oldConfiguration.MentionUser {
		configurationDiff["mention_user"] = newConfiguration.MentionUser
	}
	if newConfiguration.SecretNumber != oldConfiguration.SecretNumber {
		configurationDiff["secret_number"] = newConfiguration.SecretNumber
	}

	if len(configurationDiff) == 0 {
		return
	}

	teams, err := p.API.GetTeams()
	if err != nil {
		p.API.LogWarn("Failed to query teams OnConfigChange", "err", err)
		return
	}

	for _, team := range teams {
		demoChannelID, ok := newConfiguration.demoChannelIDs[team.Id]
		if !ok {
			p.API.LogWarn("No demo channel id for team", "team", team.Id)
			continue
		}

		newConfigurationData, jsonErr := json.Marshal(newConfiguration)
		if jsonErr != nil {
			p.API.LogWarn("Failed to marshal new configuration", "err", err)
			return
		}

		fileInfo, err := p.API.UploadFile(newConfigurationData, demoChannelID, "configuration.json")
		if err != nil {
			p.API.LogWarn("Failed to attach new configuration", "err", err)
			return
		}

		if _, err := p.API.CreatePost(&model.Post{
			UserId:    p.botID,
			ChannelId: demoChannelID,
			Message:   "OnConfigChange: loading new configuration",
			Type:      "custom_demo_plugin",
			Props:     configurationDiff,
			FileIds:   model.StringArray{fileInfo.Id},
		}); err != nil {
			p.API.LogWarn("Failed to post OnConfigChange message", "err", err)
			return
		}
	}
}

// OnConfigurationChange is invoked when configuration changes may have been made.
//
// This demo implementation ensures the configured demo user and channel are created for use
// by the plugin.
func (p *Plugin) OnConfigurationChange() error {
	if p.client == nil {
		p.client = pluginapi.NewClient(p.API, p.Driver)
	}

	configuration := p.getConfiguration().Clone()

	// Load the public configuration fields from the Mattermost server configuration.
	if loadConfigErr := p.API.LoadPluginConfiguration(configuration); loadConfigErr != nil {
		return errors.Wrap(loadConfigErr, "failed to load plugin configuration")
	}

	demoUserID, err := p.ensureDemoUser(configuration)
	if err != nil {
		return errors.Wrap(err, "failed to ensure demo user")
	}
	configuration.demoUserID = demoUserID

	botID, ensureBotError := p.client.Bot.EnsureBot(&model.Bot{
		Username:    "demoplugin",
		DisplayName: "Demo Plugin Bot",
		Description: "A bot account created by the demo plugin.",
	}, pluginapi.ProfileImagePath("/assets/icon.png"))
	if ensureBotError != nil {
		return errors.Wrap(ensureBotError, "failed to ensure demo bot")
	}

	p.botID = botID

	configuration.demoChannelIDs, err = p.ensureDemoChannels(configuration)
	if err != nil {
		return errors.Wrap(err, "failed to ensure demo channels")
	}

	p.diffConfiguration(configuration)

	p.setConfiguration(configuration)

	return nil
}

// ConfigurationWillBeSaved is invoked before saving the configuration to the
// backing store.
// An error can be returned to reject the operation. Additionally, a new
// config object can be returned to be stored in place of the provided one.
// Minimum server version: 8.0
//
// This demo implementation logs a message to the demo channel whenever config
// is going to be saved.
// If the Username config option is set to "invalid" an error will be
// returned, resulting in the config not getting saved.
// If the Username config option is set to "replaceme" the config value will be
// replaced with "replaced".
func (p *Plugin) ConfigurationWillBeSaved(newCfg *model.Config) (*model.Config, error) {
	cfg := p.getConfiguration()
	if cfg.disabled {
		return nil, nil
	}

	teams, appErr := p.API.GetTeams()
	if appErr != nil {
		p.API.LogError(
			"Failed to query teams ConfigurationWillBeSaved",
			"error", appErr.Error(),
		)
		return nil, nil
	}

	msg := "Configuration will be saved"

	configData := newCfg.PluginSettings.Plugins[manifest.Id]
	js, err := json.Marshal(configData)
	if err != nil {
		p.API.LogError(
			"Failed to marshal config data ConfigurationWillBeSaved",
			"error", err.Error(),
		)
		return nil, nil
	}

	if err := json.Unmarshal(js, &cfg); err != nil {
		p.API.LogError(
			"Failed to unmarshal config data ConfigurationWillBeSaved",
			"error", err.Error(),
		)
		return nil, nil
	}

	if cfg == nil {
		return newCfg, nil
	}

	invalidUsernameUsed := cfg.Username == "invalid"
	replaceUsernameUsed := cfg.Username == "replaceme"

	if invalidUsernameUsed {
		msg = "Configuration won't be saved, invalid Username value used"
	} else if replaceUsernameUsed {
		msg = "Configuration will be save, replacing Username value"
	}

	for _, team := range teams {
		if err := p.postPluginMessage(team.Id, msg); err != nil {
			p.API.LogError(
				"Failed to post ConfigurationWillBeSaved message",
				"channel_id", cfg.demoChannelIDs[team.Id],
				"error", err.Error(),
			)
		}
	}

	if invalidUsernameUsed {
		return nil, errors.New(msg)
	}

	if replaceUsernameUsed {
		newCfg.PluginSettings.Plugins[manifest.Id]["username"] = "replaced"
		return newCfg, nil
	}

	return nil, nil
}

func (p *Plugin) ensureDemoUser(configuration *configuration) (string, error) {
	user, err := p.API.GetUserByUsername(configuration.Username)
	if err != nil {
		if err.StatusCode == http.StatusNotFound {
			p.API.LogInfo("DemoUser doesn't exist. Trying to create it.")

			user, err = p.API.CreateUser(&model.User{
				Username:  configuration.Username,
				Password:  "Password_123",
				Email:     fmt.Sprintf("%s@example.com", configuration.Username),
				Nickname:  "Demo Day",
				FirstName: "Demo",
				LastName:  configuration.LastName,
				Position:  "Bot",
			})

			if err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	if user.LastName != configuration.LastName {
		user.LastName = configuration.LastName
		user, err = p.API.UpdateUser(user)

		if err != nil {
			return "", err
		}
	}

	teams, err := p.API.GetTeams()
	if err != nil {
		return "", err
	}

	for _, team := range teams {
		_, err := p.API.CreateTeamMember(team.Id, user.Id)
		if err != nil {
			p.API.LogError("Failed add demo user to team", "teamID", team.Id, "error", err.Error())
		}
	}

	return user.Id, nil
}

func (p *Plugin) ensureDemoChannels(configuration *configuration) (map[string]string, error) {
	teams, err := p.API.GetTeams()
	if err != nil {
		return nil, err
	}

	demoChannelIDs := make(map[string]string)
	for _, team := range teams {
		// Check for the configured channel. Ignore any error, since it's hard to
		// distinguish runtime errors from a channel simply not existing.
		channel, _ := p.API.GetChannelByNameForTeamName(team.Name, configuration.ChannelName, false)

		// Ensure the configured channel exists.
		if channel == nil {
			channel, err = p.API.CreateChannel(&model.Channel{
				TeamId:      team.Id,
				Type:        model.ChannelTypeOpen,
				DisplayName: "Demo Plugin",
				Name:        configuration.ChannelName,
				Header:      "The channel used by the demo plugin.",
				Purpose:     "This channel was created by a plugin for testing.",
			})

			if err != nil {
				return nil, err
			}
		}

		// Save the ids for later use.
		demoChannelIDs[team.Id] = channel.Id
	}

	return demoChannelIDs, nil
}

// setEnabled wraps setConfiguration to configure if the plugin is enabled.
func (p *Plugin) setEnabled(enabled bool) {
	var configuration = p.getConfiguration().Clone()
	configuration.disabled = !enabled

	p.setConfiguration(configuration)
}

type Plugin struct {
	plugin.MattermostPlugin
	client *pluginapi.Client

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration

	router *mux.Router

	// BotId of the created bot account.
	botID string

	// backgroundJob is a job that executes periodically on only one plugin instance at a time
	backgroundJob *cluster.Job
}

func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}

func (p *Plugin) handleStatus(w http.ResponseWriter, r *http.Request) {
	configuration := p.getConfiguration()

	var response = struct {
		Enabled bool `json:"enabled"`
	}{
		Enabled: !configuration.disabled,
	}

	responseJSON, _ := json.Marshal(response)

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(responseJSON); err != nil {
		p.API.LogError("Failed to write status", "err", err.Error())
	}
}

func (p *Plugin) handleHello(w http.ResponseWriter, r *http.Request) {
	if _, err := w.Write([]byte("Hello World!")); err != nil {
		p.API.LogError("Failed to write hello world", "err", err.Error())
	}
}

func (p *Plugin) handleDynamicArgTest(w http.ResponseWriter, r *http.Request) {
	queryArgs := []string{"user_input", "parsed", "root_id", "parent_id", "user_id", "site_url", "request_id", "session_id", "ip_address", "accept_language", "user_agent"}
	query := r.URL.Query()

	channelID := query.Get("channel_id")
	teamID := query.Get("team_id")
	userID := query.Get("user_id")
	rootID := query.Get("root_id")

	channel, appErr := p.API.GetChannel(channelID)
	if appErr != nil {
		http.Error(w, fmt.Sprintf("Error getting channels: %s", appErr.Error()), http.StatusInternalServerError)
		return
	}
	team, appErr := p.API.GetTeam(teamID)
	if appErr != nil {
		http.Error(w, fmt.Sprintf("Error getting team: %s", appErr.Error()), http.StatusInternalServerError)
		return
	}
	user, appErr := p.API.GetUser(userID)
	if appErr != nil {
		http.Error(w, fmt.Sprintf("Error getting user: %s", appErr.Error()), http.StatusInternalServerError)
		return
	}

	argMap := map[string]string{}
	for _, arg := range queryArgs {
		argMap[arg] = query.Get(arg)
	}
	argMapString := ""
	for k, v := range argMap {
		argMapString = fmt.Sprintf("%s  * %s:%s\n", argMapString, k, v)
	}
	result := fmt.Sprintf("dynamic argument was triggered by **%v** from team **%v** in the **%v** channel, with these arguments\n\n%v", user.GetFullName(), team.DisplayName, channel.DisplayName, argMapString)
	post := &model.Post{
		ChannelId: channelID,
		RootId:    rootID,
		UserId:    p.botID,
		Message:   result,
	}

	_, appErr = p.API.CreatePost(post)
	if appErr != nil {
		http.Error(w, fmt.Sprintf("Error creating post: %s", appErr.Error()), http.StatusInternalServerError)
		return
	}

	suggestions := []model.AutocompleteListItem{{
		Item:     "suggestion 1",
		HelpText: "help text 1",
		Hint:     "(hint)",
	}, {
		Item:     "suggestion 2",
		HelpText: "help text 2",
		Hint:     "(hint)",
	}}

	jsonBytes, err := json.Marshal(suggestions)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting dynamic args: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(jsonBytes); err != nil {
		http.Error(w, fmt.Sprintf("Error getting dynamic args: %s", err.Error()), http.StatusInternalServerError)
		return
	}
}

func (p *Plugin) handleCheckAuthHeader(w http.ResponseWriter, r *http.Request) {
	isAuthenticatedUser := r.Header.Get("Mattermost-User-ID") != ""
	authHeader := r.Header.Get(model.HeaderAuth)

	responseMessage := ""

	if isAuthenticatedUser {
		responseMessage += "You are an authenticated user. The Authorization header should be an empty string.\n"
	}

	responseMessage += fmt.Sprintf("Authorization header: %s", authHeader)

	if _, err := w.Write([]byte(responseMessage)); err != nil {
		p.API.LogError("Failed to write checkAuthHeader message", "err", err.Error())
	}
}

func (p *Plugin) handleOutgoingWebhook(w http.ResponseWriter, r *http.Request) {
	var request model.OutgoingWebhookPayload
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		p.API.LogError("Failed to decode OutgoingWebhookPayload", "err", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	s, err := PrettyJSON(request)
	if err != nil {
		p.API.LogError("Failed to Marshal payload back to JSON", "err", err.Error())
		return
	}

	text := "```\n" + s + "\n```"
	resp := model.OutgoingWebhookResponse{
		Text: &text,
	}

	p.writeJSON(w, resp)
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

func (p *Plugin) handleInteractiveAction(w http.ResponseWriter, r *http.Request) {
	var request model.PostActionIntegrationRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		p.API.LogError("Failed to decode PostActionIntegrationRequest", "err", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	user, appErr := p.API.GetUser(request.UserId)
	if appErr != nil {
		p.API.LogError("Failed to get user for interactive action", "err", appErr.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	post, postErr := p.API.GetPost(request.PostId)
	if postErr != nil {
		p.API.LogError("Failed to get post for interactive action", "err", postErr.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	rootID := post.RootId
	if rootID == "" {
		rootID = post.Id
	}

	requestJSON, jsonErr := json.MarshalIndent(request, "", "  ")
	if jsonErr != nil {
		p.API.LogError("Failed to marshal json for interactive action", "err", jsonErr.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	msg := "@%v clicked an interactive button.\n```json\n%v\n```"
	if _, appErr := p.API.CreatePost(&model.Post{
		UserId:    p.botID,
		ChannelId: request.ChannelId,
		RootId:    rootID,
		Message:   fmt.Sprintf(msg, user.Username, string(requestJSON)),
	}); appErr != nil {
		p.API.LogError("Failed to post handleInteractiveAction message", "err", appErr.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp := &model.PostActionIntegrationResponse{}
	p.writeJSON(w, resp)
}

func (p *Plugin) handleDialog2(w http.ResponseWriter, r *http.Request) {
	var request model.SubmitDialogRequest
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

	suffix := ""
	if request.State == dialogStateRelativeCallbackURL {
		suffix = "from relative callback URL"
	}

	if _, appErr = p.API.CreatePost(&model.Post{
		UserId:    p.botID,
		ChannelId: request.ChannelId,
		Message:   fmt.Sprintf("@%v confirmed an Interactive Dialog %v", user.Username, suffix),
	}); appErr != nil {
		p.API.LogError("Failed to post handleDialog2 message", "err", appErr.Error())
		return
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

func (p *Plugin) initializeAPI() {
	router := mux.NewRouter()

	router.HandleFunc("/status", p.handleStatus)
	router.HandleFunc("/hello", p.handleHello)
	router.HandleFunc("/dynamic_arg_test_url", p.handleDynamicArgTest)
	router.HandleFunc("/check_auth_header", p.handleCheckAuthHeader)

	webhook := router.PathPrefix("/webhook").Subrouter()
	webhook.Use(p.withDelay)
	webhook.HandleFunc("/outgoing", p.handleOutgoingWebhook).Methods(http.MethodPost)

	interativeRouter := router.PathPrefix("/interactive").Subrouter()
	interativeRouter.Use(p.withDelay)
	interativeRouter.HandleFunc("/button/1", p.handleInteractiveAction)

	dialogRouter := router.PathPrefix("/dialog").Subrouter()
	dialogRouter.Use(p.withDelay)
	dialogRouter.HandleFunc("/1", p.handleDialog1)
	dialogRouter.HandleFunc("/2", p.handleDialog2)
	dialogRouter.HandleFunc("/error", p.handleDialogWithError)

	p.router = router
}


func (p *Plugin) handleDialog1(w http.ResponseWriter, r *http.Request) {
	var request model.SubmitDialogRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		p.API.LogError("Failed to decode SubmitDialogRequest", "err", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if !request.Cancelled {
		number, ok := request.Submission[dialogElementNameNumber].(float64)
		if !ok {
			p.API.LogError("Request is missing field", "field", dialogElementNameNumber)
			w.WriteHeader(http.StatusOK)
			return
		}

		if number != 42 {
			response := &model.SubmitDialogResponse{
				Errors: map[string]string{
					dialogElementNameNumber: "This must be 42",
				},
			}
			p.writeJSON(w, response)
			return
		}
	}

	user, appErr := p.API.GetUser(request.UserId)
	if appErr != nil {
		p.API.LogError("Failed to get user for dialog", "err", appErr.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	msg := "@%v submitted an Interative Dialog"
	if request.Cancelled {
		msg = "@%v canceled an Interative Dialog"
	}

	rootPost, appErr := p.API.CreatePost(&model.Post{
		UserId:    p.botID,
		ChannelId: request.ChannelId,
		Message:   fmt.Sprintf(msg, user.Username),
	})
	if appErr != nil {
		p.API.LogError("Failed to post handleDialog1 message", "err", appErr.Error())
		return
	}

	if !request.Cancelled {
		// Don't post the email address publicly
		request.Submission[dialogElementNameEmail] = "xxxxxxxxxxx"

		if _, appErr = p.API.CreatePost(&model.Post{
			UserId:    p.botID,
			ChannelId: request.ChannelId,
			RootId:    rootPost.Id,
			Message:   "Data:",
			Type:      "custom_demo_plugin",
			Props:     request.Submission,
		}); appErr != nil {
			p.API.LogError("Failed to post handleDialog1 message", "err", appErr.Error())
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}