package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const (
	commandTriggerCrash             = "crash"
	commandTriggerHooks             = "demo_plugin"
	commandTriggerDialog            = "sre-request"

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

	return nil
}

func getCommandDialogAutocompleteData() *model.AutocompleteData {
	command := model.NewAutocompleteData(commandTriggerDialog, "", "Open an Interactive Dialog.")

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
			DisplayName: "Type of Request",
			Name:        "1-Type of Request",
			Type:        "select",
			Placeholder: "Select an option...",
			HelpText:    "Choose an option from the list.",
			Options: []*model.PostActionOptions{{
				Text:  "Bug",
				Value: "Bug",
			}, {
				Text:  "Feature Request",
				Value: "Feature Request",
			}},
		}, {
			DisplayName: "Short Description",
			Name:        "2-Short Description",
			Type:        "text",
			Placeholder: "Enter a quick description of the issue that's occurring",
		}, {
			DisplayName: "Long Description",
			Name:        "3-Long Description",
			Type:        "textarea",
			Placeholder: "Please describe the issue including any error messages or code snippets",
		}, {
			DisplayName: "Impact to Users",
			Name:        "4-User Impact",
			Type:        "select",
			Placeholder: "Select an option...",
			HelpText:    "Choose an option from the list.",
			Options: []*model.PostActionOptions{{
				Text:  "Low",
				Value: "Low",
			}, {
				Text:  "Medium",
				Value: "Medium",
			}, {
				Text:  "High",
				Value: "High",
			}},
		}, {
			DisplayName: "Link to Failed Pipeline",
			Name:        "5-Link to Failed Pipeline",
			Type:        "textarea",
			Placeholder: "If this is happening in a pipeline, please include a link to the failed pipeline",
			Optional: true,
		}, {
			DisplayName: "Steps to replicate the issue",
			Name:        "6-Steps to replicate",
			Type:        "textarea",
			Placeholder: "placeholder",
			Optional: true,
		}},
		SubmitLabel:    "Submit",
		NotifyOnCancel: false,
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
	// serverConfig := p.API.GetConfig()

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
			// URL:       fmt.Sprintf("%s/plugins/%s/sre-request/submit-request", *serverConfig.ServiceSettings.SiteURL , manifest.Id),
			URL:       fmt.Sprintf("%s/plugins/%s/sre-request/submit-request", "http://127.0.0.1:8065", manifest.Id),
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

func (p *Plugin) crash() {
	<-time.NewTimer(time.Second).C
	y := 0
	_ = 1 / y
}

func (p *Plugin) emitStatusChange() {
	configuration := p.getConfiguration()

	p.API.PublishWebSocketEvent("status_change", map[string]interface{}{
		"enabled": !configuration.disabled,
	}, &model.WebsocketBroadcast{})
}

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

func (p *Plugin) BackgroundJob() {
	configuration := p.getConfiguration()

	if configuration.disabled {
		return
	}

	for _, channelID := range configuration.demoChannelIDs {
		_, err := p.API.CreatePost(&model.Post{
			UserId:    p.botID,
			ChannelId: channelID,
			Message:   "Background job executed",
		})
		if err != nil {
			p.API.LogError(
				"Failed to post BackgroundJob message",
				"channel_id", channelID,
				"error", err.Error(),
			)
		}
	}
}

func PrettyJSON(in interface{}) (string, error) {
	bb, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bb), nil
}
