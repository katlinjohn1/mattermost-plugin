package plugin

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"

	root "github.com/mattermost/mattermost-plugin-demo"
)

// This is what the root declaration is bringing in (I think)
// var manifestString string

// var Manifest model.Manifest

// func init() {
// 	_ = json.NewDecoder(strings.NewReader(manifestString)).Decode(&Manifest)
// }

// change this reference to local manifest?
var (
	manifest model.Manifest = root.Manifest
)

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

const (
	commandTriggerCrash             = "crash"
	commandTriggerHooks             = "demo_plugin"
	commandTriggerDialog            = "dialog"
	commandTriggerInteractive       = "interactive"

	dialogStateSome                = "somestate"
	dialogStateRelativeCallbackURL = "relativecallbackstate"
	dialogIntroductionText         = "To request help from the Control Plane or Platform Factory team, please fill out the form"
)

func (p *Plugin) registerCommands() error {
	if err := p.API.RegisterCommand(&model.Command{

		Trigger:          commandTriggerHooks,
		AutoComplete:     true,
		AutoCompleteHint: "(true|false)",
		AutoCompleteDesc: "Enables or disables the demo plugin hooks.",
		AutocompleteData: getCommandHooksAutocompleteData(),
	}); err != nil {
		return errors.Wrapf(err, "failed to register %s command", commandTriggerHooks)
	}

	if err := p.API.RegisterCommand(&model.Command{
		Trigger:          commandTriggerCrash,
		AutoComplete:     true,
		AutoCompleteHint: "",
		AutoCompleteDesc: "Crashes Demo Plugin",
	}); err != nil {
		return errors.Wrapf(err, "failed to register %s command", commandTriggerCrash)
	}

	if err := p.API.RegisterCommand(&model.Command{
		Trigger:          commandTriggerDialog,
		AutoComplete:     true,
		AutoCompleteDesc: "Open an Interactive Dialog.",
		DisplayName:      "Demo Plugin Command",
		AutocompleteData: getCommandDialogAutocompleteData(),
	}); err != nil {
		return errors.Wrapf(err, "failed to register %s command", commandTriggerDialog)
	}

	if err := p.API.RegisterCommand(&model.Command{
		Trigger:          commandTriggerInteractive,
		AutoComplete:     true,
		AutoCompleteHint: "",
		AutoCompleteDesc: "Demonstrates  interactive message buttons.",
	}); err != nil {
		return errors.Wrapf(err, "failed to register %s command", commandTriggerInteractive)
	}

	return nil
}

func getCommandDialogAutocompleteData() *model.AutocompleteData {
	command := model.NewAutocompleteData(commandTriggerDialog, "", "Open an Interactive Dialog.")

	noElements := model.NewAutocompleteData("no-elements", "", "Open an Interactive Dialog with no elements.")
	command.AddCommand(noElements)

	relativeCallbackURL := model.NewAutocompleteData("relative-callback-url", "", "Open an Interactive Dialog with a relative callback url.")
	command.AddCommand(relativeCallbackURL)

	introText := model.NewAutocompleteData("introduction-text", "", "Open an Interactive Dialog with an introduction text.")
	command.AddCommand(introText)

	error := model.NewAutocompleteData("error", "", "Open an Interactive Dialog with error.")
	command.AddCommand(error)

	errorNoElements := model.NewAutocompleteData("error-no-elements", "", "Open an Interactive Dialog with error no elements.")
	command.AddCommand(errorNoElements)

	help := model.NewAutocompleteData("help", "", "")
	command.AddCommand(help)

	return command
}

func getCommandHooksAutocompleteData() *model.AutocompleteData {
	command := model.NewAutocompleteData(commandTriggerHooks, "", "Enables or disables the demo plugin hooks.")
	command.AddStaticListArgument("", true, []model.AutocompleteListItem{
		{
			Item:     "true",
			HelpText: "Enable demo plugin hooks",
		}, {
			Item:     "false",
			HelpText: "Disable demo plugin hooks",
		},
	})
	return command
}

func (p *Plugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	delay := p.getConfiguration().IntegrationRequestDelay
	if delay > 0 {
		time.Sleep(time.Duration(delay) * time.Second)
	}

	trigger := strings.TrimPrefix(strings.Fields(args.Command)[0], "/")
	switch trigger {
	case commandTriggerCrash:
		return p.executeCommandCrash(), nil
	case commandTriggerHooks:
		return p.executeCommandHooks(args), nil
	case commandTriggerDialog:
		return p.executeCommandDialog(args), nil
	case commandTriggerInteractive:
		return p.executeCommandInteractive(args), nil
	default:
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Unknown command: " + args.Command),
		}, nil
	}
}

func getDialogWithSampleElements() model.Dialog {
	return model.Dialog{
		CallbackId: "somecallbackid",
		Title:      "Support",
		IconURL:    "http://www.mattermost.org/wp-content/uploads/2016/04/icon.png",
		Elements: []model.DialogElement{{
			DisplayName: "Short Description",
			Name:        "shortDescription",
			Type:        "text",
			Placeholder: "Enter a quick description of the issue that's occurring",
		}, {
			DisplayName: "Long Description",
			Name:        "longDescription",
			Type:        "textarea",
			Placeholder: "Please describe the issue including any error messages or code snippets",
			Optional:    false,
			MinLength:   5,
			MaxLength:   200,
		}, {
			DisplayName: "Impact to Users",
			Name:        "userImpact",
			Type:        "select",
			Placeholder: "Select an option...",
			HelpText:    "Choose an option from the list.",
			Options: []*model.PostActionOptions{{
				Text:  "Low",
				Value: "opt1",
			}, {
				Text:  "Medium",
				Value: "opt2",
			}, {
				Text:  "High",
				Value: "opt3",
			}},
		}, {
			DisplayName: "Link to failed Pipeline",
			Name:        "pipeline",
			Type:        "textarea",
			Placeholder: "If this is happening in a pipeline, please include a link to the failed pipeline",
		}, {
			DisplayName: "Steps to replicate the issue",
			Name:        "replication",
			Type:        "textarea",
			Placeholder: "placeholder",
			MinLength:   5,
			MaxLength:   200,
		}},
		SubmitLabel:    "Submit",
		NotifyOnCancel: true,
		State:          dialogStateSome,
	}
}

func (p *Plugin) executeCommandCrash() *model.CommandResponse {
	go p.crash()
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         "Crashing plugin",
	}
}

func (p *Plugin) executeCommandHooks(args *model.CommandArgs) *model.CommandResponse {
	configuration := p.getConfiguration()

	if strings.HasSuffix(args.Command, "true") {
		if !configuration.disabled {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "The demo plugin hooks are already enabled.",
			}
		}

		p.setEnabled(true)
		p.emitStatusChange()

		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Enabled demo plugin hooks.",
		}
	}

	if strings.HasSuffix(args.Command, "false") {
		if configuration.disabled {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "The demo plugin hooks are already disabled.",
			}
		}

		p.setEnabled(false)
		p.emitStatusChange()

		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Disabled demo plugin hooks.",
		}
	}

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         fmt.Sprintf("Unknown command action: " + args.Command),
	}
}

func (p *Plugin) executeCommandDialog(args *model.CommandArgs) *model.CommandResponse {
	serverConfig := p.API.GetConfig()

	var dialogRequest model.OpenDialogRequest
	fields := strings.Fields(args.Command)
	command := ""
	if len(fields) == 2 {
		command = fields[1]
	}

	switch command {
	case "":
		dialogRequest = model.OpenDialogRequest{
			TriggerId: args.TriggerId,
			URL:       fmt.Sprintf("%s/plugins/%s/dialog/1", *serverConfig.ServiceSettings.SiteURL, manifest.Id),
			Dialog:    getDialogWithSampleElements(),
		}
	default:
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Unknown command: " + command),
		}
	}

	if err := p.API.OpenInteractiveDialog(dialogRequest); err != nil {
		errorMessage := "Failed to open Interactive Dialog"
		p.API.LogError(errorMessage, "err", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         errorMessage,
		}
	}
	return &model.CommandResponse{}
}

func (p *Plugin) executeCommandInteractive(args *model.CommandArgs) *model.CommandResponse {
	post := &model.Post{
		ChannelId: args.ChannelId,
		RootId:    args.RootId,
		UserId:    p.botID,
		Message:   "Test interactive button",
		Props: model.StringInterface{
			"attachments": []*model.SlackAttachment{{
				Actions: []*model.PostAction{{
					Integration: &model.PostActionIntegration{
						URL: fmt.Sprintf("/plugins/%s/interactive/button/1", manifest.Id),
					},
					Type: model.PostActionTypeButton,
					Name: "Interactive Button",
				}},
			}},
		},
	}

	_, err := p.API.CreatePost(post)
	if err != nil {
		const errorMessage = "Failed to create post"
		p.API.LogError(errorMessage, "err", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         errorMessage,
		}
	}

	return &model.CommandResponse{}
}

func (p *Plugin) crash() {
	<-time.NewTimer(time.Second).C
	y := 0
	_ = 1 / y
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

// setEnabled wraps setConfiguration to configure if the plugin is enabled.
func (p *Plugin) setEnabled(enabled bool) {
	var configuration = p.getConfiguration().Clone()
	configuration.disabled = !enabled

	p.setConfiguration(configuration)
}

func (p *Plugin) emitStatusChange() {
	configuration := p.getConfiguration()

	p.API.PublishWebSocketEvent("status_change", map[string]interface{}{
		"enabled": !configuration.disabled,
	}, &model.WebsocketBroadcast{})
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