package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

func HandleAppMentionEventToBot(event *slackevents.AppMentionEvent, client *slack.Client) error {

	user, err := client.GetUserInfo(event.User)
	if err != nil {
		return err
	}

	text := strings.ToLower(event.Text)

	attachment := slack.Attachment{}

	if strings.Contains(text, "hello") || strings.Contains(text, "hi") {
		attachment.Text = fmt.Sprintf("Hello %s", user.Name)
		attachment.Color = "#4af030"
	} else if strings.Contains(text, "weather") {
		attachment.Text = fmt.Sprintf("Weather is sunny today. %s", user.Name)
		attachment.Color = "#4af030"
	} else {
		attachment.Text = fmt.Sprintf("I am good. How are you %s?", user.Name)
		attachment.Color = "#4af030"
	}
	_, _, err = client.PostMessage(event.Channel, slack.MsgOptionAttachments(attachment))
	if err != nil {
		return fmt.Errorf("failed to post message: %w", err)
	}
	return nil
}

func HandleEventMessage(event slackevents.EventsAPIEvent, client *slack.Client) error {
	switch event.Type {
	case slackevents.CallbackEvent:
		innerEvent := event.InnerEvent
		switch evnt := innerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			err := HandleAppMentionEventToBot(evnt, client)
			if err != nil {
				return err
			}
		}
	default:
		return errors.New("unsupported event type")
	}
	return nil
}

func handleCreateMeeting(cmd slack.SlashCommand, client *slack.Client) error {
	attachment := slack.Attachment{
		Text:       "Please enter a name for the meeting:",
		CallbackID: "meeting_name",
		Actions: []slack.AttachmentAction{
			//probleme is here ***************************************************************************************************************************************************************************************************
			slack.AttachmentAction{
				Name: "meeting_name",
				Type: "input",
				Options: []slack.AttachmentActionOption{
					slack.AttachmentActionOption{
						Text:  "Meeting 1",
						Value: "Meeting 1",
					},
					slack.AttachmentActionOption{
						Text:  "Meeting 2",
						Value: "Meeting 2",
					},
				},
			},
		},
	}

	_, _, err := client.PostMessage(cmd.ChannelID, slack.MsgOptionAttachments(attachment))
	if err != nil {
		log.Printf("Failed to send the interactive message: %v", err)
	}
	return nil
}

func handleInteractiveCallback(callback slack.InteractionCallback, client *slack.Client) {
	if callback.CallbackID == "meeting_name" {
		fmt.Println("******************************************************************************************************")
		selectedValue := callback.ActionCallback.BlockActions[0].SelectedOption.Value
		responseMessage := fmt.Sprintf("You selected: %s", selectedValue)
		_, _, err := client.PostMessage(callback.Channel.ID, slack.MsgOptionText(responseMessage, false))
		if err != nil {
			log.Printf("Failed to send the response message: %v", err)
		}
	} else {
		log.Printf("Unknown callback ID: %s", callback.CallbackID)
	}
}

func main() {

	godotenv.Load(".env")

	token := os.Getenv("SLACK_BOT_TOKEN")
	appToken := os.Getenv("SLACK_APP_TOKEN")

	client := slack.New(token, slack.OptionDebug(true), slack.OptionAppLevelToken(appToken))

	socketClient := socketmode.New(
		client,
		socketmode.OptionDebug(true),
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.Lshortfile|log.LstdFlags)),
	)

	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	go func(ctx context.Context, client *slack.Client, socketClient *socketmode.Client) {
		for {
			select {
			case <-ctx.Done():
				log.Println("Shutting down socketmode listener")
				return
			case event := <-socketClient.Events:
				switch event.Type {
				case socketmode.RequestTypeSlashCommands:
					cmd, ok := event.Data.(slack.SlashCommand)
					if !ok {
						log.Printf("Could not type cast the event to the SlashCommand: %v\n", event)
						continue
					}
					log.Printf("Received slash command: %s, arguments: %s\n", cmd.Command, cmd.Text)

					handleCreateMeeting(cmd, client)

					socketClient.Ack(*event.Request)
				case socketmode.EventTypeInteractive:
					callback, ok := event.Data.(slack.InteractionCallback)
					if !ok {
						log.Printf("Could not type cast the event to the InteractionCallback: %v\n", event)
						continue
					}
					handleInteractiveCallback(callback, client)
					socketClient.Ack(*event.Request)
				case socketmode.EventTypeEventsAPI:
					eventsAPI, ok := event.Data.(slackevents.EventsAPIEvent)
					if !ok {
						log.Printf("Could not type cast the event to the EventsAPI: %v\n", event)
						continue
					}

					socketClient.Ack(*event.Request)
					err := HandleEventMessage(eventsAPI, client)
					if err != nil {
						log.Fatal(err)
					}
				}
			}
		}
	}(ctx, client, socketClient)

	socketClient.Run()
}
