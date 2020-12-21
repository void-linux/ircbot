package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"strings"

	"github.com/thoj/go-ircevent"
	"gopkg.in/go-playground/webhooks.v5/github"
)

func main() {
	// Config data
	serverssl := os.Getenv("IRC_SERVER")
	channel := os.Getenv("IRC_CHANNEL")
	ircnick1 := os.Getenv("IRC_NICK")

	// Set up the connection
	conn := irc.IRC(ircnick1, ircnick1)
	if conn == nil {
		log.Println("conn is nil!  Did you set IRC_NICK?")
		return
	}

	// IRC startup
	conn.QuitMessage = "I've probably crashed..."
	conn.UseTLS = true
	conn.AddCallback("001", func(e *irc.Event) {
		log.Println("Connected to Server")
		conn.Join(channel)
	})
	conn.AddCallback("366", func(e *irc.Event) {
		log.Println("Connected to Channel")
	})
	err := conn.Connect(serverssl)
	if err != nil {
		log.Println(err)
		return
	}

	// HTTP Setup
	hook, _ := github.New(github.Options.Secret(os.Getenv("WEBHOOK_SECRET")))
	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		payload, err := hook.Parse(r,
			github.PushEvent,
			github.PullRequestEvent,
		)
		if err != nil {
			log.Println("Error parsing hook")
			return
		}
		switch p := payload.(type) {
		case github.PullRequestPayload:
			if p.Action != "opened" && p.Action != "closed" {
				return
			}
			conn.Noticef(channel, "%s %s #%d (%s)", p.Sender.Login, p.Action, p.Number, p.PullRequest.Title)
		case github.PushPayload:
			// This should probably be filtering on
			// branches.
			shortMsg := p.HeadCommit.Message
			idx := strings.Index(shortMsg, "\n")
			if idx != -1 {
				shortMsg = shortMsg[0:idx]
			}
			conn.Noticef(channel, "%s pushed to %s (%s)", p.Sender.Login, p.Repository.Name, shortMsg)
		}
	})

	// Shutdown handler setup
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs

		log.Println(sig)
		done <- true
	}()

	// Startup Serving
	go conn.Loop()
	go http.ListenAndServe(":3000", nil)

	// Shut down
	<-done
	log.Println("exiting")
	conn.Quit()
}
